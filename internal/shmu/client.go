package shmu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Client struct {
	baseURL     string
	dataBaseURL string
	http        *http.Client
}

func NewClient(baseURL, dataBaseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL:     baseURL,
		dataBaseURL: dataBaseURL,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

type RawStation struct {
	StationID    int64  `json:"station_id"`
	StationName  string `json:"station_name"`
	Lat          string `json:"lat"`
	Lon          string `json:"lon"`
	DistrictCode int    `json:"district_code"`
}

type RawProduct struct {
	Type      string `json:"type"`
	DTRuntime string `json:"dt_runtime"`
	Runtime   int64  `json:"runtime"`
	FileLink  string `json:"file_link"`
}

type RawProductsResponse struct {
	Station RawStation   `json:"station"`
	Data    []RawProduct `json:"data"`
}

// Point is [unix_ts, value|null].
type Point struct {
	TS    int64
	Value *float64
}

// Series is a named time series from SHMU ALADIN JSON.
type Series struct {
	Comment string  `json:"comment"`
	Unit    string  `json:"unit"`
	Data    []Point `json:"data"`
}

func (p *Point) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) < 1 {
		return fmt.Errorf("empty point")
	}
	if err := json.Unmarshal(raw[0], &p.TS); err != nil {
		return err
	}
	if len(raw) < 2 || string(raw[1]) == "null" {
		p.Value = nil
		return nil
	}
	var v float64
	if err := json.Unmarshal(raw[1], &v); err != nil {
		return err
	}
	p.Value = &v
	return nil
}

func (p Point) MarshalJSON() ([]byte, error) {
	if p.Value == nil {
		return json.Marshal([2]any{p.TS, nil})
	}
	return json.Marshal([2]any{p.TS, *p.Value})
}

type AladinForecast struct {
	LocationName string `json:"location_name"`
	Lat          string `json:"lat"`
	Lon          string `json:"lon"`
	Elevation    string `json:"elevation"`
	SIID         string `json:"si_id"`
	ModelName    string `json:"model_name"`
	DataDateTime string `json:"data_date_time"`

	Snowfall                        Series `json:"Snowfall"`
	HighCloudCover                  Series `json:"High_cloud_cover"`
	MinimumTemperatureInTheLastHour Series `json:"Minimum_temperature_in_the_last_hour"`
	WindSpeedAt10m                  Series `json:"Wind_speed_at_10m"`
	WindDirectionAt10m              Series `json:"Wind_direction_at_10m"`
	MaximumTemperatureInTheLastHour Series `json:"Maximum_temperature_in_the_last_hour"`
	TotalPrecipitation              Series `json:"Total_precipitation"`
	LowCloudCover                   Series `json:"Low_cloud_cover"`
	AirTemperatureAt2m              Series `json:"Air_temperature_at_2m"`
	MediumCloudCover                Series `json:"Medium_cloud_cover"`
	Orography                       Series `json:"Orography"`
	TotalCloudCover                 Series `json:"Total_cloud_cover"`
	WindGustAt10m                   Series `json:"Wind_gust_at_10m"`
	MeanSeaLevelPressure            Series `json:"Mean_sea_level_pressure"`
}

// AladinFileLink builds the conventional SHMU datanwp path for a station runtime.
// Example: aladin/2026-08-07/32737_2026-08-07_00.json
func AladinFileLink(stationID, runtimeTS int64) string {
	t := time.Unix(runtimeTS, 0).UTC()
	date := t.Format("2006-01-02")
	hour := t.Format("15")
	return fmt.Sprintf("aladin/%s/%d_%s_%s.json", date, stationID, date, hour)
}

func (c *Client) GetStations(ctx context.Context) ([]RawStation, error) {
	url := c.baseURL + "/getjsonaladinstations"
	var out []RawStation
	if err := c.getJSON(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetStationProducts(ctx context.Context, stationID int64) (*RawProductsResponse, error) {
	url := fmt.Sprintf("%s/getstationproducts?station=%d", c.baseURL, stationID)
	var out RawProductsResponse
	if err := c.getJSON(ctx, url, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetAladinFile(ctx context.Context, fileLink string) (*AladinForecast, error) {
	url := c.dataBaseURL + "/" + fileLink
	var out AladinForecast
	if err := c.getJSON(ctx, url, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) getJSON(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "shmu-weather-api/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 32<<20)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(limited, 512))
		return fmt.Errorf("shmu %s: status %d: %s", url, resp.StatusCode, truncate(string(body), 200))
	}
	dec := json.NewDecoder(limited)
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func ParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
