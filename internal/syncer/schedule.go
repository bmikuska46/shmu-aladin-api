package syncer

import (
	"fmt"
	"time"
)

// SHMU documents ALADIN product updates four times per day (meteogram page):
//
//	03:45 UTC → model run 00:00 UTC
//	10:45 UTC → model run 06:00 UTC
//	15:45 UTC → model run 12:00 UTC
//	22:45 UTC → model run 18:00 UTC
//
// We start fetching publishTime + FetchDelay (default 5m), then retry every
// FetchRetryEvery (default 5m) until that runtime appears upstream.

type AladinSlot struct {
	PublishHour int // UTC hour when SHMU publishes products
	PublishMin  int
	RunHour     int // UTC model runtime hour (0, 6, 12, 18)
}

var DefaultAladinSlots = []AladinSlot{
	{PublishHour: 3, PublishMin: 45, RunHour: 0},
	{PublishHour: 10, PublishMin: 45, RunHour: 6},
	{PublishHour: 15, PublishMin: 45, RunHour: 12},
	{PublishHour: 22, PublishMin: 45, RunHour: 18},
}

// FirstAttemptTime is publish time + delay for a given UTC calendar day.
func (s AladinSlot) FirstAttemptTime(day time.Time, delay time.Duration) time.Time {
	day = day.UTC()
	publish := time.Date(day.Year(), day.Month(), day.Day(), s.PublishHour, s.PublishMin, 0, 0, time.UTC)
	return publish.Add(delay)
}

// ExpectedRuntime is the ALADIN model init time for this slot on the given UTC day.
func (s AladinSlot) ExpectedRuntime(day time.Time) time.Time {
	day = day.UTC()
	return time.Date(day.Year(), day.Month(), day.Day(), s.RunHour, 0, 0, 0, time.UTC)
}

func (s AladinSlot) CronSpec(delay time.Duration) string {
	t := time.Date(2000, 1, 1, s.PublishHour, s.PublishMin, 0, 0, time.UTC).Add(delay)
	// robfig/cron standard: min hour dom mon dow
	return fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour())
}

// CurrentOrDueSlot returns the most recent slot whose first attempt time has passed,
// and whether we should still be trying to fetch it (before the next slot's first attempt).
func CurrentOrDueSlot(now time.Time, delay time.Duration, slots []AladinSlot) (slot AladinSlot, runtime time.Time, due bool) {
	now = now.UTC()
	var best AladinSlot
	var bestAttempt time.Time
	found := false

	for dayOffset := 0; dayOffset >= -1; dayOffset-- {
		day := now.AddDate(0, 0, dayOffset)
		for _, sl := range slots {
			attempt := sl.FirstAttemptTime(day, delay)
			if attempt.After(now) {
				continue
			}
			if !found || attempt.After(bestAttempt) {
				best = sl
				bestAttempt = attempt
				found = true
			}
		}
		if found {
			break
		}
	}
	if !found {
		return AladinSlot{}, time.Time{}, false
	}

	runtime = best.ExpectedRuntime(bestAttempt)
	nextAttempt := nextFirstAttempt(now, delay, slots)
	due = nextAttempt.IsZero() || now.Before(nextAttempt)
	return best, runtime, due
}

func nextFirstAttempt(now time.Time, delay time.Duration, slots []AladinSlot) time.Time {
	now = now.UTC()
	var soonest time.Time
	for dayOffset := 0; dayOffset <= 1; dayOffset++ {
		day := now.AddDate(0, 0, dayOffset)
		for _, sl := range slots {
			attempt := sl.FirstAttemptTime(day, delay)
			if !attempt.After(now) {
				continue
			}
			if soonest.IsZero() || attempt.Before(soonest) {
				soonest = attempt
			}
		}
	}
	return soonest
}
