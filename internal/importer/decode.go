package importer

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

// Fold lower-cases s, strips diacritics and collapses spaces, for matching
// headers and location names.
func Fold(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(strings.TrimSpace(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsSpace(r):
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

var cedilla = strings.NewReplacer("ş", "ș", "ţ", "ț", "Ş", "Ș", "Ţ", "Ț")

// fixCedilla replaces the cedilla letters (common in legacy encodings) with
// the correct Romanian comma-below letters.
func fixCedilla(s string) string { return cedilla.Replace(s) }

const romanian = "ăâîșțşţĂÂÎȘȚŞŢ"

func romanianScore(s string) int {
	n := 0
	for _, r := range s {
		if strings.ContainsRune(romanian, r) {
			n++
		}
	}
	return n
}

// decodeText returns the text as UTF-8 and the detected encoding name.
func decodeText(data []byte) (string, string) {
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		return string(data[3:]), "utf-8"
	}
	if utf8.Valid(data) {
		return string(data), "utf-8"
	}
	a, _ := charmap.Windows1250.NewDecoder().Bytes(data)
	b, _ := charmap.Windows1252.NewDecoder().Bytes(data)
	if romanianScore(string(a)) > romanianScore(string(b)) {
		return string(a), "windows-1250"
	}
	return string(b), "windows-1252"
}

// detectSeparator counts candidate separators in the first line, outside quotes.
func detectSeparator(text string) rune {
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			line = l // the first non-blank line is the header
			break
		}
	}
	counts := map[rune]int{}
	quoted := false
	for _, r := range line {
		switch r {
		case '"':
			quoted = !quoted
		case ';', ',', '\t':
			if !quoted {
				counts[r]++
			}
		}
	}
	best := ';'
	for _, r := range []rune{';', ',', '\t'} {
		if counts[r] > counts[best] {
			best = r
		}
	}
	return best
}

// synonyms maps folded header names to canonical columns.
var synonyms = map[string]string{}

func init() {
	for canon, names := range map[string][]string{
		"location":   {"location", "locatie", "locatia", "nume locatie"},
		"address":    {"address", "adresa"},
		"lat":        {"lat", "latitudine", "latitude"},
		"lon":        {"lon", "lng", "longitudine", "longitude"},
		"device":     {"device", "echipament", "nume echipament", "denumire echipament"},
		"host":       {"host", "ip", "adresa ip", "ip/host", "hostname"},
		"kind":       {"kind", "tip", "type"},
		"is_primary": {"is_primary", "primar", "principal", "primary"},
		"notes":      {"notes", "observatii", "note"},
	} {
		for _, n := range names {
			synonyms[n] = canon
		}
	}
}
