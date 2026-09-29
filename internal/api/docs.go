package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bmikuska/shmu-weather-api/web"
)

const seoTitleSK = "SHMÚ API - predpoveď počasia pre Slovensko v JSON (ALADIN)"
const seoTitleEN = "SHMU API - Slovak weather forecast JSON API (ALADIN)"
const seoDescriptionSK = "SHMÚ API zadarmo a bez API kľúča: JSON predpoveď počasia pre Slovensko z modelu ALADIN. Hodinová aj denná predpoveď na 3 dni, aktuálne počasie a 1 000+ staníc."
const seoDescriptionEN = "Free SHMU API with no API key: JSON weather forecasts for Slovakia from the SHMÚ ALADIN model. Hourly and daily 3-day forecasts, current weather and 1,000+ stations."

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	s.writeDocs(w, false)
}

func (s *Server) handleEnglishDocs(w http.ResponseWriter, r *http.Request) {
	s.writeDocs(w, true)
}

// staticAssets maps root-level icon and manifest paths to their content types.
var staticAssets = map[string]string{
	"/favicon.ico":          "image/x-icon",
	"/favicon-32x32.png":    "image/png",
	"/apple-touch-icon.png": "image/png",
	"/icon-192.png":         "image/png",
	"/icon-512.png":         "image/png",
	"/site.webmanifest":     "application/manifest+json",
}

func (s *Server) handleStaticAsset(w http.ResponseWriter, r *http.Request) {
	body, err := web.Static.ReadFile("static" + r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", staticAssets[r.URL.Path])
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.webURL, "/")
	body := fmt.Sprintf("User-agent: *\nAllow: /\nAllow: /docs\nAllow: /en\nDisallow: /api/\nDisallow: /health\n\nSitemap: %s/sitemap.xml\n", base)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(s.webURL, "/")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
        xmlns:xhtml="http://www.w3.org/1999/xhtml">
  <url>
    <loc>%[1]s/</loc>
    <xhtml:link rel="alternate" hreflang="sk" href="%[1]s/" />
    <xhtml:link rel="alternate" hreflang="en" href="%[1]s/en" />
    <xhtml:link rel="alternate" hreflang="x-default" href="%[1]s/" />
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
  <url>
    <loc>%[1]s/en</loc>
    <xhtml:link rel="alternate" hreflang="sk" href="%[1]s/" />
    <xhtml:link rel="alternate" hreflang="en" href="%[1]s/en" />
    <xhtml:link rel="alternate" hreflang="x-default" href="%[1]s/" />
    <changefreq>weekly</changefreq>
    <priority>0.9</priority>
  </url>
</urlset>
`, base)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) writeDocs(w http.ResponseWriter, english bool) {
	body := s.docsSK
	if english {
		body = s.docsEN
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", map[bool]string{false: "sk", true: "en"}[english])
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// renderDocs builds a docs page once; the result depends only on the language and webURL.
func renderDocs(webURL string, english bool) []byte {
	body := string(web.DocsHTML)
	if english {
		body = englishDocsReplacer.Replace(body)
		body = strings.NewReplacer(
			"__DOCS_PATH__", "/en",
			"__LANG_PATH__", "/",
			"__LANG_CODE__", "sk",
			"__LANG_ARIA__", "Switch to Slovak",
			"__LANG_LABEL__", "Slovenčina",
		).Replace(body)
	} else {
		body = strings.NewReplacer(
			"__DOCS_PATH__", "/",
			"__LANG_PATH__", "/en",
			"__LANG_CODE__", "en",
			"__LANG_ARIA__", "Prepnúť do angličtiny",
			"__LANG_LABEL__", "English",
		).Replace(body)
	}
	body = strings.ReplaceAll(body, "__WEB_URL__", strings.TrimRight(webURL, "/"))
	return []byte(body)
}

var englishDocsReplacer = strings.NewReplacer(
	`<html lang="sk">`, `<html lang="en">`,
	seoTitleSK, seoTitleEN,
	seoDescriptionSK, seoDescriptionEN,
	`"og:locale" content="sk_SK"`, `"og:locale" content="en_US"`,
	`"og:locale:alternate" content="en_US"`, `"og:locale:alternate" content="sk_SK"`,
	`"inLanguage": "sk"`, `"inLanguage": "en"`,
	`SHMU API dokumentácia`, `SHMU API documentation`,
	`SHMÚ API: predpoveď počasia pre Slovensko`, `SHMU API: weather forecast for Slovakia`,
	`Bezplatné JSON API nad dátami numerického modelu ALADIN od SHMÚ. Vyhľadávanie staníc, hodinová a denná predpoveď na 3 dni a aktuálne počasie bez registrácie a API kľúča.`, `A free JSON API for SHMÚ ALADIN numerical model data. Search stations and get hourly and daily 3-day forecasts and current weather with no sign-up and no API key.`,
	`Čo je SHMÚ API`, `What is the SHMU API`,
	`SHMÚ API (SHMU API) sprístupňuje predpovede počasia <a href="https://www.shmu.sk/" rel="noopener">Slovenského hydrometeorologického ústavu</a> v jednoduchom formáte JSON. SHMÚ zverejňuje výstupy modelu ALADIN ako dátové súbory. Táto služba ich pravidelne sťahuje, spracuje a ponúka cez REST endpointy s odpoveďami podobnými OpenWeatherMap.`, `The SHMU API (SHMÚ API) serves weather forecasts from the <a href="https://www.shmu.sk/" rel="noopener">Slovak Hydrometeorological Institute</a> as simple JSON. SHMÚ publishes ALADIN model output as data files. This service downloads them on a schedule, processes them and exposes them through REST endpoints with OpenWeatherMap-style responses.`,
	`Ide o neoficiálny projekt s <a href="https://github.com/bmikuska46/shmu-aladin-api" rel="noopener">otvoreným zdrojovým kódom</a>, ktorý SHMÚ neprevádzkuje. Zdrojom všetkých predpovedí sú verejné dáta SHMÚ.`, `It is an unofficial <a href="https://github.com/bmikuska46/shmu-aladin-api" rel="noopener">open-source project</a> that is not operated by SHMÚ. All forecasts come from public SHMÚ data.`,
	`Časté otázky`, `FAQ`,
	`Je SHMÚ API zadarmo?`, `Is the SHMU API free?`,
	`Áno. SHMÚ API je bezplatné a nevyžaduje registráciu ani API kľúč. Platí iba limit 10 požiadaviek za minútu pre každý endpoint a IP adresu.`, `Yes. The SHMU API is free and needs no sign-up or API key. The only limit is 10 requests per minute for each endpoint and IP address.`,
	`Je to oficiálne API SHMÚ?`, `Is this the official SHMÚ API?`,
	`Nie. Ide o neoficiálny open-source projekt, ktorý spracúva verejne dostupné dáta modelu ALADIN od SHMÚ. Neposkytuje oficiálne výstrahy SHMÚ.`, `No. It is an unofficial open-source project that processes publicly available SHMÚ ALADIN model data. It does not provide official SHMÚ warnings.`,
	`Aké dáta SHMU API poskytuje?`, `What data does the SHMU API provide?`,
	`Hodinovú a dennú predpoveď na 3 dni (teplota, vietor, oblačnosť, zrážky a tlak), aktuálne počasie, viac ako 1 000 staníc na Slovensku a odvodené indikátory ako mráz, horúčava alebo silný vietor.`, `Hourly and daily 3-day forecasts (temperature, wind, cloud cover, precipitation and pressure), current weather, more than 1,000 stations in Slovakia and derived indicators such as frost, heat or strong wind.`,
	`Ako často sa predpoveď aktualizuje?`, `How often is the forecast updated?`,
	`Model ALADIN beží štyrikrát denne (00, 06, 12 a 18 UTC). SHMÚ zverejňuje výsledky približne 4 hodiny po behu a API vracia najnovší dostupný beh.`, `The ALADIN model runs four times a day (00, 06, 12 and 18 UTC). SHMÚ publishes the results about 4 hours after each run and the API returns the latest available run.`,
	`Dá sa predpoveď získať podľa GPS súradníc?`, `Can I get a forecast by GPS coordinates?`,
	`Áno. Endpointy /forecast, /forecast/daily, /now a /indicators prijímajú parametre lat a lon a použijú najbližšiu stanicu modelu ALADIN.`, `Yes. The /forecast, /forecast/daily, /now and /indicators endpoints accept lat and lon parameters and use the nearest ALADIN model station.`,
	`Dokumentácia&nbsp; / &nbsp;<strong>API referencia</strong>`, `Documentation&nbsp; / &nbsp;<strong>API reference</strong>`,
	`aria-label="Otvoriť navigáciu"`, `aria-label="Open navigation"`,
	`aria-label="Dokumentácia"`, `aria-label="Documentation"`,
	`Začíname`, `Getting started`,
	`Prehľad`, `Overview`,
	`Rýchly štart`, `Quick start`,
	`Limity a aktualizácie`, `Limits and updates`,
	`API referencia`, `API reference`,
	`JSON API nad dátami numerického modelu ALADIN. Vyhľadávanie staníc a hodinové predpovede na nasledujúce 3 dni.`, `A JSON API for ALADIN numerical model data. Search weather stations and retrieve hourly forecasts for the next 3 days.`,
	`Služba sťahuje dáta zo SHMÚ a sprístupňuje ich cez HTTP. Základná adresa API:`, `The service downloads data from SHMU and exposes it over HTTP. API base URL:`,
	`Kopírovať`, `Copy`,
	`Stanice`, `Stations`,
	`Viac ako 1 000 lokalít. Vyhľadávanie funguje aj bez diakritiky.`, `More than 1,000 locations. Search works without diacritics too.`,
	`Predpoveď`, `Forecast`,
	`Hodinové kroky na nasledujúce 3 dni (horizont modelu ALADIN): teplota, vietor, oblačnosť, zrážky a tlak.`, `Hourly steps for the next 3 days (the ALADIN model horizon): temperature, wind, cloud cover, precipitation, and pressure.`,
	`<strong>Tip:</strong> Vyhľadávanie je bez diakritiky — <code class="inline-code">zilina</code> nájde aj „Žilina“.`, `<strong>Tip:</strong> Search is diacritic-insensitive — <code class="inline-code">zilina</code> also matches “Žilina”.`,
	`Endpointy`, `Endpoints`,
	`Všetky endpointy používajú metódu GET. Chybové odpovede majú tvar`, `All endpoints use the GET method. Error responses have the following shape:`,
	`Zoznam dostupných endpointov API (JSON).`, `List of available API endpoints (JSON).`,
	`Zoznam staníc so stránkovaním a vyhľadávaním.`, `A paginated, searchable list of stations.`,
	`Popis`, `Description`,
	`Textový filter, podporuje vyhľadávanie bez diakritiky.`, `Text filter with diacritic-insensitive search.`,
	`Číslo stránky. Predvolená hodnota je 1.`, `Page number. The default is 1.`,
	`Veľkosť stránky. Predvolená hodnota je 50, maximum 200.`, `Page size. The default is 50 and the maximum is 200.`,
	`Detail stanice podľa ID. Napríklad`, `Station details by ID. For example,`,
	`označuje Bratislavu centrum.`, `identifies central Bratislava.`,
	`Najnovšia predpoveď ALADIN na nasledujúce 3 dni. Zadajte buď <code class="inline-code">station</code> (ID stanice), alebo <code class="inline-code">lat</code> a <code class="inline-code">lon</code> (najbližšia stanica). Voliteľný parameter <code class="inline-code">hours</code> obmedzí odpoveď na N hodinových krokov od aktuálnej hodiny (nie od začiatku behu modelu).`, `The latest ALADIN forecast for the next 3 days. Provide either <code class="inline-code">station</code> (station ID), or <code class="inline-code">lat</code> and <code class="inline-code">lon</code> (nearest station). The optional <code class="inline-code">hours</code> parameter limits the response to N hourly steps from the current hour (not from the model run start).`,
	`Alias: <code class="inline-code">/api/v1/weather</code> s rovnakými parametrami.`, `Alias: <code class="inline-code">/api/v1/weather</code> with the same parameters.`,
	`ID stanice. Alternatíva k <code class="inline-code">lat</code> + <code class="inline-code">lon</code>.`, `Station ID. Alternative to <code class="inline-code">lat</code> + <code class="inline-code">lon</code>.`,
	`Zemepisná šírka (−90 až 90). Použite spolu s <code class="inline-code">lon</code>.`, `Latitude (−90 to 90). Use together with <code class="inline-code">lon</code>.`,
	`Zemepisná dĺžka (−180 až 180). Použite spolu s <code class="inline-code">lat</code>.`, `Longitude (−180 to 180). Use together with <code class="inline-code">lat</code>.`,
	`Počet hodinových krokov od aktuálnej hodiny.`, `Number of hourly steps from the current hour.`,
	`Pri súradniciach odpoveď obsahuje <code class="inline-code">location_match</code> (vzdialenosť a nadmorská výška stanice). Voliteľne <code class="inline-code">max_distance_km</code> (max. 500) odmietne príliš vzdialenú zhodu.`, `Coordinate responses include <code class="inline-code">location_match</code> (distance and station elevation). Optional <code class="inline-code">max_distance_km</code> (max 500) rejects matches that are too far.`,
	`Maximálna vzdialenosť k najbližšej stanici (iba s lat/lon).`, `Maximum distance to the nearest station (lat/lon only).`,
	`Denné súhrny z hodinovej predpovede ALADIN (kalendárne dni <code class="inline-code">Europe/Bratislava</code>): min/max teplota, zrážky, vietor, dominantné počasie, východ/západ Slnka.`, `Daily summaries from the hourly ALADIN forecast (Europe/Bratislava calendar days): min/max temperature, precipitation, wind, dominant weather, sunrise/sunset.`,
	`Rovnaké pravidlá ako pri hodinovej predpovedi.`, `Same rules as the hourly forecast.`,
	`Počet dní (1 až dostupný horizont). Bez parametra vráti všetky dostupné dni.`, `Number of days (1 through the available horizon). Omit to return every available day.`,
	`Maximálna vzdialenosť pri súradniciach.`, `Maximum distance for coordinate requests.`,
	`Aktuálne počasie ako hodinový krok ALADIN najbližší k momentu požiadavky. Nie je to živé meranie zo stanice — ide o modelovú predpoveď pre „teraz“. Zadajte buď <code class="inline-code">station</code> / <code class="inline-code">stationId</code>, alebo <code class="inline-code">lat</code> a <code class="inline-code">lon</code>.`, `Current weather as the hourly ALADIN step closest to the request time. This is not a live station observation — it is model guidance for “now”. Provide either <code class="inline-code">station</code> / <code class="inline-code">stationId</code>, or <code class="inline-code">lat</code> and <code class="inline-code">lon</code>.`,
	`Katalóg enumov počasia. Každá položka má stabilný <code class="inline-code">code</code> a ľudsky čitateľný <code class="inline-code">description</code>. Rovnaké hodnoty vracia aj pole <code class="inline-code">weather</code> v hodinovej a dennej predpovedi. Index dňa <code class="inline-code">date_index</code> je 0 = pondelok … 6 = nedeľa (Europe/Bratislava).`, `Catalog of weather enums. Each entry has a stable <code class="inline-code">code</code> and a human-readable <code class="inline-code">description</code>. The same values appear in the <code class="inline-code">weather</code> field of hourly and daily forecasts. Day index <code class="inline-code">date_index</code> is 0 = Monday … 6 = Sunday (Europe/Bratislava).`,
	`Indikátory odvodené z hodinovej predpovede (mráz, horúčava, vietor, intenzívny dážď, sneh, zmiešané zrážky, nízka oblačnosť v horách). <strong>Nie sú oficiálnymi výstrahami SHMÚ</strong> — každá položka má <code class="inline-code">official: false</code> a <code class="inline-code">source_kind: "forecast"</code>.`, `Indicators derived from the hourly forecast (frost, heat, wind, heavy rain, snow, mixed precipitation, low-cloud mountain caution). <strong>These are not official SHMU warnings</strong> — every item has <code class="inline-code">official: false</code> and <code class="inline-code">source_kind: "forecast"</code>.`,
	`Rovnaké pravidlá ako pri predpovedi.`, `Same rules as the forecast endpoint.`,
	`Filtrovanie typov oddelených čiarkou, napr. <code class="inline-code">frost,wind,heavy_rain</code>.`, `Comma-separated type filter, e.g. <code class="inline-code">frost,wind,heavy_rain</code>.`,
	`Ukážka odpovede`, `Example response`,
	`Obmedzenie požiadaviek`, `Rate limiting`,
	`Predvolený limit je <strong>10 požiadaviek za minútu</strong> pre každý endpoint a IP adresu. Pri prekročení API vráti stav`, `The default limit is <strong>10 requests per minute</strong> for each endpoint and IP address. When exceeded, the API returns status`,
	`a hlavičku <code class="inline-code">Retry-After</code>. Dokumentácia nie je limitovaná.`, `and a <code class="inline-code">Retry-After</code> header. Documentation routes are not rate-limited.`,
	`Skopírované`, `Copied`,
	`Označte text`, `Select the text`,
)
