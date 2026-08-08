package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests handled, labeled by method, route pattern, and status code. No client identity.",
		},
		[]string{"method", "route", "code"},
	)
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds by method and route pattern. No client identity.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
	// Station popularity: only station ID (and how it was chosen). Never lat/lon or IP.
	apiStationRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_station_requests_total",
			Help: "Successful station resolutions by station_id, endpoint, and lookup mode (station|coords|path). Coordinates are never stored.",
		},
		[]string{"station_id", "endpoint", "lookup"},
	)
	// Requested forecast length: "full" when hours omitted; otherwise the requested positive integer.
	apiForecastHoursRequestedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_forecast_hours_requested_total",
			Help: "Forecast hour window requests. Label hours is 'full' when omitted, else the requested positive integer. No client identity.",
		},
		[]string{"hours"},
	)
	// Requested daily forecast length: "full" when days omitted; otherwise the requested positive integer.
	apiForecastDaysRequestedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_forecast_days_requested_total",
			Help: "Daily forecast day-window requests. Label days is 'full' when omitted, else the requested positive integer. No client identity.",
		},
		[]string{"days"},
	)
)

func recordStationRequest(stationID int64, endpoint, lookup string) {
	apiStationRequestsTotal.WithLabelValues(strconv.FormatInt(stationID, 10), endpoint, lookup).Inc()
}

func recordForecastHoursRequested(hours int) {
	label := "full"
	if hours > 0 {
		label = strconv.Itoa(hours)
	}
	apiForecastHoursRequestedTotal.WithLabelValues(label).Inc()
}

func recordForecastDaysRequested(days int) {
	label := "full"
	if days > 0 {
		label = strconv.Itoa(days)
	}
	apiForecastDaysRequestedTotal.WithLabelValues(label).Inc()
}

func stationLookupMode(match *model.LocationMatch) string {
	if match != nil {
		return "coords"
	}
	return "station"
}

func metricsHandler() http.Handler {
	return promhttp.Handler()
}

// withMetrics records anonymous request counters and latency. Labels are only
// method, route pattern, and status — never IP, User-Agent, or query params.
func (s *Server) withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := metricsRoute(r)
		code := strconv.Itoa(rec.status)
		httpRequestsTotal.WithLabelValues(r.Method, route, code).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
