package transform

import (
	"testing"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func TestLimitForecastHoursFromNowNotModelStart(t *testing.T) {
	// Model run starts at 00:00; request at 10:30 should start at 10:00.
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	list := make([]model.ForecastItem, 0, 24)
	for i := 0; i < 24; i++ {
		ts := base.Add(time.Duration(i) * time.Hour).Unix()
		list = append(list, model.ForecastItem{DT: ts})
	}
	resp := model.ForecastResponse{Hours: len(list), List: list}

	at := time.Date(2026, 8, 10, 10, 30, 0, 0, time.UTC)
	out := LimitForecastHours(resp, 6, at)

	if out.Hours != 6 || len(out.List) != 6 {
		t.Fatalf("hours=%d len=%d", out.Hours, len(out.List))
	}
	wantStart := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC).Unix()
	if out.List[0].DT != wantStart {
		t.Fatalf("start dt=%d want %d (from now, not model start)", out.List[0].DT, wantStart)
	}
	if out.List[5].DT != wantStart+5*3600 {
		t.Fatalf("end dt=%d", out.List[5].DT)
	}
}

func TestLimitForecastHoursCapsAtRemaining(t *testing.T) {
	base := time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC)
	list := []model.ForecastItem{
		{DT: base.Unix()},
		{DT: base.Add(time.Hour).Unix()},
		{DT: base.Add(2 * time.Hour).Unix()},
	}
	resp := model.ForecastResponse{Hours: 3, List: list}

	at := time.Date(2026, 8, 10, 21, 15, 0, 0, time.UTC)
	out := LimitForecastHours(resp, 10, at)
	if out.Hours != 2 || len(out.List) != 2 {
		t.Fatalf("hours=%d len=%d want 2 remaining from 21:00", out.Hours, len(out.List))
	}
	if out.List[0].DT != base.Add(time.Hour).Unix() {
		t.Fatalf("start=%d", out.List[0].DT)
	}
}

func TestLimitForecastHoursPastHorizon(t *testing.T) {
	resp := model.ForecastResponse{
		Hours: 1,
		List:  []model.ForecastItem{{DT: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC).Unix()}},
	}
	at := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	out := LimitForecastHours(resp, 3, at)
	if out.Hours != 0 || len(out.List) != 0 {
		t.Fatalf("expected empty, got hours=%d len=%d", out.Hours, len(out.List))
	}
}

func TestLimitForecastHoursNoopWhenNZero(t *testing.T) {
	resp := model.ForecastResponse{
		Hours: 2,
		List:  []model.ForecastItem{{DT: 1}, {DT: 2}},
	}
	out := LimitForecastHours(resp, 0, time.Now())
	if out.Hours != 2 || len(out.List) != 2 {
		t.Fatalf("expected unchanged, got hours=%d len=%d", out.Hours, len(out.List))
	}
}
