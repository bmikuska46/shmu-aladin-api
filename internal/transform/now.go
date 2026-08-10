package transform

import (
	"fmt"
	"math"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

// SelectNow returns the hourly forecast step closest to at.
// offset_seconds is current.dt − at (negative means the step is in the past).
func SelectNow(hourly model.ForecastResponse, at time.Time) (model.NowWeatherResponse, error) {
	if len(hourly.List) == 0 {
		return model.NowWeatherResponse{}, fmt.Errorf("forecast has no hourly steps")
	}

	at = at.UTC()
	target := at.Unix()
	bestIdx := closestHourIndex(hourly.List, target)
	item := hourly.List[bestIdx]
	return model.NowWeatherResponse{
		Code:          "200",
		Message:       0,
		AsOf:          at.Format(time.RFC3339),
		OffsetSeconds: item.DT - target,
		Current:       item,
		City:          hourly.City,
		Meta:          hourly.Meta,
	}, nil
}

// LimitForecastHours returns up to n hourly steps starting from the current clock
// hour (UTC), not from the model run start. n <= 0 leaves the response unchanged.
// If fewer than n steps remain after that point, all remaining steps are returned.
func LimitForecastHours(resp model.ForecastResponse, n int, at time.Time) model.ForecastResponse {
	if n <= 0 || len(resp.List) == 0 {
		return resp
	}

	from := at.UTC().Truncate(time.Hour).Unix()
	start := 0
	for start < len(resp.List) && resp.List[start].DT < from {
		start++
	}
	if start >= len(resp.List) {
		resp.List = []model.ForecastItem{}
		resp.Hours = 0
		return resp
	}

	end := start + n
	if end > len(resp.List) {
		end = len(resp.List)
	}
	resp.List = resp.List[start:end]
	resp.Hours = len(resp.List)
	return resp
}

func closestHourIndex(list []model.ForecastItem, target int64) int {
	bestIdx := 0
	bestAbs := int64(math.MaxInt64)
	for i, item := range list {
		diff := item.DT - target
		if diff < 0 {
			diff = -diff
		}
		if diff < bestAbs {
			bestAbs = diff
			bestIdx = i
		}
	}
	return bestIdx
}
