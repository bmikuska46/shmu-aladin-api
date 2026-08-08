package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr             string
	WebURL           string // public base URL shown in docs (WEB_URL)
	DatabasePath     string
	SHMUBaseURL      string
	SHMUDataBaseURL  string
	StationsCacheTTL time.Duration
	HTTPTimeout      time.Duration
	SyncForecasts    bool // enable ALADIN publish cron + retries
	RateLimit        int
	RateLimitWindow  time.Duration

	// ALADIN publish schedule handling (SHMU: 03:45/10:45/15:45/22:45 UTC).
	FetchDelayAfterPublish time.Duration // first try after publish (default 5m)
	FetchRetryEvery        time.Duration // retry until run appears (default 5m)
	ProbeStationID         int64         // station used to detect publish readiness

	// Bulk sync concurrency (point 5): bounded workers + global upstream rate.
	SyncWorkers    int // parallel station sync workers (default 3, clamped 1–4)
	SyncRatePerSec int // max SHMU station-sync jobs per second (default 20)

	// ForecastStaleAfter marks stored forecasts as stale in derived responses.
	ForecastStaleAfter time.Duration
}

func Load() Config {
	loadDotEnv(".env")
	return Config{
		Addr:             getenv("ADDR", ":8080"),
		WebURL:           strings.TrimRight(getenv("WEB_URL", "http://localhost:8080"), "/"),
		DatabasePath:     getenv("DATABASE_PATH", "data/shmu.db"),
		SHMUBaseURL:      getenv("SHMU_BASE_URL", "https://www.shmu.sk/api/v1/nwp"),
		SHMUDataBaseURL:  getenv("SHMU_DATA_BASE_URL", "https://www.shmu.sk/data/datanwp/json"),
		StationsCacheTTL: durationEnv("STATIONS_CACHE_TTL", 7*24*time.Hour),
		HTTPTimeout:      durationEnv("HTTP_TIMEOUT", 30*time.Second),
		SyncForecasts:    boolEnv("SYNC_FORECASTS", true),
		RateLimit:        intEnv("RATE_LIMIT", 10),
		RateLimitWindow:  durationEnv("RATE_LIMIT_WINDOW", time.Minute),

		FetchDelayAfterPublish: durationEnv("FETCH_DELAY_AFTER_PUBLISH", 5*time.Minute),
		FetchRetryEvery:        durationEnv("FETCH_RETRY_EVERY", 5*time.Minute),
		ProbeStationID:         int64(intEnv("PROBE_STATION_ID", 32737)),

		SyncWorkers:    clampInt(intEnv("SYNC_WORKERS", 3), 1, 4),
		SyncRatePerSec: clampInt(intEnv("SYNC_RATE_PER_SEC", 20), 1, 100),

		ForecastStaleAfter: durationEnv("FORECAST_STALE_AFTER", 8*time.Hour),
	}
}

// loadDotEnv sets env vars from a .env file without overriding existing process env.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		_ = os.Setenv(key, val)
	}
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func intEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
