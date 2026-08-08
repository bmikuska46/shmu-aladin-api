package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/config"
	"github.com/bmikuska/shmu-weather-api/internal/geo"
	"github.com/bmikuska/shmu-weather-api/internal/indicator"
	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/store"
	"github.com/bmikuska/shmu-weather-api/internal/syncer"
	"github.com/bmikuska/shmu-weather-api/internal/transform"
)

const maxDistanceKmCap = 500

type Server struct {
	store      *store.Store
	syncer     *syncer.Syncer
	mux        *http.ServeMux
	limiter    *routeLimiter
	webURL     string
	staleAfter time.Duration
}

func New(st *store.Store, syn *syncer.Syncer, cfg config.Config) *Server {
	s := &Server{
		store:      st,
		syncer:     syn,
		mux:        http.NewServeMux(),
		limiter:    newRouteLimiter(cfg.RateLimit, cfg.RateLimitWindow),
		webURL:     cfg.WebURL,
		staleAfter: cfg.ForecastStaleAfter,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleDocs)
	s.mux.HandleFunc("GET /docs", s.handleDocs)
	s.mux.HandleFunc("GET /en", s.handleEnglishDocs)
	s.mux.HandleFunc("GET /en/{$}", s.handleEnglishDocs)
	s.mux.HandleFunc("GET /robots.txt", s.handleRobotsTxt)
	s.mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1", s.handleAPIIndex)
	s.mux.HandleFunc("GET /api/v1/{$}", s.handleAPIIndex)
	s.mux.HandleFunc("GET /api/v1/stations", s.handleListStations)
	s.mux.HandleFunc("GET /api/v1/stations/{id}", s.handleGetStation)
	s.mux.HandleFunc("GET /api/v1/forecast", s.handleForecast)
	s.mux.HandleFunc("GET /api/v1/forecast/daily", s.handleDailyForecast)
	s.mux.HandleFunc("GET /api/v1/weather", s.handleForecast) // alias
	s.mux.HandleFunc("GET /api/v1/indicators", s.handleIndicators)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, If-None-Match")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		route := routeBucketKey(r)
		// Keep docs and health probes unbounded.
		if route != "GET /health" && route != "GET /" && route != "GET /docs" && route != "GET /en" && route != "GET /robots.txt" && route != "GET /sitemap.xml" {
			key := clientIP(r) + "|" + route
			ok, remaining, retryAfter := s.limiter.allow(key, time.Now())
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(s.limiter.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Window", s.limiter.window.String())
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeError(w, r, http.StatusTooManyRequests, "rate limit exceeded: max "+strconv.Itoa(s.limiter.limit)+" requests per route per "+s.limiter.window.String())
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	write(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.webURL, "/")
	write(w, r, http.StatusOK, map[string]any{
		"name":          "SHMU ALADIN API",
		"version":       "v1",
		"base_url":      base + "/api/v1",
		"documentation": base + "/",
		"endpoints": []map[string]string{
			{
				"method":      "GET",
				"path":        "/api/v1/stations",
				"description": "Paginated station list with search",
			},
			{
				"method":      "GET",
				"path":        "/api/v1/stations/{id}",
				"description": "Single station by ID",
			},
			{
				"method":      "GET",
				"path":        "/api/v1/forecast?station={id}|lat={lat}&lon={lon}",
				"description": "Latest ALADIN forecast for the next 3 days (by station ID or nearest station to coordinates)",
			},
			{
				"method":      "GET",
				"path":        "/api/v1/forecast/daily?station={id}|lat={lat}&lon={lon}",
				"description": "Daily summaries aggregated from the hourly ALADIN forecast",
			},
			{
				"method":      "GET",
				"path":        "/api/v1/weather?station={id}|lat={lat}&lon={lon}",
				"description": "Alias of /api/v1/forecast",
			},
			{
				"method":      "GET",
				"path":        "/api/v1/indicators?station={id}|lat={lat}&lon={lon}",
				"description": "API-derived non-official weather indicators from the hourly ALADIN forecast",
			},
		},
	})
}

func (s *Server) handleListStations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = r.URL.Query().Get("search")
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	list, err := s.store.ListStations(r.Context(), q, page, pageSize)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to list stations")
		return
	}
	write(w, r, http.StatusOK, list)
}

func (s *Server) handleGetStation(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveInt(r.PathValue("id"))
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid station id")
		return
	}

	st, err := s.store.GetStation(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "station not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to get station")
		return
	}
	write(w, r, http.StatusOK, st)
}

func (s *Server) handleForecast(w http.ResponseWriter, r *http.Request) {
	res, status, msg := s.resolveLocation(r)
	if status != 0 {
		writeError(w, r, status, msg)
		return
	}

	exists, err := s.store.StationExists(r.Context(), res.StationID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to validate station")
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "station not found")
		return
	}

	cf, err := s.syncer.LoadCurrentForecast(r.Context(), res.StationID)
	if err != nil {
		writeError(w, r, http.StatusBadGateway, "failed to load forecast")
		return
	}

	cnt, ok, cntMsg := parseOptionalCnt(r.URL.Query().Get("cnt"))
	if !ok {
		writeError(w, r, http.StatusBadRequest, cntMsg)
		return
	}

	needsRewrite := res.Match != nil || cnt > 0
	if needsRewrite {
		var resp model.ForecastResponse
		if err := json.Unmarshal(cf.ResponseJSON, &resp); err != nil {
			writeError(w, r, http.StatusInternalServerError, "corrupt forecast cache")
			return
		}
		if cnt > 0 && cnt < resp.Cnt {
			resp.List = resp.List[:cnt]
			resp.Cnt = cnt
		}
		if res.Match != nil {
			res.Match.MatchedElevation = resp.City.Elevation
			// Attach as a top-level extension via a wrapper map to avoid changing
			// the stored ForecastResponse type used by station-ID cache hits.
			payload := forecastWithMatch{ForecastResponse: resp, LocationMatch: res.Match}
			etag := etagFor(payload)
			if checkConditional(w, r, etag) {
				return
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.Header().Set("X-Cache", "STORE")
			write(w, r, http.StatusOK, payload)
			return
		}
		etag := etagFor(resp)
		if checkConditional(w, r, etag) {
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("X-Cache", "STORE")
		write(w, r, http.StatusOK, resp)
		return
	}

	if checkConditional(w, r, cf.ETag) {
		return
	}
	w.Header().Set("ETag", cf.ETag)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Cache", "STORE")

	if acceptsGzip(r) && len(cf.ResponseGzip) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cf.ResponseGzip)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cf.ResponseJSON)
}

type forecastWithMatch struct {
	model.ForecastResponse
	LocationMatch *model.LocationMatch `json:"location_match"`
}

func (s *Server) handleDailyForecast(w http.ResponseWriter, r *http.Request) {
	res, status, msg := s.resolveLocation(r)
	if status != 0 {
		writeError(w, r, status, msg)
		return
	}

	days, ok, daysMsg := parseOptionalDays(r.URL.Query().Get("days"))
	if !ok {
		writeError(w, r, http.StatusBadRequest, daysMsg)
		return
	}

	hourly, cf, err := s.loadHourlyForecast(r, res.StationID)
	if err != nil {
		writeForecastLoadError(w, r, err)
		return
	}

	availableDays := countLocalDays(hourly.List)
	if days > 0 && days > availableDays {
		writeError(w, r, http.StatusBadRequest, fmt.Sprintf("query parameter 'days' exceeds available forecast horizon (%d day(s))", availableDays))
		return
	}

	resp := transform.AggregateDaily(hourly, days)
	resp.Source = s.sourceMeta(cf)
	if res.Match != nil {
		res.Match.MatchedElevation = hourly.City.Elevation
		resp.LocationMatch = res.Match
	}

	etag := etagFor(resp)
	if checkConditional(w, r, etag) {
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Cache", "STORE")
	write(w, r, http.StatusOK, resp)
}

func (s *Server) handleIndicators(w http.ResponseWriter, r *http.Request) {
	res, status, msg := s.resolveLocation(r)
	if status != 0 {
		writeError(w, r, status, msg)
		return
	}

	types := parseTypeFilter(r.URL.Query().Get("type"))
	hourly, cf, err := s.loadHourlyForecast(r, res.StationID)
	if err != nil {
		writeForecastLoadError(w, r, err)
		return
	}

	list, err := indicator.Evaluate(hourly, types)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	resp := model.IndicatorsResponse{
		Code:    "200",
		Message: 0,
		Cnt:     len(list),
		List:    list,
		City:    hourly.City,
		Meta:    hourly.Meta,
		Source:  s.sourceMeta(cf),
	}
	if res.Match != nil {
		res.Match.MatchedElevation = hourly.City.Elevation
		resp.LocationMatch = res.Match
	}

	etag := etagFor(resp)
	if checkConditional(w, r, etag) {
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("X-Cache", "STORE")
	write(w, r, http.StatusOK, resp)
}

func (s *Server) loadHourlyForecast(r *http.Request, stationID int64) (model.ForecastResponse, store.CurrentForecast, error) {
	exists, err := s.store.StationExists(r.Context(), stationID)
	if err != nil {
		return model.ForecastResponse{}, store.CurrentForecast{}, err
	}
	if !exists {
		return model.ForecastResponse{}, store.CurrentForecast{}, store.ErrNotFound
	}
	cf, err := s.syncer.LoadCurrentForecast(r.Context(), stationID)
	if err != nil {
		return model.ForecastResponse{}, store.CurrentForecast{}, err
	}
	var resp model.ForecastResponse
	if err := json.Unmarshal(cf.ResponseJSON, &resp); err != nil {
		return model.ForecastResponse{}, store.CurrentForecast{}, fmt.Errorf("corrupt forecast cache: %w", err)
	}
	return resp, cf, nil
}

func writeForecastLoadError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "station not found")
		return
	}
	if strings.Contains(err.Error(), "corrupt") {
		writeError(w, r, http.StatusInternalServerError, "corrupt forecast cache")
		return
	}
	writeError(w, r, http.StatusBadGateway, "failed to load forecast")
}

func (s *Server) sourceMeta(cf store.CurrentForecast) model.SourceMeta {
	fresh := geo.ForecastFreshness(cf.FetchedAt, time.Now().UTC(), s.staleAfter)
	runtime := ""
	if cf.RuntimeTS > 0 {
		runtime = time.Unix(cf.RuntimeTS, 0).UTC().Format(time.RFC3339)
	}
	fetched := ""
	if !fresh.FetchedAt.IsZero() {
		fetched = fresh.FetchedAt.Format(time.RFC3339)
	}
	return model.SourceMeta{
		Source:     "SHMU ALADIN",
		Kind:       "forecast",
		Runtime:    runtime,
		FetchedAt:  fetched,
		AgeSeconds: fresh.AgeSeconds,
		IsStale:    fresh.IsStale,
	}
}

// locationResult is the resolved station for a forecast-style request.
type locationResult struct {
	StationID int64
	Match     *model.LocationMatch
}

func (s *Server) resolveLocation(r *http.Request) (locationResult, int, string) {
	q := r.URL.Query()
	hasStation := q.Has("station")
	hasLat := q.Has("lat") || q.Has("latitude")
	hasLon := q.Has("lon") || q.Has("longitude")

	if q.Has("elevation") || q.Has("alt") || q.Has("altitude") {
		return locationResult{}, http.StatusBadRequest, "elevation is not supported; forecasts are not elevation-corrected"
	}

	maxDist, hasMaxDist, maxMsg := parseMaxDistanceKm(q.Get("max_distance_km"))
	if maxMsg != "" {
		return locationResult{}, http.StatusBadRequest, maxMsg
	}

	if hasStation && (hasLat || hasLon) {
		return locationResult{}, http.StatusBadRequest, "provide either 'station' or 'lat' and 'lon', not both"
	}
	if hasStation {
		if hasMaxDist {
			return locationResult{}, http.StatusBadRequest, "query parameter 'max_distance_km' is only valid with lat/lon"
		}
		id, ok, msg := parseStationQuery(q.Get("station"))
		if !ok {
			return locationResult{}, http.StatusBadRequest, msg
		}
		return locationResult{StationID: id}, 0, ""
	}
	if hasLat || hasLon {
		if !hasLat || !hasLon {
			return locationResult{}, http.StatusBadRequest, "query parameters 'lat' and 'lon' are both required"
		}
		lat, ok, msg := parseCoord(firstNonEmpty(q.Get("lat"), q.Get("latitude")), "lat", -90, 90)
		if !ok {
			return locationResult{}, http.StatusBadRequest, msg
		}
		lon, ok, msg := parseCoord(firstNonEmpty(q.Get("lon"), q.Get("longitude")), "lon", -180, 180)
		if !ok {
			return locationResult{}, http.StatusBadRequest, msg
		}
		nearest, err := s.store.FindNearestStation(r.Context(), lat, lon)
		if errors.Is(err, store.ErrNotFound) {
			return locationResult{}, http.StatusNotFound, "no stations available"
		}
		if err != nil {
			return locationResult{}, http.StatusInternalServerError, "failed to find nearest station"
		}
		if hasMaxDist && nearest.DistanceKm > maxDist {
			return locationResult{}, http.StatusNotFound, fmt.Sprintf(
				"nearest station is %.1f km away (max_distance_km=%.1f)", nearest.DistanceKm, maxDist)
		}
		match := &model.LocationMatch{
			Method:             "nearest_aladin_station",
			RequestedCoord:     model.Coord{Lat: lat, Lon: lon},
			MatchedCoord:       model.Coord{Lat: nearest.Station.Lat, Lon: nearest.Station.Lon},
			DistanceKm:         nearest.DistanceKm,
			MatchedStationID:   nearest.Station.ID,
			MatchedStationName: nearest.Station.Name,
		}
		return locationResult{StationID: nearest.Station.ID, Match: match}, 0, ""
	}
	return locationResult{}, http.StatusBadRequest, "query parameter 'station' or both 'lat' and 'lon' is required"
}

// resolveForecastStation kept for older tests; wraps resolveLocation.
func (s *Server) resolveForecastStation(r *http.Request) (int64, int, string) {
	res, status, msg := s.resolveLocation(r)
	return res.StationID, status, msg
}

func parseStationQuery(raw string) (int64, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, "query parameter 'station' must be a valid station id"
	}
	id, ok := parsePositiveInt(raw)
	if !ok {
		return 0, false, "query parameter 'station' must be a valid station id"
	}
	return id, true, ""
}

func parseCoord(raw, name string, min, max float64) (float64, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, "query parameter '" + name + "' must be a valid number"
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, "query parameter '" + name + "' must be a valid number"
	}
	if v < min || v > max {
		return 0, false, fmt.Sprintf("query parameter '%s' must be between %g and %g", name, min, max)
	}
	return v, true, ""
}

func parseMaxDistanceKm(raw string) (float64, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, ""
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, false, "query parameter 'max_distance_km' must be a positive number"
	}
	if v > maxDistanceKmCap {
		return 0, false, fmt.Sprintf("query parameter 'max_distance_km' must be at most %d", maxDistanceKmCap)
	}
	return v, true, ""
}

func parseOptionalCnt(raw string) (int, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true, ""
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false, "query parameter 'cnt' must be a positive integer"
	}
	return n, true, ""
}

func parseOptionalDays(raw string) (int, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true, ""
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false, "query parameter 'days' must be a positive integer"
	}
	return n, true, ""
}

func parseTypeFilter(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func countLocalDays(list []model.ForecastItem) int {
	seen := map[string]bool{}
	for _, it := range list {
		seen[geo.LocalDateString(it.DT)] = true
	}
	return len(seen)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parsePositiveInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		encoding := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if strings.EqualFold(encoding, "gzip") {
			return true
		}
	}
	return false
}

func checkConditional(w http.ResponseWriter, r *http.Request, etag string) bool {
	inm := strings.TrimSpace(r.Header.Get("If-None-Match"))
	if inm == "" || etag == "" {
		return false
	}
	for _, part := range strings.Split(inm, ",") {
		if strings.TrimSpace(part) == etag {
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.WriteHeader(http.StatusNotModified)
			return true
		}
	}
	return false
}

func etagFor(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

func write(w http.ResponseWriter, r *http.Request, status int, v any) {
	_ = r
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	write(w, r, status, model.APIError{Code: status, Message: msg})
}
