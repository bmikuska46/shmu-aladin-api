package normalize_test

import (
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/normalize"
)

func TestFold(t *testing.T) {
	cases := map[string]string{
		"Bánovce nad Bebravou": "banovce nad bebravou",
		"Bratislava (centrum)": "bratislava (centrum)",
		"Žilina":               "zilina",
		"Košice":               "kosice",
		"Čadca":                "cadca",
		"  Abrahám  ":          "abraham",
	}
	for in, want := range cases {
		if got := normalize.Fold(in); got != want {
			t.Fatalf("Fold(%q)=%q want %q", in, got, want)
		}
	}
}
