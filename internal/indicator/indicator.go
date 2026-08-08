package indicator

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/model"
)

// Supported indicator type names (v1).
var SupportedTypes = []string{
	"frost", "heat", "wind", "heavy_rain", "snow", "mixed_precipitation", "low_cloud_mountain",
}

// Evaluate runs the requested indicator rules against hourly forecast items.
// An empty or nil types filter evaluates all supported types.
func Evaluate(hourly model.ForecastResponse, types []string) ([]model.Indicator, error) {
	wanted, err := normalizeTypes(types)
	if err != nil {
		return nil, err
	}

	runtime := hourly.Meta.Runtime
	var out []model.Indicator
	for _, typ := range wanted {
		switch typ {
		case "frost":
			out = append(out, evalFrost(hourly.List, runtime)...)
		case "heat":
			out = append(out, evalHeat(hourly.List, runtime)...)
		case "wind":
			out = append(out, evalWind(hourly.List, runtime)...)
		case "heavy_rain":
			out = append(out, evalHeavyRain(hourly.List, runtime)...)
		case "snow":
			out = append(out, evalSnow(hourly.List, runtime)...)
		case "mixed_precipitation":
			out = append(out, evalMixed(hourly.List, runtime)...)
		case "low_cloud_mountain":
			out = append(out, evalLowCloudMountain(hourly.List, runtime)...)
		}
	}
	return out, nil
}

func normalizeTypes(types []string) ([]string, error) {
	if len(types) == 0 {
		return append([]string{}, SupportedTypes...), nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range types {
		t := strings.TrimSpace(strings.ToLower(raw))
		if t == "" {
			continue
		}
		if !isSupported(t) {
			return nil, fmt.Errorf("unknown indicator type %q", raw)
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return append([]string{}, SupportedTypes...), nil
	}
	// Preserve SupportedTypes order for stable output.
	ordered := make([]string, 0, len(out))
	for _, t := range SupportedTypes {
		if seen[t] {
			ordered = append(ordered, t)
		}
	}
	return ordered, nil
}

func isSupported(t string) bool {
	for _, s := range SupportedTypes {
		if s == t {
			return true
		}
	}
	return false
}

type hourPoint struct {
	ts    int64
	temp  *float64
	speed *float64
	gust  *float64
	rain  *float64
	snow  *float64
	low   *float64
}

func pointsFrom(list []model.ForecastItem) []hourPoint {
	out := make([]hourPoint, 0, len(list))
	for _, it := range list {
		var rain, snow *float64
		if it.Rain != nil {
			v := it.Rain.H1
			rain = &v
		}
		if it.Snow != nil {
			v := it.Snow.H1
			snow = &v
		}
		out = append(out, hourPoint{
			ts:    it.DT,
			temp:  it.Main.Temp,
			speed: it.Wind.Speed,
			gust:  it.Wind.Gust,
			rain:  rain,
			snow:  snow,
			low:   it.Clouds.Low,
		})
	}
	return out
}

func base(typ, version, runtime string) model.Indicator {
	return model.Indicator{
		Type:          typ,
		SourceKind:    "forecast",
		SourceRuntime: runtime,
		RuleVersion:   version,
		Official:      false,
		Available:     true,
	}
}

func unavailable(typ, version, runtime, reason string) model.Indicator {
	ind := base(typ, version, runtime)
	ind.Available = false
	ind.Level = "unknown"
	ind.Summary = reason
	ind.UnavailableReason = reason
	return ind
}

func windowFields(from, to int64) (string, string) {
	return time.Unix(from, 0).UTC().Format(time.RFC3339),
		time.Unix(to, 0).UTC().Format(time.RFC3339)
}

// mergeAdjacent merges consecutive hourly windows (end+3600==next.start).
func mergeAdjacent(windows []model.Indicator, mergeInputs func(a, b model.Indicator) map[string]any, mergeLevel func(a, b string) string, summary func(model.Indicator) string) []model.Indicator {
	if len(windows) == 0 {
		return windows
	}
	out := []model.Indicator{windows[0]}
	for i := 1; i < len(windows); i++ {
		prev := &out[len(out)-1]
		cur := windows[i]
		prevTo, _ := time.Parse(time.RFC3339, prev.ValidTo)
		curFrom, _ := time.Parse(time.RFC3339, cur.ValidFrom)
		if curFrom.Unix() == prevTo.Unix()+3600 {
			prev.ValidTo = cur.ValidTo
			prev.Level = mergeLevel(prev.Level, cur.Level)
			prev.Inputs = mergeInputs(*prev, cur)
			prev.Summary = summary(*prev)
			continue
		}
		out = append(out, cur)
	}
	return out
}

func worseLevel(a, b string) string {
	rank := map[string]int{"low": 1, "moderate": 2, "high": 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func evalFrost(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "frost/v1"
	pts := pointsFrom(list)
	haveTemp := false
	for _, p := range pts {
		if p.temp != nil {
			haveTemp = true
			break
		}
	}
	if !haveTemp {
		return []model.Indicator{unavailable("frost", version, runtime, "temperature data unavailable")}
	}

	var windows []model.Indicator
	var run []hourPoint
	flush := func() {
		if len(run) < 3 {
			run = nil
			return
		}
		minT := *run[0].temp
		for _, p := range run {
			if *p.temp < minT {
				minT = *p.temp
			}
		}
		level := "moderate"
		if minT <= -5 {
			level = "high"
		}
		from, to := windowFields(run[0].ts, run[len(run)-1].ts)
		ind := base("frost", version, runtime)
		ind.Level = level
		ind.ValidFrom = from
		ind.ValidTo = to
		ind.Inputs = map[string]any{
			"minimum_temperature": round1(minT),
			"duration_hours":      len(run),
			"threshold_celsius":   0,
		}
		ind.Summary = fmt.Sprintf("Air temperature is forecast below 0 °C for %d hours (min %.1f °C).", len(run), minT)
		windows = append(windows, ind)
		run = nil
	}

	for _, p := range pts {
		if p.temp != nil && *p.temp < 0 {
			if len(run) > 0 && p.ts != run[len(run)-1].ts+3600 {
				flush()
			}
			run = append(run, p)
			continue
		}
		flush()
	}
	flush()

	if len(windows) == 0 {
		ind := base("frost", version, runtime)
		ind.Level = "low"
		ind.Summary = "No frost risk in the forecast horizon (no 3+ consecutive hours below 0 °C)."
		ind.Inputs = map[string]any{"duration_hours": 0}
		return []model.Indicator{ind}
	}
	return windows
}

func evalHeat(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "heat/v1"
	pts := pointsFrom(list)
	haveTemp := false
	for _, p := range pts {
		if p.temp != nil {
			haveTemp = true
			break
		}
	}
	if !haveTemp {
		return []model.Indicator{unavailable("heat", version, runtime, "temperature data unavailable")}
	}

	var windows []model.Indicator
	var run []hourPoint
	flush := func() {
		if len(run) < 3 {
			run = nil
			return
		}
		maxT := *run[0].temp
		for _, p := range run {
			if *p.temp > maxT {
				maxT = *p.temp
			}
		}
		level := "moderate"
		if maxT >= 35 {
			level = "high"
		}
		from, to := windowFields(run[0].ts, run[len(run)-1].ts)
		ind := base("heat", version, runtime)
		ind.Level = level
		ind.ValidFrom = from
		ind.ValidTo = to
		ind.Inputs = map[string]any{
			"maximum_temperature": round1(maxT),
			"duration_hours":      len(run),
			"threshold_celsius":   30,
		}
		ind.Summary = fmt.Sprintf("Air temperature is forecast at or above 30 °C for %d hours (max %.1f °C).", len(run), maxT)
		windows = append(windows, ind)
		run = nil
	}

	for _, p := range pts {
		if p.temp != nil && *p.temp >= 30 {
			if len(run) > 0 && p.ts != run[len(run)-1].ts+3600 {
				flush()
			}
			run = append(run, p)
			continue
		}
		flush()
	}
	flush()

	if len(windows) == 0 {
		ind := base("heat", version, runtime)
		ind.Level = "low"
		ind.Summary = "No heat risk in the forecast horizon (no 3+ consecutive hours at or above 30 °C)."
		ind.Inputs = map[string]any{"duration_hours": 0}
		return []model.Indicator{ind}
	}
	return windows
}

func evalWind(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "wind/v1"
	pts := pointsFrom(list)
	have := false
	for _, p := range pts {
		if p.speed != nil || p.gust != nil {
			have = true
			break
		}
	}
	if !have {
		return []model.Indicator{unavailable("wind", version, runtime, "wind data unavailable")}
	}

	var windows []model.Indicator
	for _, p := range pts {
		speed, gust := 0.0, 0.0
		hasSpeed, hasGust := false, false
		if p.speed != nil {
			speed = *p.speed
			hasSpeed = true
		}
		if p.gust != nil {
			gust = *p.gust
			hasGust = true
		}
		qualifies := (hasSpeed && speed >= 10) || (hasGust && gust >= 15)
		if !qualifies {
			continue
		}
		level := "moderate"
		if (hasSpeed && speed >= 15) || (hasGust && gust >= 25) {
			level = "high"
		}
		from, to := windowFields(p.ts, p.ts)
		ind := base("wind", version, runtime)
		ind.Level = level
		ind.ValidFrom = from
		ind.ValidTo = to
		inputs := map[string]any{}
		if hasSpeed {
			inputs["wind_speed"] = round1(speed)
		}
		if hasGust {
			inputs["wind_gust"] = round1(gust)
		}
		ind.Inputs = inputs
		ind.Summary = fmt.Sprintf("Strong wind: speed %.1f m/s, gust %.1f m/s.", speed, gust)
		windows = append(windows, ind)
	}

	windows = mergeAdjacent(windows,
		func(a, b model.Indicator) map[string]any {
			out := map[string]any{}
			for _, src := range []model.Indicator{a, b} {
				if v, ok := src.Inputs["wind_speed"].(float64); ok {
					if cur, ok := out["wind_speed_max"].(float64); !ok || v > cur {
						out["wind_speed_max"] = v
					}
				}
				if v, ok := src.Inputs["wind_gust"].(float64); ok {
					if cur, ok := out["wind_gust_max"].(float64); !ok || v > cur {
						out["wind_gust_max"] = v
					}
				}
			}
			return out
		},
		worseLevel,
		func(ind model.Indicator) string {
			sp, _ := ind.Inputs["wind_speed_max"].(float64)
			gu, _ := ind.Inputs["wind_gust_max"].(float64)
			return fmt.Sprintf("Strong wind period: peak speed %.1f m/s, peak gust %.1f m/s.", sp, gu)
		},
	)

	if len(windows) == 0 {
		ind := base("wind", version, runtime)
		ind.Level = "low"
		ind.Summary = "No strong-wind risk in the forecast horizon."
		return []model.Indicator{ind}
	}
	return windows
}

func evalHeavyRain(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "heavy_rain/v1"
	pts := pointsFrom(list)
	haveRain := false
	for _, p := range pts {
		if p.rain != nil {
			haveRain = true
			break
		}
	}
	if !haveRain {
		return []model.Indicator{unavailable("heavy_rain", version, runtime, "precipitation data unavailable")}
	}

	thresholds := []struct {
		hours int
		mm    float64
	}{
		{1, 15}, {3, 30}, {6, 40}, {24, 60},
	}

	var windows []model.Indicator
	seen := map[string]bool{}
	for _, th := range thresholds {
		for i := 0; i < len(pts); i++ {
			end := i + th.hours - 1
			if end >= len(pts) {
				break
			}
			// Require contiguous hourly steps.
			if pts[end].ts != pts[i].ts+int64((th.hours-1)*3600) {
				continue
			}
			sum := 0.0
			complete := true
			for j := i; j <= end; j++ {
				if pts[j].rain == nil {
					complete = false
					break
				}
				sum += *pts[j].rain
			}
			if !complete || sum < th.mm {
				continue
			}
			level := "moderate"
			if sum >= th.mm*1.5 {
				level = "high"
			}
			from, to := windowFields(pts[i].ts, pts[end].ts)
			key := fmt.Sprintf("%s|%s|%d", from, to, th.hours)
			if seen[key] {
				continue
			}
			seen[key] = true
			ind := base("heavy_rain", version, runtime)
			ind.Level = level
			ind.ValidFrom = from
			ind.ValidTo = to
			ind.Inputs = map[string]any{
				"window_hours":     th.hours,
				"precipitation_mm": round1(sum),
				"threshold_mm":     th.mm,
			}
			ind.Summary = fmt.Sprintf("Heavy-rain potential: %.1f mm over %d hour(s) (threshold %.0f mm).", sum, th.hours, th.mm)
			windows = append(windows, ind)
		}
	}

	if len(windows) == 0 {
		ind := base("heavy_rain", version, runtime)
		ind.Level = "low"
		ind.Summary = "No heavy-rain potential in the forecast horizon."
		return []model.Indicator{ind}
	}
	return windows
}

func evalSnow(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "snow/v1"
	pts := pointsFrom(list)
	haveSnow, haveTemp := false, false
	for _, p := range pts {
		if p.snow != nil {
			haveSnow = true
		}
		if p.temp != nil {
			haveTemp = true
		}
	}
	if !haveSnow || !haveTemp {
		return []model.Indicator{unavailable("snow", version, runtime, "snowfall or temperature data unavailable")}
	}

	var windows []model.Indicator
	for _, p := range pts {
		if p.snow == nil || p.temp == nil {
			continue
		}
		if *p.snow > 0 && *p.temp <= 2 {
			from, to := windowFields(p.ts, p.ts)
			ind := base("snow", version, runtime)
			ind.Level = "moderate"
			if *p.snow >= 2 || *p.temp <= -2 {
				ind.Level = "high"
			}
			ind.ValidFrom = from
			ind.ValidTo = to
			ind.Inputs = map[string]any{
				"snowfall_mm": round1(*p.snow),
				"temperature": round1(*p.temp),
			}
			ind.Summary = fmt.Sprintf("Snowfall water equivalent %.1f mm with air temperature %.1f °C.", *p.snow, *p.temp)
			windows = append(windows, ind)
		}
	}

	windows = mergeAdjacent(windows,
		func(a, b model.Indicator) map[string]any {
			sa := snowAmount(a.Inputs)
			sb := snowAmount(b.Inputs)
			ta := snowTemp(a.Inputs)
			tb := snowTemp(b.Inputs)
			return map[string]any{
				"snowfall_total_mm":   round1(sa + sb),
				"minimum_temperature": round1(math.Min(ta, tb)),
			}
		},
		worseLevel,
		func(ind model.Indicator) string {
			s := snowAmount(ind.Inputs)
			t := snowTemp(ind.Inputs)
			return fmt.Sprintf("Snow period: %.1f mm water equivalent, minimum temperature %.1f °C.", s, t)
		},
	)

	if len(windows) == 0 {
		ind := base("snow", version, runtime)
		ind.Level = "low"
		ind.Summary = "No snowfall with near-freezing temperatures in the forecast horizon."
		return []model.Indicator{ind}
	}
	return windows
}

func evalMixed(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "mixed_precipitation/v1"
	pts := pointsFrom(list)
	have := false
	for _, p := range pts {
		if p.rain != nil || p.snow != nil {
			have = true
			break
		}
	}
	if !have {
		return []model.Indicator{unavailable("mixed_precipitation", version, runtime, "precipitation data unavailable")}
	}

	var windows []model.Indicator
	for _, p := range pts {
		if p.rain == nil || p.snow == nil {
			continue
		}
		liquid := math.Max(0, *p.rain-*p.snow)
		if liquid > 0 && *p.snow > 0 {
			from, to := windowFields(p.ts, p.ts)
			ind := base("mixed_precipitation", version, runtime)
			ind.Level = "moderate"
			ind.ValidFrom = from
			ind.ValidTo = to
			ind.Inputs = map[string]any{
				"rain_mm": round1(liquid),
				"snow_mm": round1(*p.snow),
			}
			ind.Summary = fmt.Sprintf("Mixed precipitation: liquid %.1f mm and snow %.1f mm.", liquid, *p.snow)
			windows = append(windows, ind)
		}
	}
	windows = mergeAdjacent(windows,
		func(a, b model.Indicator) map[string]any {
			ra, _ := asFloat(a.Inputs["rain_mm"])
			if v, ok := asFloat(a.Inputs["rain_total_mm"]); ok {
				ra = v
			}
			sa, _ := asFloat(a.Inputs["snow_mm"])
			if v, ok := asFloat(a.Inputs["snow_total_mm"]); ok {
				sa = v
			}
			rb, _ := asFloat(b.Inputs["rain_mm"])
			sb, _ := asFloat(b.Inputs["snow_mm"])
			return map[string]any{
				"rain_total_mm": round1(ra + rb),
				"snow_total_mm": round1(sa + sb),
			}
		},
		worseLevel,
		func(ind model.Indicator) string {
			r, _ := asFloat(ind.Inputs["rain_total_mm"])
			s, _ := asFloat(ind.Inputs["snow_total_mm"])
			return fmt.Sprintf("Mixed precipitation period: liquid %.1f mm and snow %.1f mm.", r, s)
		},
	)
	if len(windows) == 0 {
		ind := base("mixed_precipitation", version, runtime)
		ind.Level = "low"
		ind.Summary = "No mixed rain and snow in the forecast horizon."
		return []model.Indicator{ind}
	}
	return windows
}

func evalLowCloudMountain(list []model.ForecastItem, runtime string) []model.Indicator {
	const version = "low_cloud_mountain/v1"
	pts := pointsFrom(list)
	haveCloud := false
	for _, p := range pts {
		if p.low != nil {
			haveCloud = true
			break
		}
	}
	if !haveCloud {
		return []model.Indicator{unavailable("low_cloud_mountain", version, runtime, "low cloud cover data unavailable")}
	}

	var windows []model.Indicator
	for _, p := range pts {
		if p.low == nil || *p.low < 80 {
			continue
		}
		freezing := p.temp != nil && *p.temp <= 0
		snowing := p.snow != nil && *p.snow > 0
		gusty := p.gust != nil && *p.gust >= 15
		if !(freezing || snowing || gusty) {
			continue
		}
		from, to := windowFields(p.ts, p.ts)
		ind := base("low_cloud_mountain", version, runtime)
		ind.Level = "moderate"
		if (freezing && snowing) || (gusty && *p.low >= 90) {
			ind.Level = "high"
		}
		ind.ValidFrom = from
		ind.ValidTo = to
		ind.Inputs = map[string]any{"low_cloud_cover": round1(*p.low)}
		if p.temp != nil {
			ind.Inputs["temperature"] = round1(*p.temp)
		}
		if p.snow != nil {
			ind.Inputs["snowfall_mm"] = round1(*p.snow)
		}
		if p.gust != nil {
			ind.Inputs["wind_gust"] = round1(*p.gust)
		}
		ind.Summary = "Low cloud with freezing temperature, snowfall, or strong gusts (station-level model guidance)."
		windows = append(windows, ind)
	}
	windows = mergeAdjacent(windows,
		func(a, b model.Indicator) map[string]any {
			out := map[string]any{}
			for k, v := range a.Inputs {
				out[k] = v
			}
			if v, ok := asFloat(b.Inputs["low_cloud_cover"]); ok {
				if cur, ok := asFloat(out["low_cloud_cover"]); !ok || v > cur {
					out["low_cloud_cover"] = v
				}
			}
			if v, ok := asFloat(b.Inputs["wind_gust"]); ok {
				if cur, ok := asFloat(out["wind_gust"]); !ok || v > cur {
					out["wind_gust"] = v
				}
			}
			if v, ok := asFloat(b.Inputs["temperature"]); ok {
				if cur, ok := asFloat(out["temperature"]); !ok || v < cur {
					out["temperature"] = v
				}
			}
			return out
		},
		worseLevel,
		func(model.Indicator) string {
			return "Low-cloud mountain caution period (station-level model guidance, not trail-level visibility)."
		},
	)
	if len(windows) == 0 {
		ind := base("low_cloud_mountain", version, runtime)
		ind.Level = "low"
		ind.Summary = "No low-cloud mountain caution in the forecast horizon."
		return []model.Indicator{ind}
	}
	return windows
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func snowAmount(inputs map[string]any) float64 {
	if v, ok := asFloat(inputs["snowfall_total_mm"]); ok {
		return v
	}
	v, _ := asFloat(inputs["snowfall_mm"])
	return v
}

func snowTemp(inputs map[string]any) float64 {
	if v, ok := asFloat(inputs["minimum_temperature"]); ok {
		return v
	}
	v, _ := asFloat(inputs["temperature"])
	return v
}
