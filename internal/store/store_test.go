package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

func TestSaveForecastKeepsOnlyNewestRuntimes(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	mustUpsertStation(t, st, 1)

	for _, rt := range []int64{100, 200, 300, 400} {
		if err := st.SaveForecast(ctx, 1, "aladin", rt, "link", map[string]any{"rt": rt}); err != nil {
			t.Fatalf("SaveForecast(%d): %v", rt, err)
		}
	}

	runtimes, err := st.ListForecastRuntimes(ctx, 1, "aladin")
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{400, 300}
	if len(runtimes) != len(want) {
		t.Fatalf("got %d runtimes %v, want %v", len(runtimes), runtimes, want)
	}
	for i := range want {
		if runtimes[i] != want[i] {
			t.Fatalf("runtimes = %v, want %v", runtimes, want)
		}
	}

	if _, err := st.GetForecastPayload(ctx, 1, "aladin", 200); err != ErrNotFound {
		t.Fatalf("expected pruned runtime 200 to be gone, got err=%v", err)
	}
	if _, err := st.GetForecastPayload(ctx, 1, "aladin", 400); err != nil {
		t.Fatalf("expected newest runtime retained: %v", err)
	}
}

func TestMigratePrunesExistingForecastHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mustUpsertStation(t, st, 7)

	// Bypass SaveForecast pruning to seed unbounded history.
	for _, rt := range []int64{10, 20, 30, 40, 50} {
		_, err := st.write.ExecContext(ctx, `
INSERT INTO forecasts (station_id, product_type, runtime_ts, file_link, payload_json, fetched_at)
VALUES (?, 'aladin', ?, 'link', '{}', '2026-01-01T00:00:00Z')`, 7, rt)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	runtimes, err := st.ListForecastRuntimes(ctx, 7, "aladin")
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{50, 40}
	if len(runtimes) != len(want) {
		t.Fatalf("after migrate got %v, want %v", runtimes, want)
	}
	for i := range want {
		if runtimes[i] != want[i] {
			t.Fatalf("after migrate got %v, want %v", runtimes, want)
		}
	}
}

func TestSaveForecastDoesNotAffectOtherStations(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	mustUpsertStation(t, st, 1)
	mustUpsertStation(t, st, 2)

	for _, rt := range []int64{100, 200, 300} {
		if err := st.SaveForecast(ctx, 1, "aladin", rt, "a", map[string]any{"rt": rt}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveForecast(ctx, 2, "aladin", 999, "b", map[string]any{"rt": 999}); err != nil {
		t.Fatal(err)
	}

	r1, err := st.ListForecastRuntimes(ctx, 1, "aladin")
	if err != nil {
		t.Fatal(err)
	}
	if len(r1) != ForecastRuntimesToKeep || r1[0] != 300 || r1[1] != 200 {
		t.Fatalf("station 1 runtimes = %v", r1)
	}
	r2, err := st.ListForecastRuntimes(ctx, 2, "aladin")
	if err != nil {
		t.Fatal(err)
	}
	if len(r2) != 1 || r2[0] != 999 {
		t.Fatalf("station 2 runtimes = %v", r2)
	}
}

func TestStationIDsMissingCurrentForecast(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	mustUpsertStation(t, st, 1)
	mustUpsertStation(t, st, 2)
	mustUpsertStation(t, st, 3)

	if err := st.SaveCurrentForecast(ctx, CurrentForecast{
		StationID: 1, RuntimeTS: 100,
		ResponseJSON: []byte(`{}`), ResponseGzip: []byte("x"), ETag: `"a"`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCurrentForecast(ctx, CurrentForecast{
		StationID: 2, RuntimeTS: 99, // wrong runtime
		ResponseJSON: []byte(`{}`), ResponseGzip: []byte("x"), ETag: `"b"`,
	}); err != nil {
		t.Fatal(err)
	}

	missing, err := st.StationIDsMissingCurrentForecast(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 2 || missing[0] != 2 || missing[1] != 3 {
		t.Fatalf("missing = %v, want [2 3]", missing)
	}
}

func TestFindNearestStation(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.FindNearestStation(ctx, 48.15, 17.10); err != ErrNotFound {
		t.Fatalf("empty DB: got err=%v, want ErrNotFound", err)
	}

	err := st.UpsertStations(ctx, []model.Station{
		{ID: 1, Name: "Far", Lat: 49.0, Lon: 20.0, DistrictCode: 1},
		{ID: 32737, Name: "Bratislava", Lat: 48.156, Lon: 17.105, DistrictCode: 101},
		{ID: 3, Name: "Nearish", Lat: 48.20, Lon: 17.20, DistrictCode: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := st.FindNearestStation(ctx, 48.15, 17.10)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 32737 {
		t.Fatalf("nearest id=%d name=%q, want 32737 Bratislava", got.ID, got.Name)
	}
}

func TestOpenUsesSeparateReadWritePools(t *testing.T) {
	st := openTestStore(t)
	if st.write == nil || st.read == nil {
		t.Fatal("expected write and read pools")
	}
	if st.write == st.read {
		t.Fatal("write and read should be distinct *sql.DB pools")
	}
	if got := st.write.Stats().MaxOpenConnections; got != writerMaxConns {
		t.Fatalf("writer MaxOpenConnections = %d, want %d", got, writerMaxConns)
	}
	if got := st.read.Stats().MaxOpenConnections; got != readerMaxConns {
		t.Fatalf("reader MaxOpenConnections = %d, want %d", got, readerMaxConns)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func mustUpsertStation(t *testing.T, st *Store, id int64) {
	t.Helper()
	err := st.UpsertStations(context.Background(), []model.Station{{
		ID: id, Name: "Test", Lat: 1, Lon: 2, DistrictCode: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
}
