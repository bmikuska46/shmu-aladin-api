package transform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/geo"
	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/shmu"
	"github.com/bmikuska/shmu-weather-api/internal/transform"
)

func TestToOWMForecastFromSample(t *testing.T) {
	root := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "32737_2026-08-07_00.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw shmu.AladinForecast
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	out, err := transform.ToOWMForecast(&raw, 1786060800)
	if err != nil {
		t.Fatal(err)
	}
	if out.Code != "200" {
		t.Fatalf("code=%s", out.Code)
	}
	if out.City.Name != "Bratislava (centrum)" {
		t.Fatalf("name=%s", out.City.Name)
	}
	if out.Hours == 0 || len(out.List) == 0 {
		t.Fatal("expected forecast list")
	}
	if out.Hours != len(out.List) {
		t.Fatalf("hours=%d len(list)=%d", out.Hours, len(out.List))
	}
	item := out.List[0]
	if item.Main.Temp == nil {
		t.Fatal("expected temp")
	}
	if len(item.Weather) == 0 || item.Weather[0].Code == "" || item.Weather[0].Description == "" {
		t.Fatalf("expected weather code+description, got %+v", item.Weather)
	}
	if item.Date == "" {
		t.Fatal("expected local date")
	}
	if item.DateIndex < 0 || item.DateIndex > 6 {
		t.Fatalf("date_index=%d", item.DateIndex)
	}
	// 2026-08-07 is a Friday → index 4
	wantIndex := geo.WeekdayIndex(time.Friday)
	foundFriday := false
	for _, it := range out.List {
		if it.Date == "2026-08-07" {
			foundFriday = true
			if it.DateIndex != wantIndex {
				t.Fatalf("date_index=%d want %d for %s", it.DateIndex, wantIndex, it.Date)
			}
			break
		}
	}
	if !foundFriday {
		t.Fatal("expected items on 2026-08-07")
	}
	if out.Meta.Source != "SHMU ALADIN" {
		t.Fatalf("meta.source=%s", out.Meta.Source)
	}
	// August runtime → CEST (+7200).
	if out.City.Timezone != 7200 {
		t.Fatalf("timezone=%d want 7200", out.City.Timezone)
	}
	// Zero precip should be preserved when source has a reading.
	foundZeroRain := false
	for _, it := range out.List {
		if it.Rain != nil && it.Rain.H1 == 0 {
			foundZeroRain = true
			break
		}
	}
	if !foundZeroRain {
		t.Fatal("expected rain:0 when source has explicit zero")
	}
	// Weather uses catalog codes only (no icon/id/main).
	for _, it := range out.List {
		for _, w := range it.Weather {
			if w.Code == model.WeatherClear && w.Description != "clear sky" {
				t.Fatalf("clear description=%q", w.Description)
			}
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "32737_2026-08-07_00.json")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repo root not found")
	return ""
}
