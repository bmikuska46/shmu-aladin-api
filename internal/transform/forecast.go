package transform

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/bmikuska/shmu-weather-api/internal/model"
	"github.com/bmikuska/shmu-weather-api/internal/shmu"
)

// ToOWMForecast converts SHMU ALADIN time series into an OpenWeatherMap-like forecast.
func ToOWMForecast(raw *shmu.AladinForecast, runtimeTS int64) (model.ForecastResponse, error) {
	id, _ := strconv.ParseInt(raw.SIID, 10, 64)
	lat, _ := shmu.ParseFloat(raw.Lat)
	lon, _ := shmu.ParseFloat(raw.Lon)

	var elev *float64
	if e, err := shmu.ParseFloat(raw.Elevation); err == nil {
		elev = &e
	}

	timestamps := collectTimestamps(raw)
	items := make([]model.ForecastItem, 0, len(timestamps))

	temp := indexSeries(raw.AirTemperatureAt2m)
	tempMin := indexSeries(raw.MinimumTemperatureInTheLastHour)
	tempMax := indexSeries(raw.MaximumTemperatureInTheLastHour)
	pressure := indexSeries(raw.MeanSeaLevelPressure)
	cloudsAll := indexSeries(raw.TotalCloudCover)
	cloudsLow := indexSeries(raw.LowCloudCover)
	cloudsMid := indexSeries(raw.MediumCloudCover)
	cloudsHigh := indexSeries(raw.HighCloudCover)
	windSpeed := indexSeries(raw.WindSpeedAt10m)
	windDeg := indexSeries(raw.WindDirectionAt10m)
	windGust := indexSeries(raw.WindGustAt10m)
	rain := indexSeries(raw.TotalPrecipitation)
	snow := indexSeries(raw.Snowfall)

	for _, ts := range timestamps {
		item := model.ForecastItem{
			DT:    ts,
			DTTxt: time.Unix(ts, 0).UTC().Format("2006-01-02 15:04:05"),
			Main: model.ForecastMain{
				Temp:     temp[ts],
				TempMin:  tempMin[ts],
				TempMax:  tempMax[ts],
				Pressure: pressure[ts],
				SeaLevel: pressure[ts],
			},
			Clouds: model.ForecastClouds{
				All:  cloudsAll[ts],
				Low:  cloudsLow[ts],
				Mid:  cloudsMid[ts],
				High: cloudsHigh[ts],
			},
			Wind: model.ForecastWind{
				Speed: windSpeed[ts],
				Deg:   windDeg[ts],
				Gust:  windGust[ts],
			},
			Weather: deriveWeather(rain[ts], snow[ts], cloudsAll[ts]),
		}
		if v := rain[ts]; v != nil && *v > 0 {
			item.Rain = &model.Precip1h{H1: round3(*v)}
		}
		if v := snow[ts]; v != nil && *v > 0 {
			item.Snow = &model.Precip1h{H1: round3(*v)}
		}
		items = append(items, item)
	}

	runtime := ""
	if runtimeTS > 0 {
		runtime = time.Unix(runtimeTS, 0).UTC().Format(time.RFC3339)
	}

	return model.ForecastResponse{
		Code:    "200",
		Message: 0,
		Cnt:     len(items),
		List:    items,
		City: model.ForecastCity{
			ID:        id,
			Name:      raw.LocationName,
			Coord:     model.Coord{Lat: lat, Lon: lon},
			Country:   "SK",
			Elevation: elev,
			Timezone:  7200, // Europe/Bratislava CET/CEST approx; clients should prefer UTC dt
		},
		Meta: model.ForecastMeta{
			Model:        raw.ModelName,
			DataDateTime: raw.DataDateTime,
			Source:       "SHMU ALADIN",
			Runtime:      runtime,
		},
	}, nil
}

func collectTimestamps(raw *shmu.AladinForecast) []int64 {
	seen := map[int64]struct{}{}
	var order []int64
	add := func(s shmu.Series) {
		for _, pt := range s.Data {
			if _, ok := seen[pt.TS]; ok {
				continue
			}
			seen[pt.TS] = struct{}{}
			order = append(order, pt.TS)
		}
	}
	// Prefer temperature series for the main timeline (excludes Orography single point if first).
	add(raw.AirTemperatureAt2m)
	if len(order) == 0 {
		add(raw.TotalCloudCover)
	}
	return order
}

func indexSeries(s shmu.Series) map[int64]*float64 {
	out := make(map[int64]*float64, len(s.Data))
	for _, pt := range s.Data {
		if pt.Value == nil {
			out[pt.TS] = nil
			continue
		}
		rv := round3(*pt.Value)
		out[pt.TS] = &rv
	}
	return out
}

func deriveWeather(rain, snow, clouds *float64) []model.WeatherCond {
	r := 0.0
	s := 0.0
	c := 0.0
	if rain != nil {
		r = *rain
	}
	if snow != nil {
		s = *snow
	}
	if clouds != nil {
		c = *clouds
	}

	switch {
	case s > 0 && r > 0:
		return []model.WeatherCond{{ID: 616, Main: "Snow", Description: "rain and snow", Icon: "13d"}}
	case s > 0:
		return []model.WeatherCond{{ID: 600, Main: "Snow", Description: "light snow", Icon: "13d"}}
	case r >= 7.5:
		return []model.WeatherCond{{ID: 502, Main: "Rain", Description: "heavy intensity rain", Icon: "10d"}}
	case r >= 2.5:
		return []model.WeatherCond{{ID: 501, Main: "Rain", Description: "moderate rain", Icon: "10d"}}
	case r > 0:
		return []model.WeatherCond{{ID: 500, Main: "Rain", Description: "light rain", Icon: "10d"}}
	case c >= 85:
		return []model.WeatherCond{{ID: 804, Main: "Clouds", Description: "overcast clouds", Icon: "04d"}}
	case c >= 51:
		return []model.WeatherCond{{ID: 803, Main: "Clouds", Description: "broken clouds", Icon: "04d"}}
	case c >= 25:
		return []model.WeatherCond{{ID: 802, Main: "Clouds", Description: "scattered clouds", Icon: "03d"}}
	case c > 0:
		return []model.WeatherCond{{ID: 801, Main: "Clouds", Description: "few clouds", Icon: "02d"}}
	default:
		return []model.WeatherCond{{ID: 800, Main: "Clear", Description: "clear sky", Icon: "01d"}}
	}
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// ProductsToModel converts SHMU products payload into API model.
func ProductsToModel(raw *shmu.RawProductsResponse) (model.StationProducts, error) {
	lat, err := shmu.ParseFloat(raw.Station.Lat)
	if err != nil {
		return model.StationProducts{}, fmt.Errorf("lat: %w", err)
	}
	lon, err := shmu.ParseFloat(raw.Station.Lon)
	if err != nil {
		return model.StationProducts{}, fmt.Errorf("lon: %w", err)
	}

	out := model.StationProducts{
		Station: model.Station{
			ID:           raw.Station.StationID,
			Name:         raw.Station.StationName,
			Lat:          lat,
			Lon:          lon,
			DistrictCode: raw.Station.DistrictCode,
		},
		Data: make([]model.Product, 0, len(raw.Data)),
	}
	for _, p := range raw.Data {
		rt, _ := time.Parse("2006-01-02 15:04:05", p.DTRuntime)
		out.Data = append(out.Data, model.Product{
			Type:      p.Type,
			Runtime:   rt.UTC(),
			RuntimeTS: p.Runtime,
			FileLink:  p.FileLink,
		})
	}
	return out, nil
}

func LatestAladinProduct(products []model.Product) (model.Product, bool) {
	var best model.Product
	found := false
	for _, p := range products {
		if p.Type != "aladin" {
			continue
		}
		if !found || p.RuntimeTS > best.RuntimeTS {
			best = p
			found = true
		}
	}
	return best, found
}
