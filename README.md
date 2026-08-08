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
| `GET` | `/api/v1/weather?...` | Alias predpovede |

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
GET /api/v1/forecast?station=32737&cnt=24
GET /api/v1/forecast?lat=48.15&lon=17.11
GET /api/v1/forecast?lat=48.15&lon=17.11&cnt=24
```

Zadajte buď `station` (ID stanice), alebo `lat` a `lon` (WGS84). Pri súradniciach API vyberie najbližšiu stanicu a vráti jej predpoveď (`city` v odpovedi je stále nájdená stanica).
Príklad odpovede:

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
internal/shmu/       Klient zdrojového API SHMÚ
internal/store/      SQLite a FTS5
internal/transform/  Mapovanie ALADIN → štruktúra podobná OWM
internal/syncer/     Sťahovanie a obnova dát
internal/normalize/  Normalizácia diakritiky pre vyhľadávanie
```

## Testy

```sh
go test ./...
```
