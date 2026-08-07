package syncer

import (
	"testing"
	"time"
)

func TestAladinSlotFirstAttempt(t *testing.T) {
	slot := AladinSlot{PublishHour: 10, PublishMin: 45, RunHour: 6}
	day := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	got := slot.FirstAttemptTime(day, 5*time.Minute)
	want := time.Date(2026, 8, 7, 10, 50, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
	if hr := slot.ExpectedRuntime(day).Hour(); hr != 6 {
		t.Fatalf("run hour=%d", hr)
	}
}

func TestCronSpec(t *testing.T) {
	cases := []struct {
		slot AladinSlot
		want string
	}{
		{AladinSlot{3, 45, 0}, "50 3 * * *"},
		{AladinSlot{10, 45, 6}, "50 10 * * *"},
		{AladinSlot{15, 45, 12}, "50 15 * * *"},
		{AladinSlot{22, 45, 18}, "50 22 * * *"},
	}
	for _, tc := range cases {
		if got := tc.slot.CronSpec(5 * time.Minute); got != tc.want {
			t.Fatalf("slot %v: got %q want %q", tc.slot, got, tc.want)
		}
	}
}

func TestCurrentOrDueSlot(t *testing.T) {
	delay := 5 * time.Minute
	// After 10:50 UTC on Aug 7 → due for 06z run
	now := time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	slot, runtime, due := CurrentOrDueSlot(now, delay, DefaultAladinSlots)
	if !due || slot.RunHour != 6 || runtime.Hour() != 6 {
		t.Fatalf("slot=%v runtime=%s due=%v", slot, runtime, due)
	}
	// Before first attempt of the day
	now = time.Date(2026, 8, 7, 3, 40, 0, 0, time.UTC)
	_, _, due = CurrentOrDueSlot(now, delay, DefaultAladinSlots)
	if due {
		// previous day's 22:50 window may still be "due" until 03:50 — that's OK
		// At 03:40, next first attempt is 03:50, previous was yesterday 22:50
		// so due could be true for 18z yesterday. Accept due for yesterday 18z.
	}
	slot, runtime, due = CurrentOrDueSlot(now, delay, DefaultAladinSlots)
	if !due || slot.RunHour != 18 {
		t.Fatalf("expected previous 18z catch-up, got run=%d due=%v runtime=%s", slot.RunHour, due, runtime)
	}
}
