package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/config"
)

func TestAPIIndex(t *testing.T) {
	server := New(nil, nil, config.Config{WebURL: "https://weather.example/"})

	for _, path := range []string{"/api/v1", "/api/v1/"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			res := httptest.NewRecorder()
			server.Handler().ServeHTTP(res, req)

			if res.Code != http.StatusOK {
				t.Fatalf("status=%d want %d", res.Code, http.StatusOK)
			}
			if ct := res.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type=%q", ct)
			}

			var body struct {
				Name          string `json:"name"`
				Version       string `json:"version"`
				BaseURL       string `json:"base_url"`
				Documentation string `json:"documentation"`
				Endpoints     []struct {
					Method string `json:"method"`
					Path   string `json:"path"`
				} `json:"endpoints"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatalf("json: %v", err)
			}
			if body.Name != "SHMU ALADIN API" || body.Version != "v1" {
				t.Fatalf("name/version = %q %q", body.Name, body.Version)
			}
			if body.BaseURL != "https://weather.example/api/v1" {
				t.Fatalf("base_url=%q", body.BaseURL)
			}
			if body.Documentation != "https://weather.example/" {
				t.Fatalf("documentation=%q", body.Documentation)
			}
			wantPaths := []string{
				"/api/v1/stations",
				"/api/v1/stations/{id}",
				"/api/v1/forecast?station={id}|lat={lat}&lon={lon}",
				"/api/v1/forecast/daily?station={id}|lat={lat}&lon={lon}",
				"/api/v1/weather?station={id}|lat={lat}&lon={lon}",
				"/api/v1/indicators?station={id}|lat={lat}&lon={lon}",
			}
			if len(body.Endpoints) != len(wantPaths) {
				t.Fatalf("endpoints=%d want %d", len(body.Endpoints), len(wantPaths))
			}
			for i, want := range wantPaths {
				if body.Endpoints[i].Method != "GET" || body.Endpoints[i].Path != want {
					t.Errorf("endpoint[%d]=%s %s want GET %s", i, body.Endpoints[i].Method, body.Endpoints[i].Path, want)
				}
			}
		})
	}
}
