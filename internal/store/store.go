package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/normalize"

	_ "modernc.org/sqlite"
)

// maxSearchQueryRunes caps user search input to bound FTS/LIKE work.
const maxSearchQueryRunes = 100

var ErrNotFound = errors.New("not found")

// ForecastRuntimesToKeep is how many raw ALADIN runtimes to retain per station.
// Current + previous supports rollback; older runtimes are pruned to bound DB growth.
const ForecastRuntimesToKeep = 2

// SQLite pool sizing for a low-RAM single instance: one writer, two readers.
const (
	writerMaxConns = 1
	readerMaxConns = 2
	busyTimeoutMS  = 5000
	// Negative cache_size is kibibytes → 2 MiB page cache per connection.
	pageCacheKiB = 2048
)

type Store struct {
	write *sql.DB
	read  *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	writeDB, err := openSQLite(path, false, writerMaxConns)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}

	s := &Store{write: writeDB}
	if err := s.migrate(); err != nil {
		_ = writeDB.Close()
		return nil, err
	}

	readDB, err := openSQLite(path, true, readerMaxConns)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	s.read = readDB
	return s, nil
}

func openSQLite(path string, readOnly bool, maxConns int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteDSN(path, readOnly))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	db.SetConnMaxLifetime(0)

	// Force a connection so DSN pragmas are applied (and fail fast).
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func sqliteDSN(path string, readOnly bool) string {
	q := url.Values{}
	q.Set("_busy_timeout", fmt.Sprintf("%d", busyTimeoutMS))
	q.Set("_foreign_keys", "on")
	q.Add("_pragma", fmt.Sprintf("cache_size(-%d)", pageCacheKiB))
	if readOnly {
		q.Set("mode", "ro")
		q.Set("_query_only", "true")
	} else {
		q.Set("_journal_mode", "WAL")
		q.Set("_synchronous", "NORMAL")
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

func (s *Store) Close() error {
	var err error
	if s.read != nil {
		err = s.read.Close()
	}
	if s.write != nil {
		if e := s.write.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS stations (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  name_folded TEXT NOT NULL,
  lat REAL NOT NULL,
  lon REAL NOT NULL,
  district_code INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_stations_name_folded ON stations(name_folded);

CREATE VIRTUAL TABLE IF NOT EXISTS stations_fts USING fts5(
  name_folded,
  content='stations',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS stations_ai AFTER INSERT ON stations BEGIN
  INSERT INTO stations_fts(rowid, name_folded) VALUES (new.id, new.name_folded);
END;
CREATE TRIGGER IF NOT EXISTS stations_ad AFTER DELETE ON stations BEGIN
  INSERT INTO stations_fts(stations_fts, rowid, name_folded) VALUES('delete', old.id, old.name_folded);
END;
CREATE TRIGGER IF NOT EXISTS stations_au AFTER UPDATE ON stations BEGIN
  INSERT INTO stations_fts(stations_fts, rowid, name_folded) VALUES('delete', old.id, old.name_folded);
  INSERT INTO stations_fts(rowid, name_folded) VALUES (new.id, new.name_folded);
END;

CREATE TABLE IF NOT EXISTS sync_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS station_products (
  station_id INTEGER PRIMARY KEY,
  payload_json TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  FOREIGN KEY(station_id) REFERENCES stations(id)
);

CREATE TABLE IF NOT EXISTS forecasts (
  station_id INTEGER NOT NULL,
  product_type TEXT NOT NULL,
  runtime_ts INTEGER NOT NULL,
  file_link TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (station_id, product_type, runtime_ts),
  FOREIGN KEY(station_id) REFERENCES stations(id)
);

CREATE INDEX IF NOT EXISTS idx_forecasts_station_type ON forecasts(station_id, product_type, runtime_ts DESC);

CREATE TABLE IF NOT EXISTS current_forecasts (
  station_id INTEGER PRIMARY KEY,
  runtime_ts INTEGER NOT NULL,
  response_json BLOB NOT NULL,
  response_gzip BLOB NOT NULL,
  etag TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  FOREIGN KEY(station_id) REFERENCES stations(id)
);
`
	if _, err := s.write.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := s.pruneAllOldForecasts(context.Background(), ForecastRuntimesToKeep); err != nil {
		return fmt.Errorf("migrate prune forecasts: %w", err)
	}
	return nil
}

// EnsureForecastRenderVersion clears cached rendered forecasts when the transform
// version changes so responses are rebuilt from stored raw ALADIN payloads.
func (s *Store) EnsureForecastRenderVersion(ctx context.Context, version string) error {
	cur, ok, err := s.GetMeta(ctx, "forecast_render_version")
	if err != nil {
		return err
	}
	if ok && cur == version {
		return nil
	}
	if err := s.ClearCurrentForecasts(ctx); err != nil {
		return err
	}
	return s.SetMeta(ctx, "forecast_render_version", version)
}

func (s *Store) UpsertStations(ctx context.Context, stations []model.Station) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO stations (id, name, name_folded, lat, lon, district_code, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name,
  name_folded=excluded.name_folded,
  lat=excluded.lat,
  lon=excluded.lon,
  district_code=excluded.district_code,
  updated_at=excluded.updated_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, st := range stations {
		_, err := stmt.ExecContext(ctx,
			st.ID, st.Name, normalize.Fold(st.Name), st.Lat, st.Lon, st.DistrictCode, now,
		)
		if err != nil {
			return err
		}
	}

	if err := setMetaTx(ctx, tx, "stations_synced_at", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) StationsSyncedAt(ctx context.Context) (time.Time, bool, error) {
	return s.getMetaTime(ctx, "stations_synced_at")
}

func (s *Store) ListStations(ctx context.Context, query string, page, pageSize int) (model.StationList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize
	query = truncateRunes(query, maxSearchQueryRunes)
	folded := normalize.Fold(query)

	var (
		total int
		rows  *sql.Rows
		err   error
	)

	if folded == "" {
		err = s.read.QueryRowContext(ctx, `SELECT COUNT(*) FROM stations`).Scan(&total)
		if err != nil {
			return model.StationList{}, err
		}
		rows, err = s.read.QueryContext(ctx, `
SELECT id, name, lat, lon, district_code
FROM stations
ORDER BY name COLLATE NOCASE
LIMIT ? OFFSET ?`, pageSize, offset)
	} else {
		// Prefer FTS match; fall back to LIKE on folded name for partial substrings.
		// Both use bound parameters; FTS tokens are quoted/sanitized and LIKE
		// wildcards in user input are escaped so they cannot alter query semantics.
		ftsQuery := buildFTSQuery(folded)
		likePattern := "%" + escapeLike(folded) + "%"
		err = s.read.QueryRowContext(ctx, `
SELECT COUNT(*) FROM (
  SELECT s.id FROM stations s
  JOIN stations_fts f ON f.rowid = s.id
  WHERE stations_fts MATCH ?
  UNION
  SELECT id FROM stations WHERE name_folded LIKE ? ESCAPE '\'
)`, ftsQuery, likePattern).Scan(&total)
		if err != nil {
			return model.StationList{}, err
		}
		rows, err = s.read.QueryContext(ctx, `
SELECT id, name, lat, lon, district_code FROM (
  SELECT s.id, s.name, s.lat, s.lon, s.district_code, s.name_folded
  FROM stations s
  JOIN stations_fts f ON f.rowid = s.id
  WHERE stations_fts MATCH ?
  UNION
  SELECT id, name, lat, lon, district_code, name_folded
  FROM stations WHERE name_folded LIKE ? ESCAPE '\'
)
ORDER BY name COLLATE NOCASE
LIMIT ? OFFSET ?`, ftsQuery, likePattern, pageSize, offset)
	}
	if err != nil {
		return model.StationList{}, err
	}
	defer rows.Close()

	stations := make([]model.Station, 0, pageSize)
	for rows.Next() {
		var st model.Station
		if err := rows.Scan(&st.ID, &st.Name, &st.Lat, &st.Lon, &st.DistrictCode); err != nil {
			return model.StationList{}, err
		}
		stations = append(stations, st)
	}
	if err := rows.Err(); err != nil {
		return model.StationList{}, err
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if total == 0 {
		totalPages = 0
	}

	return model.StationList{
		Stations:   stations,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		Query:      query,
	}, nil
}

// buildFTSQuery turns folded user text into a safe FTS5 MATCH expression.
// Each token is restricted to letters/digits/hyphen/apostrophe and wrapped in
// double quotes so FTS operators (AND/OR/NOT/NEAR/:, *, ^, etc.) stay literal.
func buildFTSQuery(folded string) string {
	parts := strings.Fields(folded)
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = sanitizeFTSToken(p)
		if p == "" {
			continue
		}
		quoted = append(quoted, `"`+p+`"*`)
	}
	if len(quoted) == 0 {
		// Match nothing: empty quoted phrase with prefix is invalid for useful
		// hits and avoids passing raw user text into MATCH.
		return `""`
	}
	return strings.Join(quoted, " ")
}

func sanitizeFTSToken(p string) string {
	var b strings.Builder
	b.Grow(len(p))
	for _, r := range p {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '-', r == '\'':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeLike escapes \, %, and _ so user input cannot inject LIKE wildcards.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	b.Grow(max)
	n := 0
	for _, r := range s {
		if n >= max {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func (s *Store) GetStation(ctx context.Context, id int64) (model.Station, error) {
	var st model.Station
	err := s.read.QueryRowContext(ctx, `
SELECT id, name, lat, lon, district_code FROM stations WHERE id = ?`, id).
		Scan(&st.ID, &st.Name, &st.Lat, &st.Lon, &st.DistrictCode)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Station{}, ErrNotFound
	}
	return st, err
}

func (s *Store) StationExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.read.QueryRowContext(ctx, `SELECT 1 FROM stations WHERE id = ?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// NearestStation is a station plus its Haversine distance from a query point.
type NearestStation struct {
	Station    model.Station
	DistanceKm float64
}

// FindNearestStation returns the station closest to the given WGS84 coordinates.
// Distance is exact Haversine (km). Equal distances break ties by lower station ID.
func (s *Store) FindNearestStation(ctx context.Context, lat, lon float64) (NearestStation, error) {
	rows, err := s.read.QueryContext(ctx, `
SELECT id, name, lat, lon, district_code FROM stations`)
	if err != nil {
		return NearestStation{}, err
	}
	defer rows.Close()

	found := false
	var best NearestStation
	for rows.Next() {
		var st model.Station
		if err := rows.Scan(&st.ID, &st.Name, &st.Lat, &st.Lon, &st.DistrictCode); err != nil {
			return NearestStation{}, err
		}
		d := haversineKm(lat, lon, st.Lat, st.Lon)
		if !found || d < best.DistanceKm-1e-12 || (nearlyEqual(d, best.DistanceKm) && st.ID < best.Station.ID) {
			best = NearestStation{Station: st, DistanceKm: d}
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		return NearestStation{}, err
	}
	if !found {
		return NearestStation{}, ErrNotFound
	}
	best.DistanceKm = math.Round(best.DistanceKm*10) / 10
	return best, nil
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

// ClearCurrentForecasts deletes all pre-rendered forecast rows (used when render semantics change).
func (s *Store) ClearCurrentForecasts(ctx context.Context) error {
	_, err := s.write.ExecContext(ctx, `DELETE FROM current_forecasts`)
	return err
}

func (s *Store) SaveProducts(ctx context.Context, stationID int64, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.write.ExecContext(ctx, `
INSERT INTO station_products (station_id, payload_json, fetched_at)
VALUES (?, ?, ?)
ON CONFLICT(station_id) DO UPDATE SET
  payload_json=excluded.payload_json,
  fetched_at=excluded.fetched_at
`, stationID, string(b), now)
	return err
}

func (s *Store) GetProductsJSON(ctx context.Context, stationID int64) ([]byte, time.Time, error) {
	var raw string
	var fetched string
	err := s.read.QueryRowContext(ctx, `
SELECT payload_json, fetched_at FROM station_products WHERE station_id = ?`, stationID).
		Scan(&raw, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	t, _ := time.Parse(time.RFC3339, fetched)
	return []byte(raw), t, nil
}

func (s *Store) SaveForecast(ctx context.Context, stationID int64, productType string, runtimeTS int64, fileLink string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
INSERT INTO forecasts (station_id, product_type, runtime_ts, file_link, payload_json, fetched_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(station_id, product_type, runtime_ts) DO UPDATE SET
  file_link=excluded.file_link,
  payload_json=excluded.payload_json,
  fetched_at=excluded.fetched_at
`, stationID, productType, runtimeTS, fileLink, string(b), now)
	if err != nil {
		return err
	}
	if err := pruneOldForecastsTx(ctx, tx, stationID, productType, ForecastRuntimesToKeep); err != nil {
		return err
	}
	return tx.Commit()
}

// pruneOldForecastsTx deletes raw forecast rows older than the N newest runtimes
// for a station/product pair. keep <= 0 means delete all.
func pruneOldForecastsTx(ctx context.Context, tx *sql.Tx, stationID int64, productType string, keep int) error {
	if keep <= 0 {
		_, err := tx.ExecContext(ctx, `
DELETE FROM forecasts WHERE station_id = ? AND product_type = ?`, stationID, productType)
		return err
	}
	// SQLite: delete all but the keep newest via OFFSET.
	_, err := tx.ExecContext(ctx, `
DELETE FROM forecasts
WHERE rowid IN (
  SELECT rowid FROM forecasts
  WHERE station_id = ? AND product_type = ?
  ORDER BY runtime_ts DESC
  LIMIT -1 OFFSET ?
)`, stationID, productType, keep)
	return err
}

// pruneAllOldForecasts removes unbounded forecast history across all stations,
// retaining only the newest `keep` runtimes per (station_id, product_type).
func (s *Store) pruneAllOldForecasts(ctx context.Context, keep int) error {
	if keep <= 0 {
		_, err := s.write.ExecContext(ctx, `DELETE FROM forecasts`)
		return err
	}
	_, err := s.write.ExecContext(ctx, `
DELETE FROM forecasts
WHERE rowid IN (
  SELECT f.rowid
  FROM forecasts f
  WHERE (
    SELECT COUNT(*)
    FROM forecasts newer
    WHERE newer.station_id = f.station_id
      AND newer.product_type = f.product_type
      AND newer.runtime_ts > f.runtime_ts
  ) >= ?
)`, keep)
	return err
}

func (s *Store) LatestForecast(ctx context.Context, stationID int64, productType string) (runtimeTS int64, fileLink string, payload []byte, err error) {
	var raw string
	err = s.read.QueryRowContext(ctx, `
SELECT runtime_ts, file_link, payload_json
FROM forecasts
WHERE station_id = ? AND product_type = ?
ORDER BY runtime_ts DESC
LIMIT 1`, stationID, productType).Scan(&runtimeTS, &fileLink, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil, ErrNotFound
	}
	if err != nil {
		return 0, "", nil, err
	}
	return runtimeTS, fileLink, []byte(raw), nil
}

func (s *Store) HasForecast(ctx context.Context, stationID int64, productType string, runtimeTS int64) (bool, error) {
	var n int
	err := s.read.QueryRowContext(ctx, `
SELECT 1 FROM forecasts
WHERE station_id = ? AND product_type = ? AND runtime_ts = ?
LIMIT 1`, stationID, productType, runtimeTS).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CurrentForecast holds a pre-rendered API forecast response.
type CurrentForecast struct {
	StationID    int64
	RuntimeTS    int64
	ResponseJSON []byte
	ResponseGzip []byte
	ETag         string
	FetchedAt    int64
}

func (s *Store) SaveCurrentForecast(ctx context.Context, cf CurrentForecast) error {
	if cf.FetchedAt == 0 {
		cf.FetchedAt = time.Now().UTC().Unix()
	}
	_, err := s.write.ExecContext(ctx, `
INSERT INTO current_forecasts (station_id, runtime_ts, response_json, response_gzip, etag, fetched_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(station_id) DO UPDATE SET
  runtime_ts=excluded.runtime_ts,
  response_json=excluded.response_json,
  response_gzip=excluded.response_gzip,
  etag=excluded.etag,
  fetched_at=excluded.fetched_at
`, cf.StationID, cf.RuntimeTS, cf.ResponseJSON, cf.ResponseGzip, cf.ETag, cf.FetchedAt)
	return err
}

func (s *Store) GetCurrentForecast(ctx context.Context, stationID int64) (CurrentForecast, error) {
	var cf CurrentForecast
	err := s.read.QueryRowContext(ctx, `
SELECT station_id, runtime_ts, response_json, response_gzip, etag, fetched_at
FROM current_forecasts WHERE station_id = ?`, stationID).
		Scan(&cf.StationID, &cf.RuntimeTS, &cf.ResponseJSON, &cf.ResponseGzip, &cf.ETag, &cf.FetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CurrentForecast{}, ErrNotFound
	}
	return cf, err
}

func (s *Store) HasCurrentForecast(ctx context.Context, stationID, runtimeTS int64) (bool, error) {
	var n int
	err := s.read.QueryRowContext(ctx, `
SELECT 1 FROM current_forecasts
WHERE station_id = ? AND runtime_ts = ?
LIMIT 1`, stationID, runtimeTS).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) GetForecastPayload(ctx context.Context, stationID int64, productType string, runtimeTS int64) ([]byte, error) {
	var raw string
	err := s.read.QueryRowContext(ctx, `
SELECT payload_json FROM forecasts
WHERE station_id = ? AND product_type = ? AND runtime_ts = ?
LIMIT 1`, stationID, productType, runtimeTS).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}

// ListForecastRuntimes returns stored runtime timestamps for a station/product,
// newest first. Intended for tests and diagnostics.
func (s *Store) ListForecastRuntimes(ctx context.Context, stationID int64, productType string) ([]int64, error) {
	rows, err := s.read.QueryContext(ctx, `
SELECT runtime_ts FROM forecasts
WHERE station_id = ? AND product_type = ?
ORDER BY runtime_ts DESC`, stationID, productType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

func (s *Store) ListStationIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT id FROM stations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// StationIDsMissingCurrentForecast returns stations without a pre-rendered
// forecast for the given ALADIN runtime (used to retry only unfinished work).
func (s *Store) StationIDsMissingCurrentForecast(ctx context.Context, runtimeTS int64) ([]int64, error) {
	rows, err := s.read.QueryContext(ctx, `
SELECT s.id FROM stations s
WHERE NOT EXISTS (
  SELECT 1 FROM current_forecasts c
  WHERE c.station_id = s.id AND c.runtime_ts = ?
)
ORDER BY s.id`, runtimeTS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.write.ExecContext(ctx, `
INSERT INTO sync_meta (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at
`, key, value, now)
	return err
}

func (s *Store) GetMeta(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.read.QueryRowContext(ctx, `SELECT value FROM sync_meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func setMetaTx(ctx context.Context, tx *sql.Tx, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx, `
INSERT INTO sync_meta (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at
`, key, value, now)
	return err
}

func (s *Store) getMetaTime(ctx context.Context, key string) (time.Time, bool, error) {
	var v string
	err := s.read.QueryRowContext(ctx, `SELECT value FROM sync_meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}
