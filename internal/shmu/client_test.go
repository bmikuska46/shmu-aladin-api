package shmu

import "testing"

func TestAladinFileLink(t *testing.T) {
	// 2026-08-07 00:00:00 UTC
	const runtimeTS = 1786060800
	got := AladinFileLink(32737, runtimeTS)
	want := "aladin/2026-08-07/32737_2026-08-07_00.json"
	if got != want {
		t.Fatalf("AladinFileLink = %q, want %q", got, want)
	}

	// 2026-08-07 06:00:00 UTC
	got = AladinFileLink(1, runtimeTS+6*3600)
	want = "aladin/2026-08-07/1_2026-08-07_06.json"
	if got != want {
		t.Fatalf("AladinFileLink = %q, want %q", got, want)
	}
}
