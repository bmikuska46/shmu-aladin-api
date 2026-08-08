package model

import "time"

type Station struct {
	XMLName      struct{} `json:"-" xml:"station"`
	ID           int64    `json:"id" xml:"id"`
	Name         string   `json:"name" xml:"name"`
	Lat          float64  `json:"lat" xml:"lat"`
	Lon          float64  `json:"lon" xml:"lon"`
	DistrictCode int      `json:"-" xml:"-"` // kept for SHMU sync / DB only
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
	Hours   int            `json:"hours" xml:"hours"`
	List    []ForecastItem `json:"list" xml:"list>item"`
	City    ForecastCity   `json:"city" xml:"city"`
	Meta    ForecastMeta   `json:"meta" xml:"meta"`
}

type ForecastCity struct {
	Name      string   `json:"name" xml:"name"`
	Coord     Coord    `json:"coord" xml:"coord"`
	Country   string   `json:"country" xml:"country"`
	Elevation *float64 `json:"elevation,omitempty" xml:"elevation,omitempty"`
	Timezone  int      `json:"timezone" xml:"timezone"`
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

// LocationMatch describes how coordinates were resolved to an ALADIN station.
type LocationMatch struct {
	Method             string   `json:"method"`
	RequestedCoord     Coord    `json:"requested_coord"`
	MatchedCoord       Coord    `json:"matched_coord"`
	DistanceKm         float64  `json:"distance_km"`
	MatchedStationID   int64    `json:"matched_station_id"`
	MatchedStationName string   `json:"matched_station_name"`
	MatchedElevation   *float64 `json:"matched_elevation,omitempty"`
}

// SourceMeta is shared freshness metadata for derived forecast endpoints.
type SourceMeta struct {
	Source     string `json:"source"`
	Kind       string `json:"kind"`
	Runtime    string `json:"runtime,omitempty"`
	FetchedAt  string `json:"fetched_at,omitempty"`
	AgeSeconds int64  `json:"age_seconds"`
	IsStale    bool   `json:"is_stale"`
}

// DailyForecastResponse is a calendar-day aggregation of hourly ALADIN data.
type DailyForecastResponse struct {
	Code          string             `json:"code"`
	Message       float64            `json:"message"`
	Cnt           int                `json:"cnt"`
	List          []DailyForecastDay `json:"list"`
	City          ForecastCity       `json:"city"`
	Meta          ForecastMeta       `json:"meta"`
	Source        SourceMeta         `json:"source_meta"`
	LocationMatch *LocationMatch     `json:"location_match,omitempty"`
}

type DailyForecastDay struct {
	Date               string      `json:"date"`
	DateIndex          int         `json:"date_index"` // 0=Monday … 6=Sunday (Europe/Bratislava)
	TempMin            *float64    `json:"temp_min"`
	TempMax            *float64    `json:"temp_max"`
	PrecipitationTotal *float64    `json:"precipitation_total"`
	RainTotal          *float64    `json:"rain_total"`
	SnowTotal          *float64    `json:"snow_total"`
	PrecipitationHours *int        `json:"precipitation_hours"`
	WindSpeedMax       *float64    `json:"wind_speed_max"`
	WindGustMax        *float64    `json:"wind_gust_max"`
	CloudCoverMean     *float64    `json:"cloud_cover_mean"`
	Weather            WeatherCond `json:"weather"`
	Sunrise            string      `json:"sunrise,omitempty"`
	Sunset             string      `json:"sunset,omitempty"`
	IsPartial          bool        `json:"is_partial"`
}

// IndicatorsResponse holds API-derived, non-official weather risk indicators.
type IndicatorsResponse struct {
	Code          string         `json:"code"`
	Message       float64        `json:"message"`
	Cnt           int            `json:"cnt"`
	List          []Indicator    `json:"list"`
	City          ForecastCity   `json:"city"`
	Meta          ForecastMeta   `json:"meta"`
	Source        SourceMeta     `json:"source_meta"`
	LocationMatch *LocationMatch `json:"location_match,omitempty"`
}

type Indicator struct {
	Type              string         `json:"type"`
	Level             string         `json:"level"`
	ValidFrom         string         `json:"valid_from,omitempty"`
	ValidTo           string         `json:"valid_to,omitempty"`
	Summary           string         `json:"summary"`
	Inputs            map[string]any `json:"inputs,omitempty"`
	SourceKind        string         `json:"source_kind"`
	SourceRuntime     string         `json:"source_runtime,omitempty"`
	RuleVersion       string         `json:"rule_version"`
	Official          bool           `json:"official"`
	Available         bool           `json:"available"`
	UnavailableReason string         `json:"unavailable_reason,omitempty"`
}

type ForecastItem struct {
	DT        int64          `json:"dt" xml:"dt"`
	DTTxt     string         `json:"dt_txt" xml:"dt_txt"`
	Date      string         `json:"date" xml:"date"`             // local YYYY-MM-DD (Europe/Bratislava)
	DateIndex int            `json:"date_index" xml:"date_index"` // 0=Monday … 6=Sunday
	Main      ForecastMain   `json:"main" xml:"main"`
	Weather   []WeatherCond  `json:"weather" xml:"weather>condition"`
	Clouds    ForecastClouds `json:"clouds" xml:"clouds"`
	Wind      ForecastWind   `json:"wind" xml:"wind"`
	Rain      *Precip1h      `json:"rain,omitempty" xml:"rain,omitempty"`
	Snow      *Precip1h      `json:"snow,omitempty" xml:"snow,omitempty"`
}

type ForecastMain struct {
	Temp     *float64 `json:"temp,omitempty" xml:"temp,omitempty"`
	TempMin  *float64 `json:"temp_min,omitempty" xml:"temp_min,omitempty"`
	TempMax  *float64 `json:"temp_max,omitempty" xml:"temp_max,omitempty"`
	Pressure *float64 `json:"pressure,omitempty" xml:"pressure,omitempty"`
	SeaLevel *float64 `json:"sea_level,omitempty" xml:"sea_level,omitempty"`
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

type APIError struct {
	XMLName struct{} `json:"-" xml:"error"`
	Code    int      `json:"code" xml:"code"`
	Message string   `json:"message" xml:"message"`
}
