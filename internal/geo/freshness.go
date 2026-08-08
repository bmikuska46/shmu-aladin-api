package geo

import "time"

// SourceFreshness describes how old a stored forecast is relative to a stale threshold.
type SourceFreshness struct {
	FetchedAt  time.Time
	AgeSeconds int64
	IsStale    bool
}

// ForecastFreshness computes age and stale state from a fetch unix timestamp.
func ForecastFreshness(fetchedAtUnix int64, now time.Time, staleAfter time.Duration) SourceFreshness {
	fetched := time.Unix(fetchedAtUnix, 0).UTC()
	if fetchedAtUnix <= 0 {
		fetched = time.Time{}
	}
	age := int64(0)
	if !fetched.IsZero() && now.After(fetched) {
		age = int64(now.Sub(fetched).Seconds())
	}
	stale := false
	if staleAfter > 0 && (fetched.IsZero() || now.Sub(fetched) > staleAfter) {
		stale = true
	}
	return SourceFreshness{
		FetchedAt:  fetched,
		AgeSeconds: age,
		IsStale:    stale,
	}
}
