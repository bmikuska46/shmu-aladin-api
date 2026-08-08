package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/config"
	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/store"
)

func TestResolveForecastStation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	err = st.UpsertStations(context.Background(), []model.Station{
		{ID: 32737, Name: "Bratislava", Lat: 48.156, Lon: 17.105, DistrictCode: 101},
		{ID: 1, Name: "Far", Lat: 49.0, Lon: 20.0, DistrictCode: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := New(st, nil, config.Config{})

	tests := []struct {
		name       string
		query      string
		wantID     int64
		wantStatus int
	}{
		{name: "station", query: "station=32737", wantID: 32737},
		{name: "lat lon", query: "lat=48.15&lon=17.10", wantID: 32737},
		{name: "latitude longitude aliases", query: "latitude=48.15&longitude=17.10", wantID: 32737},
		{name: "missing", query: "", wantStatus: http.StatusBadRequest},
		{name: "only lat", query: "lat=48.15", wantStatus: http.StatusBadRequest},
		{name: "both modes", query: "station=32737&lat=48.15&lon=17.10", wantStatus: http.StatusBadRequest},
		{name: "bad lat", query: "lat=abc&lon=17", wantStatus: http.StatusBadRequest},
		{name: "lat out of range", query: "lat=100&lon=17", wantStatus: http.StatusBadRequest},
		{name: "bad station", query: "station=0", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/forecast?"+tt.query, nil)
			id, status, msg := server.resolveForecastStation(req)
			if tt.wantStatus != 0 {
				if status != tt.wantStatus {
					t.Fatalf("status=%d want %d msg=%q", status, tt.wantStatus, msg)
				}
				return
			}
			if status != 0 {
				t.Fatalf("unexpected status=%d msg=%q", status, msg)
			}
			if id != tt.wantID {
				t.Fatalf("id=%d want %d", id, tt.wantID)
			}
		})
	}
}
