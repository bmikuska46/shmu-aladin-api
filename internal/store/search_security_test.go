package store

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func TestListStationsRejectsSQLAndFTSInjection(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	err := st.UpsertStations(ctx, []model.Station{
		{ID: 1, Name: "Bratislava", Lat: 48.1, Lon: 17.1, DistrictCode: 1},
		{ID: 2, Name: "Košice", Lat: 48.7, Lon: 21.2, DistrictCode: 2},
		{ID: 3, Name: "Banská Bystrica", Lat: 48.7, Lon: 19.1, DistrictCode: 3},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Normal search still works.
	list, err := st.ListStations(ctx, "bratislava", 1, 50)
	if err != nil {
		t.Fatalf("normal search: %v", err)
	}
	if list.Total != 1 || len(list.Stations) != 1 || list.Stations[0].ID != 1 {
		t.Fatalf("normal search got %+v", list)
	}

	payloads := []string{
		`bratislava'; DROP TABLE stations;--`,
		`bratislava" OR 1=1 --`,
		`*"`,
		`AND OR NOT NEAR`,
		`name_folded:bratislava`,
		`bratislava^`,
		`%`,
		`_`,
		`%kosice%`,
		`____`,
		`" OR ""="`,
		strings.Repeat("a", 500),
		"bratislava\x00kosice",
	}

	for _, q := range payloads {
		t.Run(truncateRunes(q, 40), func(t *testing.T) {
			got, err := st.ListStations(ctx, q, 1, 50)
			if err != nil {
				t.Fatalf("injection-like query %q must not error: %v", q, err)
			}
			// LIKE wildcards and FTS operators must not match every station.
			if got.Total > 1 {
				t.Fatalf("query %q matched %d stations (ids=%v); expected at most 1",
					q, got.Total, stationIDs(got.Stations))
			}
		})
	}

	// Literal underscore / percent in the name must still be findable when escaped.
	err = st.UpsertStations(ctx, []model.Station{
		{ID: 4, Name: "Test_Station", Lat: 1, Lon: 1, DistrictCode: 1},
		{ID: 5, Name: "100% humidity", Lat: 2, Lon: 2, DistrictCode: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.ListStations(ctx, "test_station", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Stations[0].ID != 4 {
		t.Fatalf("literal underscore search got %+v", got)
	}
	got, err = st.ListStations(ctx, "100% humidity", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Stations[0].ID != 5 {
		t.Fatalf("literal percent search got %+v", got)
	}
}

func TestBuildFTSQuerySanitizesOperators(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"bratislava", `"bratislava"*`},
		{`bra"tislava`, `"bratislava"*`},
		{"AND OR", `"AND"* "OR"*`},
		{"name:foo", `"namefoo"*`},
		{"foo*bar", `"foobar"*`},
		{"%%%", `""`},
		{"", `""`},
		{"banovce nad bebravou", `"banovce"* "nad"* "bebravou"*`},
	}
	for _, tt := range tests {
		if got := buildFTSQuery(tt.in); got != tt.want {
			t.Fatalf("buildFTSQuery(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`a%b_c\d`); got != `a\%b\_c\\d` {
		t.Fatalf("escapeLike = %q", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	s := strings.Repeat("ž", 150)
	got := truncateRunes(s, maxSearchQueryRunes)
	if utf8.RuneCountInString(got) != maxSearchQueryRunes {
		t.Fatalf("rune count=%d want %d", utf8.RuneCountInString(got), maxSearchQueryRunes)
	}
}

func stationIDs(stations []model.Station) []int64 {
	out := make([]int64, len(stations))
	for i, st := range stations {
		out[i] = st.ID
	}
	return out
}
