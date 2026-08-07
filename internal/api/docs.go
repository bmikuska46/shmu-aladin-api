package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bmikuska/shmu-weather-api/web"
)

const seoDescriptionSK = "Dokumentácia SHMU ALADIN API — predpoveď počasia na nasledujúce 3 dni z modelu ALADIN. JSON API so stanicami a hodinovými predpoveďami pre Slovensko."
const seoDescriptionEN = "SHMU ALADIN API documentation — a 3-day weather forecast powered by the ALADIN model. JSON API with stations and hourly forecasts for Slovakia."

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	s.writeDocs(w, false)
}

func (s *Server) handleEnglishDocs(w http.ResponseWriter, r *http.Request) {
	s.writeDocs(w, true)
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
  <url>
    <loc>%[1]s/docs</loc>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
</urlset>
`, base)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) writeDocs(w http.ResponseWriter, english bool) {
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
	body = strings.ReplaceAll(body, "__WEB_URL__", strings.TrimRight(s.webURL, "/"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", map[bool]string{false: "sk", true: "en"}[english])
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

var englishDocsReplacer = strings.NewReplacer(
	`<html lang="sk">`, `<html lang="en">`,
	`SHMU ALADIN API — Dokumentácia`, `SHMU ALADIN API — Documentation`,
	seoDescriptionSK, seoDescriptionEN,
	`"og:locale" content="sk_SK"`, `"og:locale" content="en_US"`,
	`"og:locale:alternate" content="en_US"`, `"og:locale:alternate" content="sk_SK"`,
	`"inLanguage": "sk"`, `"inLanguage": "en"`,
	`SHMU ALADIN API dokumentácia`, `SHMU ALADIN API documentation`,
	`Dokumentácia&nbsp; / &nbsp;<strong>API referencia</strong>`, `Documentation&nbsp; / &nbsp;<strong>API reference</strong>`,
	`aria-label="Otvoriť navigáciu"`, `aria-label="Open navigation"`,
	`aria-label="Dokumentácia"`, `aria-label="Documentation"`,
	`Začíname`, `Getting started`,
	`Prehľad`, `Overview`,
	`Rýchly štart`, `Quick start`,
	`Limity a aktualizácie`, `Limits and updates`,
	`API referencia`, `API reference`,
	`Príklady použitia`, `Usage examples`,
	`Príklady`, `Examples`,
	`curl a PowerShell`, `curl and PowerShell`,
	`Formát odpovede`, `Response format`,
	`JSON API nad dátami numerického modelu ALADIN. Vyhľadávanie staníc a hodinové predpovede na nasledujúce 3 dni.`, `A JSON API for ALADIN numerical model data. Search weather stations and retrieve hourly forecasts for the next 3 days.`,
	`Služba sťahuje dáta zo SHMÚ a sprístupňuje ich cez HTTP. Základná adresa API:`, `The service downloads data from SHMU and exposes it over HTTP. API base URL:`,
	`Kopírovať`, `Copy`,
	`Stanice`, `Stations`,
	`Viac ako 1 000 lokalít. Vyhľadávanie funguje aj bez diakritiky.`, `More than 1,000 locations. Search works without diacritics too.`,
	`Predpoveď pre Bratislavu`, `Forecast for Bratislava`,
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
	`Najnovšia predpoveď ALADIN na nasledujúce 3 dni. Voliteľný parameter`, `The latest ALADIN forecast for the next 3 days. The optional`,
	`určuje počet hodinových krokov.`, `parameter controls the number of hourly steps.`,
	`Všetky príklady používajú vyššie uvedenú základnú adresu.`, `All examples use the base URL above.`,
	`Vyhľadanie staníc`, `Search stations`,
	`curl · prvých 6 hodín`, `curl · first 6 hours`,
	`Skrátená ukážka odpovede predpovede:`, `An abbreviated forecast response:`,
	`Obmedzenie požiadaviek`, `Rate limiting`,
	`Predvolený limit je <strong>10 požiadaviek za minútu</strong> pre každý endpoint a IP adresu. Pri prekročení API vráti stav`, `The default limit is <strong>10 requests per minute</strong> for each endpoint and IP address. When exceeded, the API returns status`,
	`a hlavičku <code class="inline-code">Retry-After</code>. Dokumentácia nie je limitovaná.`, `and a <code class="inline-code">Retry-After</code> header. Documentation routes are not rate-limited.`,
	`Skopírované`, `Copied`,
	`Označte text`, `Select the text`,
)
