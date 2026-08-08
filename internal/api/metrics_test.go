package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/config"
	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func TestMetricsRouteAnonymizesPathParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stations/32737?q=secret", nil)
	got := metricsRoute(req)
	if got != "/api/v1/stations/{id}" {
		t.Fatalf("route=%q want /api/v1/stations/{id}", got)
	}
	if strings.Contains(got, "32737") || strings.Contains(got, "secret") {
		t.Fatalf("route leaked identity data: %q", got)
	}
}

func TestMetricsRouteUnknownIsLowCardinality(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist/foo", nil)
	got := metricsRoute(req)
	if got != "unmatched" {
		t.Fatalf("route=%q want unmatched", got)
	}
}

func TestMetricsRecordsWithoutClientLabels(t *testing.T) {
	srv := New(nil, nil, config.Config{
		WebURL:          "https://weather.example/",
		RateLimit:       100,
		RateLimitWindow: time.Minute,
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("User-Agent", "test-agent/1.0")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metricsRec, metricsReq)
	body := metricsRec.Body.String()

	if !strings.Contains(body, `http_requests_total{code="200",method="GET",route="/health"}`) {
		t.Fatalf("expected anonymized counter in metrics output:\n%s", body)
	}
	for _, bad := range []string{"203.0.113.9", "test-agent", "X-Forwarded-For", "User-Agent"} {
		if strings.Contains(body, bad) {
			t.Fatalf("metrics leaked %q", bad)
		}
	}
}

func TestStationAndHoursMetricsAreAnonymous(t *testing.T) {
	recordStationRequest(32737, "forecast", "coords")
	recordStationRequest(32737, "forecast", "station")
	recordForecastHoursRequested(0)
	recordForecastHoursRequested(6)
	recordForecastDaysRequested(0)
	recordForecastDaysRequested(2)

	srv := New(nil, nil, config.Config{
		WebURL:          "https://weather.example/",
		RateLimit:       100,
		RateLimitWindow: time.Minute,
	})
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metricsRec, metricsReq)
	body := metricsRec.Body.String()

	want := []string{
		`api_station_requests_total{endpoint="forecast",lookup="coords",station_id="32737"}`,
		`api_station_requests_total{endpoint="forecast",lookup="station",station_id="32737"}`,
		`api_forecast_hours_requested_total{hours="full"}`,
		`api_forecast_hours_requested_total{hours="6"}`,
		`api_forecast_days_requested_total{days="full"}`,
		`api_forecast_days_requested_total{days="2"}`,
	}
	for _, line := range want {
		if !strings.Contains(body, line) {
			t.Fatalf("missing %s in:\n%s", line, body)
		}
	}
	for _, bad := range []string{"48.15", "17.10", "lat=", "lon="} {
		if strings.Contains(body, bad) {
			t.Fatalf("metrics leaked coordinates-like data %q", bad)
		}
	}
}

func TestStationLookupMode(t *testing.T) {
	if got := stationLookupMode(nil); got != "station" {
		t.Fatalf("nil match: %q", got)
	}
	if got := stationLookupMode(&model.LocationMatch{}); got != "coords" {
		t.Fatalf("match: %q", got)
	}
}

