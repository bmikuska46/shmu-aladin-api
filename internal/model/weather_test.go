package model_test

import (
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func TestAllWeatherCodes(t *testing.T) {
	codes := model.AllWeatherCodes()
	if len(codes) == 0 {
		t.Fatal("expected weather codes")
	}
	seen := map[model.WeatherCode]bool{}
	for _, c := range codes {
		if c.Code == "" || c.Description == "" {
			t.Fatalf("incomplete entry: %+v", c)
		}
		if seen[c.Code] {
			t.Fatalf("duplicate code %s", c.Code)
		}
		seen[c.Code] = true
		cond := c.Code.Cond()
		if cond.Code != c.Code || cond.Description != c.Description {
			t.Fatalf("Cond mismatch: %+v vs %+v", cond, c)
		}
	}
}
