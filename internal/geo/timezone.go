package geo

import "time"

// Bratislava is the IANA timezone used for Slovak local calendar days and offsets.
var Bratislava *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		// Fixed CET fallback if tzdata is unavailable.
		Bratislava = time.FixedZone("CET", 3600)
		return
	}
	Bratislava = loc
}

// LocalDate returns the Europe/Bratislava calendar date for a UTC unix timestamp.
func LocalDate(ts int64) time.Time {
	t := time.Unix(ts, 0).In(Bratislava)
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, Bratislava)
}

// LocalDateString formats the local calendar date as YYYY-MM-DD.
func LocalDateString(ts int64) string {
	return LocalDate(ts).Format("2006-01-02")
}

// DateIndex returns weekday index for Europe/Bratislava: 0=Monday … 6=Sunday.
func DateIndex(ts int64) int {
	return WeekdayIndex(LocalDate(ts).Weekday())
}

// WeekdayIndex maps time.Weekday to 0=Monday … 6=Sunday.
func WeekdayIndex(w time.Weekday) int {
	// time.Weekday: Sunday=0 … Saturday=6
	return (int(w) + 6) % 7
}

// TimezoneOffsetSeconds returns the Europe/Bratislava UTC offset at ts.
func TimezoneOffsetSeconds(ts int64) int {
	_, offset := time.Unix(ts, 0).In(Bratislava).Zone()
	return offset
}

// DayBounds returns the UTC unix range [start, end) covering the local calendar day
// that contains ts. End is exclusive (start of the next local day).
func DayBounds(ts int64) (start, end int64) {
	day := LocalDate(ts)
	return day.Unix(), day.AddDate(0, 0, 1).Unix()
}
