package transform

import (
	"math"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/geo"
	"github.com/bmikuska/shmu-weather-api/internal/model"
)

// AggregateDaily builds calendar-day summaries from an hourly forecast response.
// days <= 0 means return every available local day.
func AggregateDaily(hourly model.ForecastResponse, days int) model.DailyForecastResponse {
	byDate := map[string][]model.ForecastItem{}
	var order []string
	for _, item := range hourly.List {
		d := geo.LocalDateString(item.DT)
		if _, ok := byDate[d]; !ok {
			order = append(order, d)
		}
		byDate[d] = append(byDate[d], item)
	}

	if days > 0 && days < len(order) {
		order = order[:days]
	}

	list := make([]model.DailyForecastDay, 0, len(order))
	for _, d := range order {
		list = append(list, aggregateOneDay(d, byDate[d], hourly.City.Coord.Lat, hourly.City.Coord.Lon))
	}

	return model.DailyForecastResponse{
		Code:    "200",
		Message: 0,
		Cnt:     len(list),
		List:    list,
		City:    hourly.City,
		Meta:    hourly.Meta,
	}
}

func aggregateOneDay(date string, items []model.ForecastItem, lat, lon float64) model.DailyForecastDay {
	day := model.DailyForecastDay{Date: date}
	if parsed, err := time.ParseInLocation("2006-01-02", date, geo.Bratislava); err == nil {
		day.DateIndex = geo.WeekdayIndex(parsed.Weekday())
	}

	var (
		tempMin, tempMax               *float64
		precipSum, rainSum, snowSum    float64
		havePrecip, haveRain, haveSnow bool
		precipHours                    int
		havePrecipHours                bool
		windMax, gustMax               *float64
		cloudSum                       float64
		cloudN                         int
	)

	weatherScore := map[model.WeatherCode]weatherAccum{}

	for _, it := range items {
		tMin := firstFloat(it.Main.TempMin, it.Main.Temp)
		tMax := firstFloat(it.Main.TempMax, it.Main.Temp)
		if tMin != nil {
			tempMin = minPtr(tempMin, *tMin)
		}
		if tMax != nil {
			tempMax = maxPtr(tempMax, *tMax)
		}

		var rainH, snowH *float64
		if it.Rain != nil {
			v := it.Rain.H1
			rainH = &v
			haveRain = true
			havePrecip = true
			precipSum += v
		}
		if it.Snow != nil {
			v := it.Snow.H1
			snowH = &v
			haveSnow = true
			snowSum += v
		}
		if rainH != nil {
			liquid := *rainH
			if snowH != nil {
				// Total_precipitation includes snowfall water equivalent.
				liquid = math.Max(0, *rainH-*snowH)
			}
			rainSum += liquid
		}

		if rainH != nil || snowH != nil {
			havePrecipHours = true
			r, s := 0.0, 0.0
			if rainH != nil {
				r = *rainH
			}
			if snowH != nil {
				s = *snowH
			}
			if r > 0 || s > 0 {
				precipHours++
			}
		}

		if it.Wind.Speed != nil {
			windMax = maxPtr(windMax, *it.Wind.Speed)
		}
		if it.Wind.Gust != nil {
			gustMax = maxPtr(gustMax, *it.Wind.Gust)
		}
		if it.Clouds.All != nil {
			cloudSum += *it.Clouds.All
			cloudN++
		}

		if len(it.Weather) > 0 {
			w := it.Weather[0]
			acc := weatherScore[w.Code]
			acc.cond = w
			acc.hours++
			peak := 0.0
			if rainH != nil {
				peak += *rainH
			}
			if snowH != nil {
				peak += *snowH
			}
			if peak > acc.peakPrecip {
				acc.peakPrecip = peak
			}
			weatherScore[w.Code] = acc
		}
	}

	day.TempMin = tempMin
	day.TempMax = tempMax
	if havePrecip {
		v := round3(precipSum)
		day.PrecipitationTotal = &v
	}
	if haveRain {
		v := round3(rainSum)
		day.RainTotal = &v
	}
	if haveSnow {
		v := round3(snowSum)
		day.SnowTotal = &v
	}
	if havePrecipHours {
		h := precipHours
		day.PrecipitationHours = &h
	}
	day.WindSpeedMax = windMax
	day.WindGustMax = gustMax
	if cloudN > 0 {
		v := round3(cloudSum / float64(cloudN))
		day.CloudCoverMean = &v
	}
	day.Weather = pickRepresentativeWeather(weatherScore)

	if parsed, err := time.ParseInLocation("2006-01-02", date, geo.Bratislava); err == nil {
		if rise, set, ok := geo.SunriseSunset(parsed, lat, lon); ok {
			day.Sunrise = rise.Format(time.RFC3339)
			day.Sunset = set.Format(time.RFC3339)
		}
		start, end := parsed.Unix(), parsed.AddDate(0, 0, 1).Unix()
		// A complete local day needs an hourly step in every hour of [start, end).
		// 23h spring-forward and 25h fall-back days still count as complete when
		// every local hour present in that civil day is covered.
		covered := map[int64]bool{}
		for _, it := range items {
			covered[it.DT] = true
		}
		expected := 0
		missing := 0
		for ts := start; ts < end; ts += 3600 {
			expected++
			if !covered[ts] {
				missing++
			}
		}
		day.IsPartial = expected == 0 || missing > 0
	}

	return day
}

type weatherAccum struct {
	cond       model.WeatherCond
	hours      int
	peakPrecip float64
}

// Severity priority: mixed > snow > heavy rain > moderate > light > clouds > clear.
var weatherPriority = map[model.WeatherCode]int{
	model.WeatherRainAndSnow:     100,
	model.WeatherLightSnow:       90,
	model.WeatherHeavyRain:       80,
	model.WeatherModerateRain:    70,
	model.WeatherLightRain:       60,
	model.WeatherOvercastClouds:  50,
	model.WeatherBrokenClouds:    40,
	model.WeatherScatteredClouds: 30,
	model.WeatherFewClouds:       20,
	model.WeatherClear:           10,
}

func pickRepresentativeWeather(scores map[model.WeatherCode]weatherAccum) model.WeatherCond {
	var best weatherAccum
	found := false
	for _, acc := range scores {
		if !found {
			best = acc
			found = true
			continue
		}
		bp := weatherPriority[best.cond.Code]
		ap := weatherPriority[acc.cond.Code]
		if ap > bp ||
			(ap == bp && acc.hours > best.hours) ||
			(ap == bp && acc.hours == best.hours && acc.peakPrecip > best.peakPrecip) ||
			(ap == bp && acc.hours == best.hours && acc.peakPrecip == best.peakPrecip && acc.cond.Code < best.cond.Code) {
			best = acc
		}
	}
	if !found {
		return model.WeatherClear.Cond()
	}
	return best.cond
}

func firstFloat(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func minPtr(cur *float64, v float64) *float64 {
	if cur == nil || v < *cur {
		nv := v
		return &nv
	}
	return cur
}

func maxPtr(cur *float64, v float64) *float64 {
	if cur == nil || v > *cur {
		nv := v
		return &nv
	}
	return cur
}
