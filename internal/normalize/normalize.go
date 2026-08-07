package normalize

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var stripper = transform.Chain(
	norm.NFD,
	runes.Remove(runes.In(unicode.Mn)),
	norm.NFC,
)

// Fold removes Slovak/Latin diacritics and lowercases for search matching.
// Example: "Bánovce nad Bebravou" -> "banovce nad bebravou"
func Fold(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	out, _, err := transform.String(stripper, s)
	if err != nil {
		out = s
	}
	// Slovak specific: ľ/ĺ already handled by NFD; map remaining digraphs if needed.
	out = strings.ReplaceAll(out, "ł", "l")
	out = strings.ReplaceAll(out, "Ł", "l")
	return strings.ToLower(out)
}
