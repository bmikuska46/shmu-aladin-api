package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/config"
	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/shmu"
	"github.com/bmikuska/shmu-weather-api/internal/store"
	"github.com/bmikuska/shmu-weather-api/internal/transform"
	"github.com/robfig/cron/v3"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

var ErrRuntimeNotReady = errors.New("aladin runtime not published yet")

type Syncer struct {
	cfg    config.Config
	client *shmu.Client
	store  *store.Store
	log    *log.Logger

	fetchMu       sync.Mutex
	stationFlight singleflight.Group
	upstream      *rate.Limiter
	cron          *cron.Cron
}

func New(cfg config.Config, client *shmu.Client, st *store.Store, logger *log.Logger) *Syncer {
	if logger == nil {
		logger = log.Default()
	}
	ratePerSec := cfg.SyncRatePerSec
	if ratePerSec < 1 {
		ratePerSec = 20
	}
	workers := cfg.SyncWorkers
	if workers < 1 {
		workers = 3
	}
	return &Syncer{
		cfg:      cfg,
		client:   client,
		store:    st,
		log:      logger,
		upstream: rate.NewLimiter(rate.Limit(ratePerSec), workers),
	}
}

func (s *Syncer) Run(ctx context.Context) {
	if err := s.SyncStationsIfNeeded(ctx, true); err != nil {
		s.log.Printf("stations sync error: %v", err)
	}

	if !s.cfg.SyncForecasts {
		s.log.Printf("forecast cron disabled (SYNC_FORECASTS=false); forecasts load on demand")
		s.wait(ctx)
		return
	}

	s.cron = cron.New(cron.WithLocation(time.UTC), cron.WithSeconds())
	// Stations: once a day at 04:00 UTC (after overnight 00-run publish window).
	if _, err := s.cron.AddFunc("0 0 4 * * *", func() {
		if err := s.SyncStationsIfNeeded(context.Background(), false); err != nil {
			s.log.Printf("stations cron error: %v", err)
		}
	}); err != nil {
		s.log.Printf("stations cron schedule error: %v", err)
	}

	delay := s.cfg.FetchDelayAfterPublish
	for _, slot := range DefaultAladinSlots {
		sl := slot
		spec := "0 " + sl.CronSpec(delay) // prepend seconds=0 for cron.WithSeconds
		_, err := s.cron.AddFunc(spec, func() {
			day := time.Now().UTC()
			runtime := sl.ExpectedRuntime(day)
			s.log.Printf("cron: starting ALADIN fetch for run %s (publish was %02d:%02d UTC + %s)",
				runtime.Format(time.RFC3339), sl.PublishHour, sl.PublishMin, delay)
			s.fetchRuntimeWithRetries(context.Background(), runtime)
		})
		if err != nil {
			s.log.Printf("aladin cron schedule error for run %02dz: %v", sl.RunHour, err)
			continue
		}
		s.log.Printf("scheduled ALADIN cron %q → run %02d:00 UTC (first try publish+%s)",
			spec, sl.RunHour, delay)
	}

	s.cron.Start()
	defer s.cron.Stop()

	// Catch up if we started after a publish window began.
	go s.catchUpIfNeeded(ctx)

	s.wait(ctx)
}

func (s *Syncer) wait(ctx context.Context) {
	<-ctx.Done()
}

func (s *Syncer) catchUpIfNeeded(ctx context.Context) {
	slot, runtime, due := CurrentOrDueSlot(time.Now().UTC(), s.cfg.FetchDelayAfterPublish, DefaultAladinSlots)
	if !due || runtime.IsZero() {
		s.log.Printf("no ALADIN catch-up needed at startup")
		return
	}
	want := runtime.Format(time.RFC3339)
	if last, ok, err := s.store.GetMeta(ctx, "last_aladin_runtime"); err == nil && ok && last == want {
		s.log.Printf("catch-up: run %s already marked complete", want)
		return
	}
	s.log.Printf("catch-up: fetching run %s (slot publish %02d:%02d UTC)",
		want, slot.PublishHour, slot.PublishMin)
	s.fetchRuntimeWithRetries(ctx, runtime)
}

// fetchRuntimeWithRetries polls every FetchRetryEvery until the expected runtime
// is published on SHMU (probe station), then syncs all stations.
func (s *Syncer) fetchRuntimeWithRetries(ctx context.Context, runtime time.Time) {
	if !s.fetchMu.TryLock() {
		s.log.Printf("fetch already in progress, skipping overlapping job for %s", runtime.Format(time.RFC3339))
		return
	}
	defer s.fetchMu.Unlock()

	runtimeTS := runtime.Unix()
	retry := s.cfg.FetchRetryEvery
	if retry <= 0 {
		retry = 5 * time.Minute
	}
	deadline := nextFirstAttempt(time.Now().UTC(), s.cfg.FetchDelayAfterPublish, DefaultAladinSlots)
	if deadline.IsZero() {
		deadline = time.Now().UTC().Add(3 * time.Hour)
	}

	attempt := 0
	for {
		attempt++
		if ctx.Err() != nil {
			return
		}

		s.log.Printf("ALADIN fetch attempt #%d for run %s", attempt, runtime.Format(time.RFC3339))
		ready, err := s.probeHasRuntime(ctx, runtimeTS)
		if err != nil {
			s.log.Printf("probe error: %v — retry in %s", err, retry)
		} else if !ready {
			s.log.Printf("run %s not published yet — retry in %s", runtime.Format(time.RFC3339), retry)
		} else {
			if err := s.SyncForecastsForRuntime(ctx, runtimeTS); err != nil {
				s.log.Printf("sync for run %s: %v — retry in %s", runtime.Format(time.RFC3339), err, retry)
			} else {
				s.log.Printf("ALADIN run %s fully synced", runtime.Format(time.RFC3339))
				_ = s.store.SetMeta(ctx, "last_aladin_runtime", runtime.Format(time.RFC3339))
				return
			}
		}

		now := time.Now().UTC()
		if !now.Before(deadline) {
			s.log.Printf("giving up on run %s — reached next publish window (%s)",
				runtime.Format(time.RFC3339), deadline.Format(time.RFC3339))
			return
		}

		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Syncer) probeHasRuntime(ctx context.Context, runtimeTS int64) (bool, error) {
	if err := s.waitUpstream(ctx); err != nil {
		return false, err
	}
	raw, err := s.client.GetStationProducts(ctx, s.cfg.ProbeStationID)
	if err != nil {
		return false, err
	}
	for _, p := range raw.Data {
		if p.Type == "aladin" && p.Runtime == runtimeTS {
			return true, nil
		}
	}
	return false, nil
}

func (s *Syncer) SyncStationsIfNeeded(ctx context.Context, force bool) error {
	if !force {
		syncedAt, ok, err := s.store.StationsSyncedAt(ctx)
		if err != nil {
			return err
		}
		if ok && time.Since(syncedAt) < s.cfg.StationsCacheTTL {
			s.log.Printf("stations cache fresh (synced %s), skipping", syncedAt.Format(time.RFC3339))
			return nil
		}
	}
	return s.SyncStations(ctx)
}

func (s *Syncer) SyncStations(ctx context.Context) error {
	s.log.Printf("syncing stations from SHMU...")
	if err := s.waitUpstream(ctx); err != nil {
		return err
	}
	raw, err := s.client.GetStations(ctx)
	if err != nil {
		return err
	}
	stations := make([]model.Station, 0, len(raw))
	for _, r := range raw {
		lat, err := shmu.ParseFloat(r.Lat)
		if err != nil {
			continue
		}
		lon, err := shmu.ParseFloat(r.Lon)
		if err != nil {
			continue
		}
		stations = append(stations, model.Station{
			ID:           r.StationID,
			Name:         r.StationName,
			Lat:          lat,
			Lon:          lon,
			DistrictCode: r.DistrictCode,
		})
	}
	if err := s.store.UpsertStations(ctx, stations); err != nil {
		return err
	}
	s.log.Printf("synced %d stations", len(stations))
	return nil
}

// SyncForecastsForRuntime downloads ALADIN products for a specific model runtime for all stations.
// Only stations missing the runtime are synced; incomplete stations are retried in-pass.
func (s *Syncer) SyncForecastsForRuntime(ctx context.Context, runtimeTS int64) error {
	pending, err := s.store.StationIDsMissingCurrentForecast(ctx, runtimeTS)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		ids, err := s.store.ListStationIDs(ctx)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return fmt.Errorf("no stations in DB")
		}
		s.log.Printf("ALADIN runtime %d already present for all %d stations", runtimeTS, len(ids))
		_ = s.store.SetMeta(ctx, "forecasts_synced_at", time.Now().UTC().Format(time.RFC3339))
		return nil
	}

	s.log.Printf("syncing ALADIN runtime %d for %d missing stations (workers=%d rate=%d/s)...",
		runtimeTS, len(pending), s.cfg.SyncWorkers, s.cfg.SyncRatePerSec)

	var okTotal, missTotal, errTotal int
	pass := 0
	for len(pending) > 0 {
		pass++
		ok, miss, errs, still := s.syncStationsParallel(ctx, pending, runtimeTS)
		okTotal += ok
		missTotal += miss
		errTotal += errs
		s.log.Printf("runtime sync pass %d: ok=%d miss=%d err=%d remaining=%d",
			pass, ok, miss, errs, len(still))
		if len(still) == 0 || len(still) == len(pending) {
			// Done, or no progress (all still missing/failing).
			pending = still
			break
		}
		pending = still
	}

	_ = s.store.SetMeta(ctx, "forecasts_synced_at", time.Now().UTC().Format(time.RFC3339))
	s.log.Printf("runtime sync done: ok=%d miss=%d err=%d remaining=%d", okTotal, missTotal, errTotal, len(pending))
	if okTotal == 0 && len(pending) > 0 {
		return fmt.Errorf("no stations synced for runtime %d (miss/err remaining=%d)", runtimeTS, len(pending))
	}
	if len(pending) > 0 {
		return fmt.Errorf("runtime %d incomplete: remaining=%d", runtimeTS, len(pending))
	}
	return nil
}

func (s *Syncer) syncStationsParallel(ctx context.Context, ids []int64, runtimeTS int64) (ok, miss, errs int, remaining []int64) {
	workers := s.cfg.SyncWorkers
	if workers < 1 {
		workers = 3
	}
	if workers > len(ids) {
		workers = len(ids)
	}

	jobs := make(chan int64)
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				var err error
				if ctx.Err() != nil {
					err = ctx.Err()
				} else if err = s.waitUpstream(ctx); err == nil {
					err = s.syncStationRuntime(ctx, id, runtimeTS)
				}
				mu.Lock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, ErrRuntimeNotReady):
					miss++
					remaining = append(remaining, id)
				default:
					errs++
					remaining = append(remaining, id)
					if errs <= 5 {
						s.log.Printf("station %d: %v", id, err)
					}
				}
				mu.Unlock()
			}
		}()
	}

	for i, id := range ids {
		select {
		case <-ctx.Done():
			mu.Lock()
			remaining = append(remaining, ids[i:]...)
			mu.Unlock()
			close(jobs)
			wg.Wait()
			return ok, miss, errs, remaining
		case jobs <- id:
		}
	}
	close(jobs)
	wg.Wait()
	return ok, miss, errs, remaining
}

func (s *Syncer) waitUpstream(ctx context.Context) error {
	if s.upstream == nil {
		return nil
	}
	return s.upstream.Wait(ctx)
}

func (s *Syncer) syncStationRuntime(ctx context.Context, stationID, runtimeTS int64) error {
	if ok, err := s.store.HasCurrentForecast(ctx, stationID, runtimeTS); err == nil && ok {
		return nil
	}

	fileLink := shmu.AladinFileLink(stationID, runtimeTS)
	rawForecast, err := s.loadOrFetchAladin(ctx, stationID, runtimeTS, fileLink)
	if err == nil {
		return s.persistForecast(ctx, stationID, runtimeTS, fileLink, rawForecast)
	}

	// Products endpoint as fallback when derived URL fails or raw payload missing.
	return s.syncStationRuntimeViaProducts(ctx, stationID, runtimeTS)
}

func (s *Syncer) syncStationRuntimeViaProducts(ctx context.Context, stationID, runtimeTS int64) error {
	rawProducts, err := s.client.GetStationProducts(ctx, stationID)
	if err != nil {
		return err
	}
	products, err := transform.ProductsToModel(rawProducts)
	if err != nil {
		return err
	}
	if err := s.store.SaveProducts(ctx, stationID, products); err != nil {
		return err
	}

	var match *model.Product
	for i := range products.Data {
		p := &products.Data[i]
		if p.Type == "aladin" && p.RuntimeTS == runtimeTS {
			match = p
			break
		}
	}
	if match == nil {
		return ErrRuntimeNotReady
	}

	rawForecast, err := s.loadOrFetchAladin(ctx, stationID, match.RuntimeTS, match.FileLink)
	if err != nil {
		return err
	}
	return s.persistForecast(ctx, stationID, match.RuntimeTS, match.FileLink, rawForecast)
}

func (s *Syncer) SyncStationForecast(ctx context.Context, stationID int64) error {
	if _, runtime, due := CurrentOrDueSlot(time.Now().UTC(), s.cfg.FetchDelayAfterPublish, DefaultAladinSlots); due && !runtime.IsZero() {
		runtimeTS := runtime.Unix()
		if ok, err := s.store.HasCurrentForecast(ctx, stationID, runtimeTS); err == nil && ok {
			return nil
		}
		fileLink := shmu.AladinFileLink(stationID, runtimeTS)
		if rawForecast, err := s.loadOrFetchAladin(ctx, stationID, runtimeTS, fileLink); err == nil {
			return s.persistForecast(ctx, stationID, runtimeTS, fileLink, rawForecast)
		}
	}

	if err := s.waitUpstream(ctx); err != nil {
		return err
	}
	rawProducts, err := s.client.GetStationProducts(ctx, stationID)
	if err != nil {
		return err
	}
	products, err := transform.ProductsToModel(rawProducts)
	if err != nil {
		return err
	}
	if err := s.store.SaveProducts(ctx, stationID, products); err != nil {
		return err
	}

	aladin, ok := transform.LatestAladinProduct(products.Data)
	if !ok {
		return nil
	}

	if ok, err := s.store.HasCurrentForecast(ctx, stationID, aladin.RuntimeTS); err == nil && ok {
		return nil
	}

	rawForecast, err := s.loadOrFetchAladin(ctx, stationID, aladin.RuntimeTS, aladin.FileLink)
	if err != nil {
		return err
	}
	return s.persistForecast(ctx, stationID, aladin.RuntimeTS, aladin.FileLink, rawForecast)
}

func (s *Syncer) loadOrFetchAladin(ctx context.Context, stationID, runtimeTS int64, fileLink string) (*shmu.AladinForecast, error) {
	if payload, err := s.store.GetForecastPayload(ctx, stationID, "aladin", runtimeTS); err == nil {
		var raw shmu.AladinForecast
		if err := json.Unmarshal(payload, &raw); err != nil {
			return nil, err
		}
		return &raw, nil
	} else if err != store.ErrNotFound {
		return nil, err
	}

	return s.client.GetAladinFile(ctx, fileLink)
}

func (s *Syncer) persistForecast(ctx context.Context, stationID, runtimeTS int64, fileLink string, raw *shmu.AladinForecast) error {
	if err := s.store.SaveForecast(ctx, stationID, "aladin", runtimeTS, fileLink, raw); err != nil {
		return err
	}
	rendered, err := transform.RenderForecast(raw, runtimeTS)
	if err != nil {
		return err
	}
	return s.store.SaveCurrentForecast(ctx, store.CurrentForecast{
		StationID:    stationID,
		RuntimeTS:    runtimeTS,
		ResponseJSON: rendered.JSON,
		ResponseGzip: rendered.Gzip,
		ETag:         rendered.ETag,
	})
}

func (s *Syncer) EnsureStationForecast(ctx context.Context, stationID int64) error {
	key := strconv.FormatInt(stationID, 10)
	_, err, _ := s.stationFlight.Do(key, func() (any, error) {
		return nil, s.SyncStationForecast(ctx, stationID)
	})
	return err
}

// LoadCurrentForecast returns the pre-rendered forecast for a station.
// Prefer an existing row (refreshing in the background when stale), then backfill
// from stored raw ALADIN data, then sync from SHMU.
func (s *Syncer) LoadCurrentForecast(ctx context.Context, stationID int64) (store.CurrentForecast, error) {
	cf, err := s.store.GetCurrentForecast(ctx, stationID)
	if err == nil {
		if s.forecastNeedsRefresh(cf) {
			s.scheduleStationRefresh(stationID)
		}
		return cf, nil
	}
	if err != store.ErrNotFound {
		return store.CurrentForecast{}, err
	}

	cf, err = s.BackfillCurrentForecastFromRaw(ctx, stationID)
	if err == nil {
		if s.forecastNeedsRefresh(cf) {
			s.scheduleStationRefresh(stationID)
		}
		return cf, nil
	}
	if err != store.ErrNotFound {
		return store.CurrentForecast{}, err
	}

	if err := s.EnsureStationForecast(ctx, stationID); err != nil {
		return store.CurrentForecast{}, err
	}
	return s.store.GetCurrentForecast(ctx, stationID)
}

func (s *Syncer) forecastNeedsRefresh(cf store.CurrentForecast) bool {
	_, runtime, due := CurrentOrDueSlot(time.Now().UTC(), s.cfg.FetchDelayAfterPublish, DefaultAladinSlots)
	if !due || runtime.IsZero() {
		return false
	}
	return cf.RuntimeTS < runtime.Unix()
}

func (s *Syncer) scheduleStationRefresh(stationID int64) {
	// Same singleflight key as EnsureStationForecast so cold + refresh coalesce.
	key := strconv.FormatInt(stationID, 10)
	go func() {
		_, _, _ = s.stationFlight.Do(key, func() (any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), s.cfg.HTTPTimeout*3)
			defer cancel()
			if err := s.SyncStationForecast(ctx, stationID); err != nil {
				s.log.Printf("background refresh station %d: %v", stationID, err)
				return nil, err
			}
			return nil, nil
		})
	}()
}

// BackfillCurrentForecastFromRaw rebuilds a pre-rendered response from a stored
// raw ALADIN payload (used when upgrading databases that lack current_forecasts).
func (s *Syncer) BackfillCurrentForecastFromRaw(ctx context.Context, stationID int64) (store.CurrentForecast, error) {
	rt, link, payload, err := s.store.LatestForecast(ctx, stationID, "aladin")
	if err != nil {
		return store.CurrentForecast{}, err
	}
	var raw shmu.AladinForecast
	if err := json.Unmarshal(payload, &raw); err != nil {
		return store.CurrentForecast{}, err
	}
	if err := s.persistForecast(ctx, stationID, rt, link, &raw); err != nil {
		return store.CurrentForecast{}, err
	}
	return s.store.GetCurrentForecast(ctx, stationID)
}
