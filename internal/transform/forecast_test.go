package transform_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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
