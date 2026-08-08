package geo_test

import (
	"testing"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/geo"
)

func TestHaversineBratislavaKosice(t *testing.T) {
	// Approximate known distance ~313–314 km.
	km := geo.HaversineKm(48.1486, 17.1077, 48.7164, 21.2611)
	if km < 300 || km > 330 {
		t.Fatalf("distance=%.2f km, want ~313", km)
	}
}

func TestHaversineZero(t *testing.T) {
	if d := geo.HaversineKm(48.15, 17.10, 48.15, 17.10); d != 0 {
		t.Fatalf("got %v", d)
	}
}

func TestTimezoneOffsetCETCEST(t *testing.T) {
	// 2026-01-15 12:00 UTC → CET (+1h)
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC).Unix()
	if off := geo.TimezoneOffsetSeconds(winter); off != 3600 {
		t.Fatalf("winter offset=%d want 3600", off)
	}
	// 2026-07-15 12:00 UTC → CEST (+2h)
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC).Unix()
	if off := geo.TimezoneOffsetSeconds(summer); off != 7200 {
		t.Fatalf("summer offset=%d want 7200", off)
	}
}

func TestLocalDateDSTTransitions(t *testing.T) {
	// Spring forward 2026-03-29: local day still 2026-03-29 for 00:30 CET.
	spring := time.Date(2026, 3, 28, 23, 30, 0, 0, time.UTC).Unix() // 00:30 CET
	if got := geo.LocalDateString(spring); got != "2026-03-29" {
		t.Fatalf("spring local date=%s", got)
	}
	// Fall back 2026-10-25.
	fall := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).Unix() // 02:30 CEST
	if got := geo.LocalDateString(fall); got != "2026-10-25" {
		t.Fatalf("fall local date=%s", got)
	}
}

func TestSunriseSunsetBratislavaSummer(t *testing.T) {
	day := time.Date(2026, 8, 8, 0, 0, 0, 0, geo.Bratislava)
	rise, set, ok := geo.SunriseSunset(day, 48.1486, 17.1077)
	if !ok {
		t.Fatal("expected sunrise/sunset")
	}
	if !rise.Before(set) {
		t.Fatalf("sunrise %v not before sunset %v", rise, set)
	}
	// Rough window: sunrise ~03:30–05:00 UTC, sunset ~17:00–19:00 UTC in August.
	if rise.Hour() < 2 || rise.Hour() > 6 {
		t.Fatalf("unexpected sunrise hour %d (%v)", rise.Hour(), rise)
	}
	if set.Hour() < 16 || set.Hour() > 20 {
		t.Fatalf("unexpected sunset hour %d (%v)", set.Hour(), set)
	}
}

func TestIsDaytime(t *testing.T) {
	day := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC).Unix()
	night := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC).Unix()
	if !geo.IsDaytime(day, 48.15, 17.11) {
		t.Fatal("noon should be daytime")
	}
	if geo.IsDaytime(night, 48.15, 17.11) {
		t.Fatal("midnight UTC should be nighttime in August")
	}
}

func TestForecastFreshness(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	fetched := now.Add(-2 * time.Hour).Unix()
	f := geo.ForecastFreshness(fetched, now, 8*time.Hour)
	if f.IsStale || f.AgeSeconds < 7199 || f.AgeSeconds > 7201 {
		t.Fatalf("%+v", f)
	}
	f = geo.ForecastFreshness(fetched, now, time.Hour)
	if !f.IsStale {
		t.Fatal("expected stale")
	}
}
