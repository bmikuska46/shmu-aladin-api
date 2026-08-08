package transform_test

import (
	"testing"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/geo"
	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/transform"
)

func f64(v float64) *float64 { return &v }

func TestAggregateDailyBasics(t *testing.T) {
	// Two full hours on a summer day in Bratislava (CEST, UTC+2).
	dayStart := time.Date(2026, 8, 8, 0, 0, 0, 0, geo.Bratislava).Unix()
	items := make([]model.ForecastItem, 0, 24)
	for i := 0; i < 24; i++ {
		ts := dayStart + int64(i*3600)
		rain := 0.0
		if i == 10 || i == 11 {
			rain = 1.5
		}
		items = append(items, model.ForecastItem{
			DT: ts,
			Main: model.ForecastMain{
				Temp:    f64(20 + float64(i)*0.1),
				TempMin: f64(19 + float64(i)*0.1),
				TempMax: f64(21 + float64(i)*0.1),
			},
			Clouds:  model.ForecastClouds{All: f64(40)},
			Wind:    model.ForecastWind{Speed: f64(5), Gust: f64(8 + float64(i)*0.1)},
			Rain:    &model.Precip1h{H1: rain},
			Snow:    &model.Precip1h{H1: 0},
			Weather: []model.WeatherCond{model.WeatherClear.Cond()},
		})
		if rain > 0 {
			items[i].Weather = []model.WeatherCond{model.WeatherLightRain.Cond()}
		}
	}

	hourly := model.ForecastResponse{
		List: items,
		City: model.ForecastCity{Coord: model.Coord{Lat: 48.15, Lon: 17.11}},
		Meta: model.ForecastMeta{Source: "SHMU ALADIN"},
	}
	daily := transform.AggregateDaily(hourly, 0)
	if daily.Cnt != 1 {
		t.Fatalf("cnt=%d", daily.Cnt)
	}
	d := daily.List[0]
	if d.Date != "2026-08-08" {
		t.Fatalf("date=%s", d.Date)
	}
	// 2026-08-08 is a Saturday → index 5
	if d.DateIndex != 5 {
		t.Fatalf("date_index=%d want 5", d.DateIndex)
	}
	if d.TempMin == nil || *d.TempMin != 19 {
		t.Fatalf("temp_min=%v", d.TempMin)
	}
	if d.TempMax == nil || *d.TempMax < 23 {
		t.Fatalf("temp_max=%v", d.TempMax)
	}
	if d.PrecipitationTotal == nil || *d.PrecipitationTotal != 3 {
		t.Fatalf("precip=%v", d.PrecipitationTotal)
	}
	if d.PrecipitationHours == nil || *d.PrecipitationHours != 2 {
		t.Fatalf("precip_hours=%v", d.PrecipitationHours)
	}
	if d.IsPartial {
		t.Fatal("expected complete day")
	}
	if d.Sunrise == "" || d.Sunset == "" {
		t.Fatal("expected sunrise/sunset")
	}
	if d.Weather.Code != model.WeatherLightRain {
		t.Fatalf("representative weather=%s", d.Weather.Code)
	}
}

func TestAggregateDailyPartialAndMissing(t *testing.T) {
	ts := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC).Unix()
	hourly := model.ForecastResponse{
		List: []model.ForecastItem{{
			DT:   ts,
			Main: model.ForecastMain{Temp: f64(25)},
			// no rain/snow → precipitation fields stay null
			Weather: []model.WeatherCond{model.WeatherClear.Cond()},
		}},
		City: model.ForecastCity{Coord: model.Coord{Lat: 48.15, Lon: 17.11}},
	}
	daily := transform.AggregateDaily(hourly, 1)
	d := daily.List[0]
	if !d.IsPartial {
		t.Fatal("expected partial day")
	}
	if d.PrecipitationTotal != nil || d.RainTotal != nil || d.SnowTotal != nil {
		t.Fatalf("expected null precip, got total=%v rain=%v snow=%v", d.PrecipitationTotal, d.RainTotal, d.SnowTotal)
	}
}

func TestAggregateDailyDSTSpringForward(t *testing.T) {
	// 2026-03-29 is 23 hours in Europe/Bratislava.
	start := time.Date(2026, 3, 29, 0, 0, 0, 0, geo.Bratislava).Unix()
	end := time.Date(2026, 3, 30, 0, 0, 0, 0, geo.Bratislava).Unix()
	var items []model.ForecastItem
	for ts := start; ts < end; ts += 3600 {
		items = append(items, model.ForecastItem{
			DT:      ts,
			Main:    model.ForecastMain{Temp: f64(5)},
			Rain:    &model.Precip1h{H1: 0},
			Snow:    &model.Precip1h{H1: 0},
			Weather: []model.WeatherCond{model.WeatherClear.Cond()},
		})
	}
	if len(items) != 23 {
		t.Fatalf("expected 23 hours, got %d", len(items))
	}
	daily := transform.AggregateDaily(model.ForecastResponse{
		List: items,
		City: model.ForecastCity{Coord: model.Coord{Lat: 48.15, Lon: 17.11}},
	}, 0)
	if daily.List[0].IsPartial {
		t.Fatal("23-hour DST day with all local hours should not be partial")
	}
}
