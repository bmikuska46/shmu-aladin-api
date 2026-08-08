package shmu

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAladinFileLink(t *testing.T) {
	ok := "aladin/2026-08-07/32737_2026-08-07_00.json"
	if err := ValidateAladinFileLink(ok); err != nil {
		t.Fatalf("valid link rejected: %v", err)
	}
	if err := ValidateAladinFileLink(AladinFileLink(32737, 1786060800)); err != nil {
		t.Fatalf("AladinFileLink output rejected: %v", err)
	}

	bad := []string{
		"",
		"aladin/../etc/passwd",
		"../aladin/2026-08-07/1_2026-08-07_00.json",
		"/aladin/2026-08-07/1_2026-08-07_00.json",
		"//evil.example/x.json",
		"https://evil.example/x.json",
		"http://evil.example/aladin/2026-08-07/1_2026-08-07_00.json",
		"aladin/2026-08-07/1_2026-08-07_00.json?x=1",
		"aladin/2026-08-07/1_2026-08-07_00.json#frag",
		"aladin/2026-08-07/not-a-number_2026-08-07_00.json",
		"aladin/2026-08-07/1_2026-08-07_0.json",
		"forecast/2026-08-07/1_2026-08-07_00.json",
		"aladin/2026-08-07/1_2026-08-07_00.json\n",
		"aladin\\2026-08-07\\1_2026-08-07_00.json",
	}
	for _, link := range bad {
		err := ValidateAladinFileLink(link)
		if !errors.Is(err, ErrInvalidFileLink) {
			t.Fatalf("ValidateAladinFileLink(%q)=%v, want ErrInvalidFileLink", link, err)
		}
	}
}

func TestJoinDataURL(t *testing.T) {
	base := "https://data.shmu.example/datanwp"
	link := "aladin/2026-08-07/32737_2026-08-07_00.json"
	got, err := joinDataURL(base, link)
	if err != nil {
		t.Fatal(err)
	}
	want := base + "/" + link
	if got != want {
		t.Fatalf("joinDataURL=%q want %q", got, want)
	}

	// Absolute / host-bearing refs must fail even if pattern validation were skipped.
	for _, evil := range []string{
		"https://evil.example/steal.json",
		"//evil.example/steal.json",
		"/etc/passwd",
	} {
		if _, err := joinDataURL(base, evil); err == nil {
			t.Fatalf("joinDataURL accepted %q", evil)
		}
	}

	if _, err := joinDataURL("ftp://data.shmu.example", link); err == nil {
		t.Fatal("expected non-http scheme rejection")
	}
	if _, err := joinDataURL("", link); err == nil {
		t.Fatal("expected empty base rejection")
	}
	if !strings.HasPrefix(want, "https://") {
		t.Fatal("sanity")
	}
}
