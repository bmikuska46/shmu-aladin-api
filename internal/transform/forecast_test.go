package transform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if out.City.ID != 32737 {
		t.Fatalf("city.id=%d", out.City.ID)
	}
	if out.City.Name != "Bratislava (centrum)" {
		t.Fatalf("name=%s", out.City.Name)
	}
	if out.Cnt == 0 || len(out.List) == 0 {
		t.Fatal("expected forecast list")
	}
	item := out.List[0]
	if item.Main.Temp == nil {
		t.Fatal("expected temp")
	}
	if len(item.Weather) == 0 {
		t.Fatal("expected weather condition")
	}
	if out.Meta.Source != "SHMU ALADIN" {
		t.Fatalf("meta.source=%s", out.Meta.Source)
	}
	// August runtime → CEST (+7200).
	if out.City.Timezone != 7200 {
		t.Fatalf("timezone=%d want 7200", out.City.Timezone)
	}
	// Sample includes midnight UTC hours which are nighttime in August.
	foundNight := false
	for _, it := range out.List {
		if len(it.Weather) > 0 && strings.HasSuffix(it.Weather[0].Icon, "n") {
			foundNight = true
			break
		}
	}
	if !foundNight {
		t.Fatal("expected at least one nighttime icon suffix")
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
