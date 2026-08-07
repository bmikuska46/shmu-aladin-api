package api

import (
	"testing"
	"time"
)

func TestRouteLimiterPerKey(t *testing.T) {
	l := newRouteLimiter(10, time.Minute)
	now := time.Unix(1_700_000_000, 0)

	for i := 0; i < 10; i++ {
		ok, rem, _ := l.allow("ip|/api/v1/stations", now)
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if rem != 9-i {
			t.Fatalf("remaining=%d want %d", rem, 9-i)
		}
	}

	ok, rem, retry := l.allow("ip|/api/v1/stations", now)
	if ok || rem != 0 || retry < 1 {
		t.Fatalf("11th request should be blocked: ok=%v rem=%d retry=%d", ok, rem, retry)
	}

	// Different route has its own quota.
	ok, _, _ = l.allow("ip|/api/v1/forecast", now)
	if !ok {
		t.Fatal("other route should still be allowed")
	}

	// Window reset restores quota.
	ok, rem, _ = l.allow("ip|/api/v1/stations", now.Add(time.Minute+time.Second))
	if !ok || rem != 9 {
		t.Fatalf("after window reset: ok=%v rem=%d", ok, rem)
	}
}
