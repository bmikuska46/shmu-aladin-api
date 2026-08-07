# SHMU ALADIN API

[Slovenčina](README.md) | **English**

Go HTTP API that ingests Slovak Hydrometeorological Institute (SHMU) ALADIN NWP data, stores it in SQLite, and exposes it as JSON.

Forecast responses follow an [OpenWeatherMap 5-day forecast](https://openweathermap.org/forecast5)-style structure (hourly ALADIN steps instead of 3-hour steps). Coverage is limited to the **next 3 days** — that is the ALADIN model horizon from SHMU.

## Quick start

```powershell
# requires Go 1.22+
go run ./cmd/server
```

Optional env (PowerShell):

```powershell
$env:ADDR=":8080"
$env:WEB_URL="http://localhost:8080"  # public base URL shown in /docs
$env:DATABASE_PATH="data/shmu.db"
$env:STATIONS_CACHE_TTL="168h"      # 7 days
$env:FORECAST_SYNC_EVERY="30m"
$env:SYNC_FORECASTS="false"         # skip bulk prefetch; fetch on demand
go run ./cmd/server
```

Or put the same keys in a local `.env` file (loaded automatically if present; process env wins).

On startup the server syncs stations from SHMU. With `SYNC_FORECASTS=true` (default) it also prefetches products + latest ALADIN files for all stations (slow the first time; be polite to SHMU). With `false`, forecasts are fetched lazily on first request.

## Documentation

The public API with docs is at [https://shmu.bmikuska.com](https://shmu.bmikuska.com).

Slovak documentation is available at `/` and `/docs`. The English version is at `/en`, with a language switcher in the top bar.

## Endpoints

| Method | Path | Notes |
|--------|------|--------|
| `GET` | `/health` | Liveness |
| `GET` | `/api/v1/stations` | Paginated station list + search |
| `GET` | `/api/v1/stations/{id}` | Single station |
| `GET` | `/api/v1/forecast?station={id}` | Latest ALADIN forecast, next 3 days (OWM-like) |
| `GET` | `/api/v1/weather?station={id}` | Alias of forecast |

### Stations

```
GET /api/v1/stations?q=bratislava&page=1&page_size=20
GET /api/v1/stations?search=banovce
```

- `q` / `search`: diacritic-insensitive (e.g. `zilina` matches `Žilina`)
- `page` (default 1), `page_size` / `limit` (default 50, max 200)
- Cached for a long time (`Cache-Control: max-age=86400`)

### Forecast (OWM-like)

```
GET /api/v1/forecast?station=32737
GET /api/v1/forecast?station=32737&cnt=24
```

Example shape:

```json
{
  "code": "200",
  "message": 0,
  "cnt": 103,
  "list": [
    {
      "dt": 1786064400,
      "dt_txt": "2026-08-07 01:00:00",
      "main": {
        "temp": 24.353,
        "temp_min": 24.276,
        "temp_max": 24.519,
        "pressure": 1015.485,
        "sea_level": 1015.485
      },
      "weather": [{ "id": 803, "main": "Clouds", "description": "broken clouds", "icon": "04d" }],
      "clouds": { "all": 70.986, "low": 0, "mid": 8.067, "high": 68.723 },
      "wind": { "speed": 8.902, "deg": 317.732, "gust": 19.415 },
      "rain": { "1h": 0.2 }
    }
  ],
  "city": {
    "id": 32737,
    "name": "Bratislava (centrum)",
    "coord": { "lat": 48.156, "lon": 17.105 },
    "country": "SK",
    "elevation": 132,
    "timezone": 7200
  },
  "meta": {
    "model": "ALADIN SHMU/SK 4.5km 87L",
    "data_date_time": "2026-08-07T00:00Z",
    "source": "SHMU ALADIN",
    "runtime": "2026-08-07T00:00:00Z",
    "file_link": "aladin/2026-08-07/32737_2026-08-07_00.json"
  }
}
```

Units: °C, m/s, hPa, mm/h, cloud cover % — aligned with OWM `metric` style.

## Data flow

1. `getjsonaladinstations` → stations table (long TTL)
2. `getstationproducts?station={id}` → products cache
3. Latest `type: "aladin"` `file_link` → forecast JSON → transformed API response

ALADIN runs typically appear 3×/day (`00`, `06`, `12` / `18` depending on availability); the API always serves the newest stored runtime and refreshes on the background interval (or on demand).

## Project layout

```
cmd/server/          HTTP server + background sync
internal/api/        Handlers (JSON)
internal/shmu/       Upstream SHMU client
internal/store/      SQLite + FTS5
internal/transform/  ALADIN → OWM-like mapping
internal/syncer/     Ingest / refresh worker
internal/normalize/  Diacritic folding for search
```

## Tests

```powershell
go test ./...
```
