package geo

import (
	"math"
	"time"
)

// SunriseSunset returns sunrise and sunset in UTC for the given local calendar day
// and WGS84 coordinates. Uses a NOAA-style solar calculation.
// ok is false for polar day/night (no rise or set that day).
func SunriseSunset(day time.Time, lat, lon float64) (sunrise, sunset time.Time, ok bool) {
	y, m, d := day.In(Bratislava).Date()
	noonLocal := time.Date(y, m, d, 12, 0, 0, 0, Bratislava)
	jd := julianDay(noonLocal.UTC())

	n := math.Ceil(jd - 2451545.0 - 0.0009 - lon/360)
	jStar := 2451545.0 + 0.0009 + lon/360 + n

	M := math.Mod(357.5291+0.98560028*(jStar-2451545.0), 360)
	C := 1.9148*sinDeg(M) + 0.0200*sinDeg(2*M) + 0.0003*sinDeg(3*M)
	λ := math.Mod(M+C+180+102.9372, 360)

	jTransit := jStar + 0.0053*sinDeg(M) - 0.0069*sinDeg(2*λ)
	δ := asinDeg(sinDeg(λ) * sinDeg(23.44))

	cosω := (sinDeg(-0.83) - sinDeg(lat)*sinDeg(δ)) / (cosDeg(lat) * cosDeg(δ))
	if cosω < -1 || cosω > 1 {
		return time.Time{}, time.Time{}, false
	}
	ω := acosDeg(cosω)

	jSet := jTransit + ω/360
	jRise := jTransit - ω/360

	sunrise = julianToTime(jRise)
	sunset = julianToTime(jSet)
	return sunrise, sunset, true
}

// IsDaytime reports whether ts is between sunrise and sunset at lat/lon.
// When solar times cannot be computed, daytime is assumed.
func IsDaytime(ts int64, lat, lon float64) bool {
	day := LocalDate(ts)
	sunrise, sunset, ok := SunriseSunset(day, lat, lon)
	if !ok {
		return true
	}
	t := time.Unix(ts, 0).UTC()
	return !t.Before(sunrise) && t.Before(sunset)
}

func julianDay(t time.Time) float64 {
	unix := float64(t.Unix())
	return unix/86400.0 + 2440587.5
}

func julianToTime(jd float64) time.Time {
	unix := (jd - 2440587.5) * 86400.0
	return time.Unix(int64(math.Round(unix)), 0).UTC()
}

func sinDeg(d float64) float64  { return math.Sin(d * math.Pi / 180) }
func cosDeg(d float64) float64  { return math.Cos(d * math.Pi / 180) }
func asinDeg(x float64) float64 { return math.Asin(x) * 180 / math.Pi }
func acosDeg(x float64) float64 { return math.Acos(x) * 180 / math.Pi }
