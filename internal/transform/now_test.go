package transform

import (
	"testing"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func TestSelectNowPicksClosestHour(t *testing.T) {
	hourly := model.ForecastResponse{
		List: []model.ForecastItem{
			{DT: 1_000, DTTxt: "a"},
			{DT: 1_800, DTTxt: "b"},
			{DT: 3_600, DTTxt: "c"},
		},
		City: model.ForecastCity{Name: "Test"},
		Meta: model.ForecastMeta{Source: "SHMU ALADIN"},
	}

	at := time.Unix(2_000, 0).UTC()
	resp, err := SelectNow(hourly, at)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Current.DT != 1_800 {
		t.Fatalf("dt=%d want 1800", resp.Current.DT)
	}
	if resp.OffsetSeconds != -200 {
		t.Fatalf("offset=%d want -200", resp.OffsetSeconds)
	}
	if resp.AsOf != "1970-01-01T00:33:20Z" {
		t.Fatalf("as_of=%q", resp.AsOf)
	}
	if resp.City.Name != "Test" {
		t.Fatalf("city=%q", resp.City.Name)
	}
}

func TestSelectNowEmpty(t *testing.T) {
	_, err := SelectNow(model.ForecastResponse{}, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSelectNowExactMatch(t *testing.T) {
	hourly := model.ForecastResponse{
		List: []model.ForecastItem{{DT: 5_000}, {DT: 8_000}},
	}
	resp, err := SelectNow(hourly, time.Unix(5_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if resp.OffsetSeconds != 0 || resp.Current.DT != 5_000 {
		t.Fatalf("%+v", resp)
	}
}
