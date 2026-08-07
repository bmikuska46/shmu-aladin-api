package api

import (
	"encoding/json"
	// "encoding/xml" // XML responses disabled for now; re-enable with wantsXML/write below
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/config"
	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/store"
	"github.com/bmikuska/shmu-weather-api/internal/syncer"
)

type Server struct {
	store   *store.Store
	syncer  *syncer.Syncer
	mux     *http.ServeMux
	limiter *routeLimiter
	webURL  string
}

func New(st *store.Store, syn *syncer.Syncer, cfg config.Config) *Server {
	s := &Server{
		store:   st,
		syncer:  syn,
		mux:     http.NewServeMux(),
		limiter: newRouteLimiter(cfg.RateLimit, cfg.RateLimitWindow),
		webURL:  cfg.WebURL,
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
	s.mux.HandleFunc("GET /api/v1/stations", s.handleListStations)
	s.mux.HandleFunc("GET /api/v1/stations/{id}", s.handleGetStation)
	s.mux.HandleFunc("GET /api/v1/forecast", s.handleForecast)
	s.mux.HandleFunc("GET /api/v1/weather", s.handleForecast) // alias
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
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
	stationID, ok, msg := requireStationQuery(r)
	if !ok {
		writeError(w, r, http.StatusBadRequest, msg)
		return
	}

	exists, err := s.store.StationExists(r.Context(), stationID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to validate station")
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "station not found")
		return
	}

	cf, err := s.syncer.LoadCurrentForecast(r.Context(), stationID)
	if err != nil {
		writeError(w, r, http.StatusBadGateway, "failed to load forecast")
		return
	}

	cntStr := r.URL.Query().Get("cnt")
	if cntStr != "" {
		if cnt, err := strconv.Atoi(cntStr); err == nil && cnt > 0 {
			s.writeForecastWithCnt(w, r, cf, cnt)
			return
		}
	}

	w.Header().Set("ETag", cf.ETag)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
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

func (s *Server) writeForecastWithCnt(w http.ResponseWriter, r *http.Request, cf store.CurrentForecast, cnt int) {
	var resp model.ForecastResponse
	if err := json.Unmarshal(cf.ResponseJSON, &resp); err != nil {
		writeError(w, r, http.StatusInternalServerError, "corrupt forecast cache")
		return
	}
	if cnt < resp.Cnt {
		resp.List = resp.List[:cnt]
		resp.Cnt = cnt
	}
	w.Header().Set("X-Cache", "STORE")
	write(w, r, http.StatusOK, resp)
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

func requireStationQuery(r *http.Request) (int64, bool, string) {
	if !r.URL.Query().Has("station") {
		return 0, false, "query parameter 'station' is required"
	}
	raw := strings.TrimSpace(r.URL.Query().Get("station"))
	if raw == "" {
		return 0, false, "query parameter 'station' must be a valid station id"
	}
	id, ok := parsePositiveInt(raw)
	if !ok {
		return 0, false, "query parameter 'station' must be a valid station id"
	}
	return id, true, ""
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

// XML support kept for later — uncomment encoding/xml import + these helpers to restore.
//
// func wantsXML(r *http.Request) bool {
// 	mode := strings.ToLower(r.URL.Query().Get("mode"))
// 	if mode == "xml" {
// 		return true
// 	}
// 	if mode == "json" {
// 		return false
// 	}
// 	accept := r.Header.Get("Accept")
// 	return strings.Contains(accept, "application/xml") || strings.Contains(accept, "text/xml")
// }

func write(w http.ResponseWriter, r *http.Request, status int, v any) {
	// JSON only for now.
	// if wantsXML(r) {
	// 	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// 	w.WriteHeader(status)
	// 	_, _ = w.Write([]byte(xml.Header))
	// 	enc := xml.NewEncoder(w)
	// 	enc.Indent("", "  ")
	// 	_ = enc.Encode(v)
	// 	return
	// }
	_ = r // reserved for future format negotiation (XML)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	write(w, r, status, model.APIError{Code: status, Message: msg})
}
