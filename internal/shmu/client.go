package shmu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidFileLink is returned when an opendata path fails strict validation.
var ErrInvalidFileLink = errors.New("invalid aladin file link")

// aladinFileLinkRe matches the conventional SHMU datanwp ALADIN path only.
// Example: aladin/2026-08-07/32737_2026-08-07_00.json
var aladinFileLinkRe = regexp.MustCompile(`^aladin/[0-9]{4}-[0-9]{2}-[0-9]{2}/[0-9]+_[0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}\.json$`)

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
	if err := ValidateAladinFileLink(fileLink); err != nil {
		return nil, err
	}
	fetchURL, err := joinDataURL(c.dataBaseURL, fileLink)
	if err != nil {
		return nil, err
	}
	var out AladinForecast
	if err := c.getJSON(ctx, fetchURL, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidateAladinFileLink rejects path traversal, absolute URLs, and any path
// that does not match the expected SHMU ALADIN opendata layout. Call this
// before fetching upstream so untrusted file_link values from products JSON
// cannot redirect requests off the configured data host.
func ValidateAladinFileLink(fileLink string) error {
	if fileLink == "" {
		return fmt.Errorf("%w: empty", ErrInvalidFileLink)
	}
	if fileLink != strings.TrimSpace(fileLink) {
		return fmt.Errorf("%w: surrounding whitespace", ErrInvalidFileLink)
	}
	if strings.Contains(fileLink, "..") || strings.ContainsAny(fileLink, "\\?#\x00\r\n\t ") {
		return fmt.Errorf("%w: unsafe characters", ErrInvalidFileLink)
	}
	if strings.Contains(fileLink, "://") || strings.HasPrefix(fileLink, "//") || strings.HasPrefix(fileLink, "/") {
		return fmt.Errorf("%w: absolute or scheme URL", ErrInvalidFileLink)
	}
	cleaned := path.Clean("/" + fileLink)
	if cleaned != "/"+fileLink {
		return fmt.Errorf("%w: path cleaned away from original", ErrInvalidFileLink)
	}
	if !aladinFileLinkRe.MatchString(fileLink) {
		return fmt.Errorf("%w: pattern mismatch", ErrInvalidFileLink)
	}
	return nil
}

func joinDataURL(base, fileLink string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "", fmt.Errorf("%w: empty data base URL", ErrInvalidFileLink)
	}
	u, err := url.Parse(base + "/")
	if err != nil {
		return "", fmt.Errorf("%w: parse data base URL: %v", ErrInvalidFileLink, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: data base URL must be http(s)", ErrInvalidFileLink)
	}
	ref, err := url.Parse(fileLink)
	if err != nil {
		return "", fmt.Errorf("%w: parse file link: %v", ErrInvalidFileLink, err)
	}
	if ref.IsAbs() || ref.Host != "" || strings.HasPrefix(ref.Path, "/") {
		return "", fmt.Errorf("%w: file link must be a relative path", ErrInvalidFileLink)
	}
	joined := u.ResolveReference(ref)
	// Ensure the resolved URL stays under the configured data base path.
	basePath := strings.TrimSuffix(u.EscapedPath(), "/")
	joinedPath := joined.EscapedPath()
	if joined.Scheme != u.Scheme || joined.Host != u.Host {
		return "", fmt.Errorf("%w: resolved off data host", ErrInvalidFileLink)
	}
	if basePath != "" && joinedPath != basePath && !strings.HasPrefix(joinedPath, basePath+"/") {
		return "", fmt.Errorf("%w: resolved outside data base path", ErrInvalidFileLink)
	}
	return joined.String(), nil
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
