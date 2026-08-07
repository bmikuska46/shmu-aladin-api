package model

import "time"

type Station struct {
	XMLName      struct{} `json:"-" xml:"station"`
	ID           int64    `json:"id" xml:"id"`
	Name         string   `json:"name" xml:"name"`
	Lat          float64  `json:"lat" xml:"lat"`
	Lon          float64  `json:"lon" xml:"lon"`
	DistrictCode int      `json:"district_code" xml:"district_code"`
}

type StationList struct {
	XMLName    struct{}  `json:"-" xml:"stations"`
	Stations   []Station `json:"stations" xml:"station"`
	Total      int       `json:"total" xml:"total,attr"`
	Page       int       `json:"page" xml:"page,attr"`
	PageSize   int       `json:"page_size" xml:"page_size,attr"`
	TotalPages int       `json:"total_pages" xml:"total_pages,attr"`
	Query      string    `json:"query,omitempty" xml:"query,attr,omitempty"`
}

type Product struct {
	Type      string    `json:"type" xml:"type"`
	Runtime   time.Time `json:"dt_runtime" xml:"dt_runtime"`
	RuntimeTS int64     `json:"runtime" xml:"runtime"`
	FileLink  string    `json:"file_link" xml:"file_link"`
}

type StationProducts struct {
	XMLName struct{}  `json:"-" xml:"station_products"`
	Station Station   `json:"station" xml:"station"`
	Data    []Product `json:"data" xml:"product"`
}

// ForecastResponse mirrors OpenWeatherMap 5-day / 3-hour forecast shape,
// adapted for hourly ALADIN model output from SHMU.
type ForecastResponse struct {
	XMLName struct{}       `json:"-" xml:"forecast"`
	Code    string         `json:"code" xml:"code"`
	Message float64        `json:"message" xml:"message"`
	Cnt     int            `json:"cnt" xml:"cnt"`
	List    []ForecastItem `json:"list" xml:"list>item"`
	City    ForecastCity   `json:"city" xml:"city"`
	Meta    ForecastMeta   `json:"meta" xml:"meta"`
}

type ForecastCity struct {
	ID        int64       `json:"id" xml:"id"`
	Name      string      `json:"name" xml:"name"`
	Coord     Coord       `json:"coord" xml:"coord"`
	Country   string      `json:"country" xml:"country"`
	Elevation *float64    `json:"elevation,omitempty" xml:"elevation,omitempty"`
	Timezone  int         `json:"timezone" xml:"timezone"`
}

type Coord struct {
	Lat float64 `json:"lat" xml:"lat"`
	Lon float64 `json:"lon" xml:"lon"`
}

type ForecastMeta struct {
	Model        string `json:"model" xml:"model"`
	DataDateTime string `json:"data_date_time" xml:"data_date_time"`
	Source       string `json:"source" xml:"source"`
	Runtime      string `json:"runtime,omitempty" xml:"runtime,omitempty"`
}

type ForecastItem struct {
	DT      int64           `json:"dt" xml:"dt"`
	DTTxt   string          `json:"dt_txt" xml:"dt_txt"`
	Main    ForecastMain    `json:"main" xml:"main"`
	Weather []WeatherCond   `json:"weather" xml:"weather>condition"`
	Clouds  ForecastClouds  `json:"clouds" xml:"clouds"`
	Wind    ForecastWind    `json:"wind" xml:"wind"`
	Rain    *Precip1h       `json:"rain,omitempty" xml:"rain,omitempty"`
	Snow    *Precip1h       `json:"snow,omitempty" xml:"snow,omitempty"`
}

type ForecastMain struct {
	Temp      *float64 `json:"temp,omitempty" xml:"temp,omitempty"`
	TempMin   *float64 `json:"temp_min,omitempty" xml:"temp_min,omitempty"`
	TempMax   *float64 `json:"temp_max,omitempty" xml:"temp_max,omitempty"`
	Pressure  *float64 `json:"pressure,omitempty" xml:"pressure,omitempty"`
	SeaLevel  *float64 `json:"sea_level,omitempty" xml:"sea_level,omitempty"`
}

type ForecastClouds struct {
	All  *float64 `json:"all,omitempty" xml:"all,omitempty"`
	Low  *float64 `json:"low,omitempty" xml:"low,omitempty"`
	Mid  *float64 `json:"mid,omitempty" xml:"mid,omitempty"`
	High *float64 `json:"high,omitempty" xml:"high,omitempty"`
}

type ForecastWind struct {
	Speed *float64 `json:"speed,omitempty" xml:"speed,omitempty"`
	Deg   *float64 `json:"deg,omitempty" xml:"deg,omitempty"`
	Gust  *float64 `json:"gust,omitempty" xml:"gust,omitempty"`
}

type Precip1h struct {
	H1 float64 `json:"1h" xml:"h1"`
}

type WeatherCond struct {
	ID          int    `json:"id" xml:"id,attr"`
	Main        string `json:"main" xml:"main,attr"`
	Description string `json:"description" xml:"description,attr"`
	Icon        string `json:"icon" xml:"icon,attr"`
}

type APIError struct {
	XMLName struct{} `json:"-" xml:"error"`
	Code    int      `json:"code" xml:"code"`
	Message string   `json:"message" xml:"message"`
}
