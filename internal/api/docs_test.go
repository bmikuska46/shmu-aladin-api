package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/config"
)

func TestDocumentationLocales(t *testing.T) {
	server := New(nil, nil, config.Config{WebURL: "https://weather.example/"})

	tests := []struct {
		path         string
		language     string
		bodyContains []string
	}{
		{
			path:     "/",
			language: "sk",
			bodyContains: []string{
				`<html lang="sk">`,
				`<h2>Rýchly štart</h2>`,
				`href="/en"`,
				`<pre><code>https://weather.example/api/v1</code></pre>`,
				`https://weather.example/api/v1/stations`,
				`Ukážka odpovede`,
				`Invoke-RestMethod "https://weather.example/api/v1/stations?q=zilina&amp;page_size=5"`,
				`<link rel="canonical" href="https://weather.example/" />`,
				`<meta name="robots" content="index, follow, max-image-preview:large, max-snippet:-1" />`,
				`property="og:locale" content="sk_SK"`,
				`"@type": "WebAPI"`,
				seoDescriptionSK,
			},
		},
		{
			path:     "/docs",
			language: "sk",
			bodyContains: []string{
				`<html lang="sk">`,
			},
		},
		{
			path:     "/en",
			language: "en",
			bodyContains: []string{
				`<html lang="en">`,
				`<h2>Quick start</h2>`,
				`<h2>Endpoints</h2>`,
				`Example response`,
				`Invoke-RestMethod "https://weather.example/api/v1/forecast?station=32737&amp;hours=6"`,
				`class="brand" aria-label="SHMU ALADIN API documentation"`,
				`href="/"`,
				`>Slovenčina</a>`,
				`<link rel="canonical" href="https://weather.example/en" />`,
				`property="og:locale" content="en_US"`,
				`"inLanguage": "en"`,
				seoDescriptionEN,
			},
		},
		{
			path:     "/en/",
			language: "en",
			bodyContains: []string{
				`<html lang="en">`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			res := httptest.NewRecorder()
			server.Handler().ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status=%d want %d", res.Code, http.StatusOK)
			}
			if got := res.Header().Get("Content-Language"); got != tt.language {
				t.Fatalf("Content-Language=%q want %q", got, tt.language)
			}
			if got := res.Header().Get("X-RateLimit-Limit"); got != "" {
				t.Fatalf("documentation route was rate-limited: X-RateLimit-Limit=%q", got)
			}

			body := res.Body.String()
			for _, want := range tt.bodyContains {
				if !strings.Contains(body, want) {
					t.Errorf("body does not contain %q", want)
				}
			}
			if strings.Contains(body, "__") {
				t.Error("body contains an unresolved template placeholder")
			}
		})
	}
}

func TestRobotsAndSitemap(t *testing.T) {
	server := New(nil, nil, config.Config{WebURL: "https://weather.example/"})

	t.Run("robots.txt", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("status=%d want %d", res.Code, http.StatusOK)
		}
		if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
			t.Fatalf("Content-Type=%q", got)
		}
		if got := res.Header().Get("X-RateLimit-Limit"); got != "" {
			t.Fatalf("robots.txt was rate-limited: X-RateLimit-Limit=%q", got)
		}
		body := res.Body.String()
		for _, want := range []string{
			"User-agent: *",
			"Allow: /",
			"Disallow: /api/",
			"Sitemap: https://weather.example/sitemap.xml",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("body does not contain %q", want)
			}
		}
	})

	t.Run("sitemap.xml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		res := httptest.NewRecorder()
		server.Handler().ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("status=%d want %d", res.Code, http.StatusOK)
		}
		if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/xml") {
			t.Fatalf("Content-Type=%q", got)
		}
		if got := res.Header().Get("X-RateLimit-Limit"); got != "" {
			t.Fatalf("sitemap.xml was rate-limited: X-RateLimit-Limit=%q", got)
		}
		body := res.Body.String()
		for _, want := range []string{
			"<loc>https://weather.example/</loc>",
			"<loc>https://weather.example/en</loc>",
			"<loc>https://weather.example/docs</loc>",
			`hreflang="sk"`,
			`hreflang="en"`,
			`hreflang="x-default"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("body does not contain %q", want)
			}
		}
	})
}
