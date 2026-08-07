package transform

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/shmu"
)

// RenderedForecast is a ready-to-serve API forecast payload.
type RenderedForecast struct {
	JSON []byte
	Gzip []byte
	ETag string
}

// RenderForecast builds the OWM-like API response once and pre-compresses it.
func RenderForecast(raw *shmu.AladinForecast, runtimeTS int64) (RenderedForecast, error) {
	resp, err := ToOWMForecast(raw, runtimeTS)
	if err != nil {
		return RenderedForecast{}, err
	}
	return RenderForecastResponse(resp)
}

// RenderForecastResponse serializes and gzip-compresses an already-built response.
func RenderForecastResponse(resp model.ForecastResponse) (RenderedForecast, error) {
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		return RenderedForecast{}, fmt.Errorf("marshal forecast: %w", err)
	}
	gzipBytes, err := gzipCompress(jsonBytes)
	if err != nil {
		return RenderedForecast{}, fmt.Errorf("gzip forecast: %w", err)
	}
	sum := sha256.Sum256(jsonBytes)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	return RenderedForecast{
		JSON: jsonBytes,
		Gzip: gzipBytes,
		ETag: etag,
	}, nil
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
