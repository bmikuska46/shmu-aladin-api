package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type routeLimiter struct {
	limit  int
	window time.Duration

	mu      sync.Mutex
	buckets map[string]*windowBucket
}

type windowBucket struct {
	count   int
	resetAt time.Time
}

func newRouteLimiter(limit int, window time.Duration) *routeLimiter {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return &routeLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*windowBucket),
	}
}

// allow returns whether the request may proceed, remaining quota, and retry-after seconds.
func (l *routeLimiter) allow(key string, now time.Time) (ok bool, remaining int, retryAfter int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Occasional cleanup of stale buckets.
	if len(l.buckets) > 10_000 {
		for k, b := range l.buckets {
			if now.After(b.resetAt) {
				delete(l.buckets, k)
			}
		}
	}

	b, exists := l.buckets[key]
	if !exists || now.After(b.resetAt) {
		l.buckets[key] = &windowBucket{count: 1, resetAt: now.Add(l.window)}
		return true, l.limit - 1, 0
	}

	if b.count >= l.limit {
		ra := int(b.resetAt.Sub(now).Seconds())
		if ra < 1 {
			ra = 1
		}
		return false, 0, ra
	}

	b.count++
	return true, l.limit - b.count, 0
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// routeBucketKey groups requests by API route pattern (not raw path params).
func routeBucketKey(r *http.Request) string {
	path := r.URL.Path
	switch {
	case path == "/" || path == "":
		return "GET /"
	case path == "/docs":
		return "GET /docs"
	case path == "/en" || path == "/en/":
		return "GET /en"
	case path == "/robots.txt":
		return "GET /robots.txt"
	case path == "/sitemap.xml":
		return "GET /sitemap.xml"
	case path == "/health":
		return "GET /health"
	case path == "/api/v1/stations":
		return "GET /api/v1/stations"
	case strings.HasPrefix(path, "/api/v1/stations/"):
		return "GET /api/v1/stations/{id}"
	case path == "/api/v1/forecast":
		return "GET /api/v1/forecast"
	case path == "/api/v1/weather":
		return "GET /api/v1/weather"
	default:
		return r.Method + " " + path
	}
}
