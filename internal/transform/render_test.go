package transform_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/shmu"
	"github.com/bmikuska/shmu-weather-api/internal/transform"
)

func TestRenderForecast(t *testing.T) {
	root := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "32737_2026-08-07_00.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw shmu.AladinForecast
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	rendered, err := transform.RenderForecast(&raw, 1786060800)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.JSON) == 0 {
		t.Fatal("expected JSON bytes")
	}
	if len(rendered.Gzip) == 0 {
		t.Fatal("expected gzip bytes")
	}
	if rendered.ETag == "" || rendered.ETag[0] != '"' {
		t.Fatalf("unexpected etag %q", rendered.ETag)
	}
	if len(rendered.Gzip) >= len(rendered.JSON) {
		t.Fatalf("gzip (%d) should be smaller than json (%d)", len(rendered.Gzip), len(rendered.JSON))
	}

	var resp model.ForecastResponse
	if err := json.Unmarshal(rendered.JSON, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != "200" || resp.Cnt == 0 {
		t.Fatalf("unexpected response: code=%s cnt=%d", resp.Code, resp.Cnt)
	}

	zr, err := gzip.NewReader(bytes.NewReader(rendered.Gzip))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	unzipped, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unzipped, rendered.JSON) {
		t.Fatal("gzip payload does not match JSON")
	}
}
