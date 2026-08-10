# SHMU ALADIN API

**Slovenčina** | [English](README.en.md)

HTTP API v jazyku Go, ktoré získava dáta numerického modelu ALADIN od Slovenského hydrometeorologického ústavu (SHMÚ), ukladá ich do SQLite a sprístupňuje vo formáte JSON.

Odpovede s predpoveďou majú štruktúru podobnú [5-dňovej predpovedi OpenWeatherMap](https://openweathermap.org/forecast5), ale namiesto 3-hodinových intervalov používajú hodinové kroky modelu ALADIN. Predpoveď pokrýva **nasledujúce 3 dni**, čo zodpovedá horizontu modelu ALADIN od SHMÚ.

## Rýchly štart

```sh
# vyžaduje Go 1.22+
go run ./cmd/server
```

Voliteľné premenné prostredia (macOS/Linux):

```sh
export ADDR=":8080"
export WEB_URL="http://localhost:8080"  # verejná základná URL zobrazená v dokumentácii
export DATABASE_PATH="data/shmu.db"
export STATIONS_CACHE_TTL="168h"         # 7 dní
export FORECAST_SYNC_EVERY="30m"
export SYNC_FORECASTS="false"            # bez hromadného sťahovania; načítanie na požiadanie
export FORECAST_STALE_AFTER="8h"         # označenie zastaranej predpovede v odvodených odpovediach
go run ./cmd/server
```

Rovnaké premenné môžete uložiť aj do lokálneho súboru `.env`. Ak existuje, načíta sa automaticky; premenné procesu majú prednosť.

Pri spustení server synchronizuje stanice zo SHMÚ. Pri predvolenom nastavení `SYNC_FORECASTS=true` vopred stiahne aj produkty a najnovšie súbory ALADIN pre všetky stanice. Prvé spustenie preto môže trvať dlhšie. Pri hodnote `false` sa predpovede načítajú až pri prvej požiadavke.

## Dokumentácia

Verejné API s dokumentáciou je na [https://shmu.bmikuska.com](https://shmu.bmikuska.com).

Slovenská dokumentácia je dostupná na `/` a `/docs`. Anglická verzia je na `/en`; medzi jazykmi sa dá prepnúť priamo v hornej lište.

## Endpointy

| Metóda | Cesta | Popis |
|--------|------|--------|
| `GET` | `/health` | Kontrola dostupnosti |
| `GET` | `/api/v1` | Zoznam dostupných endpointov |
| `GET` | `/api/v1/stations` | Stránkovaný zoznam staníc a vyhľadávanie |
| `GET` | `/api/v1/stations/{id}` | Detail stanice |
| `GET` | `/api/v1/forecast?station={id}` alebo `?lat=&lon=` | Najnovšia predpoveď ALADIN na 3 dni v štruktúre podobnej OWM; súradnice sa mapujú na najbližšiu stanicu |
| `GET` | `/api/v1/forecast/daily?...` | Denné súhrny z hodinovej predpovede |
| `GET` | `/api/v1/now?...` | Aktuálne počasie (hodinový krok ALADIN najbližší k „teraz“) |
| `GET` | `/api/v1/weather?...` | Alias predpovede |
| `GET` | `/api/v1/weather/codes` | Katalóg enumov počasia (`code` + `description`) |
| `GET` | `/api/v1/indicators?...` | Odvodené (neoficiálne) indikátory rizika |

### Stanice

```
GET /api/v1/stations?q=bratislava&page=1&page_size=20
GET /api/v1/stations?search=banovce
```

- `q` / `search`: vyhľadávanie bez rozlišovania diakritiky (napr. `zilina` nájde `Žilina`)
- `page` (predvolene 1), `page_size` / `limit` (predvolene 50, maximum 200)
- Výsledky sa ukladajú do dlhodobej vyrovnávacej pamäte (`Cache-Control: max-age=86400`)

### Predpoveď (štruktúra podobná OWM)

```
GET /api/v1/forecast?station=32737
GET /api/v1/forecast?station=32737&hours=24
GET /api/v1/forecast?lat=48.15&lon=17.11
GET /api/v1/forecast?lat=48.15&lon=17.11&hours=24
```

Zadajte buď `station` (ID stanice), alebo `lat` a `lon` (WGS84). Pri súradniciach API vyberie najbližšiu stanicu a vráti jej predpoveď (`city` v odpovedi je stále nájdená stanica) spolu s `location_match` (vzdialenosť v km, nadmorská výška stanice). Voliteľne `max_distance_km` (max. 500) vráti `404`, ak je najbližšia stanica príliš ďaleko. Parameter `elevation` nie je podporovaný — predpoveď nie je korigovaná podľa výšky. Voliteľný `hours` obmedzí zoznam na N krokov od aktuálnej UTC hodiny (nie od začiatku behu modelu).

Každý hodinový krok obsahuje lokálny `date` (Europe/Bratislava) a `date_index` (0 = pondelok … 6 = nedeľa). Počasie je `{ "code", "description" }`; zoznam všetkých kódov je na `GET /api/v1/weather/codes`.

### Denný súhrn

```
GET /api/v1/forecast/daily?station=32737
GET /api/v1/forecast/daily?lat=48.15&lon=17.11&days=3
```

Agregácia hodinových krokov podľa kalendárneho dňa `Europe/Bratislava`: min/max teplota, úhrny zrážok (celkové / kvapalné / snehový vodný ekvivalent), počet hodín so zrážkami, max. vietor a nárazy, priemerná oblačnosť, reprezentatívne počasie, východ/západ Slnka, príznak `is_partial`.

`precipitation_total` je súčet hodinového poľa Total precipitation; `rain_total` je odhad kvapalnej zložky (total − snowfall, min. 0); `snow_total` je vodný ekvivalent snehu. Chýbajúce vstupy sa vracajú ako `null`, nie ako nula.

### Aktuálne počasie

```
GET /api/v1/now?station=32737
GET /api/v1/now?stationId=32737
GET /api/v1/now?lat=48.15&lon=17.11
```

Vráti hodinový krok ALADIN najbližší k momentu požiadavky (`current`), plus `as_of` (čas požiadavky UTC) a `offset_seconds` (rozdiel `current.dt − as_of`). Nie je to živé meranie zo stanice — ide o modelovú predpoveď pre „teraz“.

### Indikátory (neoficiálne)

```
GET /api/v1/indicators?station=32737
GET /api/v1/indicators?lat=49.12&lon=20.06&type=frost,wind,heavy_rain
```

Typy v1: `frost`, `heat`, `wind`, `heavy_rain`, `snow`, `mixed_precipitation`, `low_cloud_mountain`. Každá položka má `official: false` a `source_kind: "forecast"` — nie sú oficiálnymi výstrahami SHMÚ.

Príklad odpovede hodinovej predpovede:

```json
{
  "code": "200",
  "message": 0,
  "hours": 103,
  "list": [
    {
      "dt": 1786064400,
      "dt_txt": "2026-08-07 01:00:00",
      "date": "2026-08-07",
      "date_index": 4,
      "main": {
        "temp": 24.353,
        "temp_min": 24.276,
        "temp_max": 24.519,
        "pressure": 1015.485,
        "sea_level": 1015.485
      },
      "weather": [{ "code": "broken_clouds", "description": "broken clouds" }],
      "clouds": { "all": 70.986, "low": 0, "mid": 8.067, "high": 68.723 },
      "wind": { "speed": 8.902, "deg": 317.732, "gust": 19.415 },
      "rain": { "1h": 0.2 }
    }
  ],
  "city": {
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

Jednotky: °C, m/s, hPa, mm/h a oblačnosť v % — v súlade so štýlom OWM `metric`.

## Tok dát

1. `getjsonaladinstations` → tabuľka staníc (dlhé TTL)
2. `getstationproducts?station={id}` → vyrovnávacia pamäť produktov
3. Najnovší `file_link` s `type: "aladin"` → JSON predpovede → transformovaná odpoveď API

Výstupy modelu ALADIN sa zvyčajne objavujú 3× denne (`00`, `06`, `12` alebo `18` podľa dostupnosti). API vždy poskytuje najnovší uložený výpočet a obnovuje ho v nastavenom intervale na pozadí alebo na požiadanie.

## Štruktúra projektu

```
cmd/server/          HTTP server a synchronizácia na pozadí
internal/api/        Obsluha požiadaviek (JSON)
internal/geo/        Časové pásmo, východ/západ Slnka, Haversine, čerstvosť
internal/indicator/  Odvodené neoficiálne indikátory rizika
internal/shmu/       Klient zdrojového API SHMÚ
internal/store/      SQLite a FTS5
internal/transform/  Mapovanie ALADIN → štruktúra podobná OWM + denné súhrny
internal/syncer/     Sťahovanie a obnova dát
internal/normalize/  Normalizácia diakritiky pre vyhľadávanie
```

## Testy

```sh
go test ./...
```
