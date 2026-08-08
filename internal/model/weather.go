package model

// WeatherCode is the stable enum used in forecast weather conditions.
type WeatherCode string

const (
	WeatherClear            WeatherCode = "clear"
	WeatherFewClouds        WeatherCode = "few_clouds"
	WeatherScatteredClouds  WeatherCode = "scattered_clouds"
	WeatherBrokenClouds     WeatherCode = "broken_clouds"
	WeatherOvercastClouds   WeatherCode = "overcast_clouds"
	WeatherLightRain        WeatherCode = "light_rain"
	WeatherModerateRain     WeatherCode = "moderate_rain"
	WeatherHeavyRain        WeatherCode = "heavy_rain"
	WeatherLightSnow        WeatherCode = "light_snow"
	WeatherRainAndSnow      WeatherCode = "rain_and_snow"
)

// WeatherCond is a forecast weather condition (code + human-readable description).
type WeatherCond struct {
	Code        WeatherCode `json:"code" xml:"code,attr"`
	Description string      `json:"description" xml:"description,attr"`
}

// WeatherCodeInfo is one entry in the weather-code catalog endpoint.
type WeatherCodeInfo struct {
	Code        WeatherCode `json:"code"`
	Description string      `json:"description"`
}

// WeatherCodesResponse lists every weather code the API can return.
type WeatherCodesResponse struct {
	Codes []WeatherCodeInfo `json:"codes"`
}

// AllWeatherCodes returns the catalog of weather codes in a stable order.
func AllWeatherCodes() []WeatherCodeInfo {
	return []WeatherCodeInfo{
		{Code: WeatherClear, Description: "clear sky"},
		{Code: WeatherFewClouds, Description: "few clouds"},
		{Code: WeatherScatteredClouds, Description: "scattered clouds"},
		{Code: WeatherBrokenClouds, Description: "broken clouds"},
		{Code: WeatherOvercastClouds, Description: "overcast clouds"},
		{Code: WeatherLightRain, Description: "light rain"},
		{Code: WeatherModerateRain, Description: "moderate rain"},
		{Code: WeatherHeavyRain, Description: "heavy intensity rain"},
		{Code: WeatherLightSnow, Description: "light snow"},
		{Code: WeatherRainAndSnow, Description: "rain and snow"},
	}
}

// Cond returns a WeatherCond for the given catalog code.
func (c WeatherCode) Cond() WeatherCond {
	for _, info := range AllWeatherCodes() {
		if info.Code == c {
			return WeatherCond{Code: info.Code, Description: info.Description}
		}
	}
	return WeatherCond{Code: c, Description: string(c)}
}
