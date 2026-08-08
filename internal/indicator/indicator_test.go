package indicator_test

import (
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/indicator"
	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func f64(v float64) *float64 { return &v }

func hourlyTemps(vals []float64, start int64) model.ForecastResponse {
	list := make([]model.ForecastItem, len(vals))
	for i, v := range vals {
		list[i] = model.ForecastItem{
			DT:   start + int64(i*3600),
			Main: model.ForecastMain{Temp: f64(v)},
		}
	}
	return model.ForecastResponse{List: list, Meta: model.ForecastMeta{Runtime: "2026-01-01T00:00:00Z"}}
}

func TestFrostThresholds(t *testing.T) {
	// 2 hours below 0 → low (no window)
	resp, err := indicator.Evaluate(hourlyTemps([]float64{-1, -1, 1}, 1_700_000_000), []string{"frost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 1 || resp[0].Level != "low" || !resp[0].Available || resp[0].Official {
		t.Fatalf("%+v", resp[0])
	}

	// 3 hours below 0, min -2 → moderate
	resp, err = indicator.Evaluate(hourlyTemps([]float64{-1, -2, -1}, 1_700_000_000), []string{"frost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 1 || resp[0].Level != "moderate" || resp[0].RuleVersion != "frost/v1" {
		t.Fatalf("%+v", resp[0])
	}

	// high at <= -5
	resp, err = indicator.Evaluate(hourlyTemps([]float64{-5, -6, -5}, 1_700_000_000), []string{"frost"})
	if err != nil {
		t.Fatal(err)
	}
	if resp[0].Level != "high" {
		t.Fatalf("level=%s", resp[0].Level)
	}
}

func TestHeatAndWind(t *testing.T) {
	resp, err := indicator.Evaluate(hourlyTemps([]float64{30, 31, 32}, 1_700_000_000), []string{"heat"})
	if err != nil {
		t.Fatal(err)
	}
	if resp[0].Level != "moderate" {
		t.Fatalf("%+v", resp[0])
	}
	resp, err = indicator.Evaluate(hourlyTemps([]float64{35, 36, 35}, 1_700_000_000), []string{"heat"})
	if err != nil {
		t.Fatal(err)
	}
	if resp[0].Level != "high" {
		t.Fatalf("%+v", resp[0])
	}

	wind := model.ForecastResponse{
		List: []model.ForecastItem{{
			DT:   1_700_000_000,
			Wind: model.ForecastWind{Speed: f64(10), Gust: f64(15)},
		}},
		Meta: model.ForecastMeta{Runtime: "x"},
	}
	resp, err = indicator.Evaluate(wind, []string{"wind"})
	if err != nil {
		t.Fatal(err)
	}
	if resp[0].Level != "moderate" {
		t.Fatalf("%+v", resp[0])
	}
}

func TestHeavyRainRolling(t *testing.T) {
	list := make([]model.ForecastItem, 3)
	for i := 0; i < 3; i++ {
		list[i] = model.ForecastItem{
			DT:   1_700_000_000 + int64(i*3600),
			Rain: &model.Precip1h{H1: 10},
		}
	}
	resp, err := indicator.Evaluate(model.ForecastResponse{List: list}, []string{"heavy_rain"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ind := range resp {
		if ind.Type == "heavy_rain" && ind.Level != "low" {
			found = true
			if ind.Official {
				t.Fatal("must not be official")
			}
		}
	}
	if !found {
		t.Fatalf("expected heavy rain hit: %+v", resp)
	}
}

func TestUnknownTypeAndOfficialGuard(t *testing.T) {
	_, err := indicator.Evaluate(hourlyTemps([]float64{1}, 1), []string{"flood"})
	if err == nil {
		t.Fatal("expected error")
	}
	resp, err := indicator.Evaluate(hourlyTemps([]float64{1, 1, 1}, 1_700_000_000), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ind := range resp {
		if ind.Official || ind.SourceKind != "forecast" {
			t.Fatalf("%+v", ind)
		}
	}
}

func TestMissingTempUnavailable(t *testing.T) {
	resp, err := indicator.Evaluate(model.ForecastResponse{
		List: []model.ForecastItem{{DT: 1}},
	}, []string{"frost"})
	if err != nil {
		t.Fatal(err)
	}
	if resp[0].Available || resp[0].Level != "unknown" {
		t.Fatalf("%+v", resp[0])
	}
}
