// Package importer reads location lists from CSV/XLSX files and validates
// them row by row, without touching the database.
package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"netwatch/internal/apperr"
	"netwatch/internal/domain"
	"netwatch/internal/settings"
)

const (
	// MaxRows is the largest number of data rows in one file.
	MaxRows = 1000
	// MaxLocations is the largest number of locations in one file.
	MaxLocations = 200
)

// Columns are the canonical column names, in display order.
var Columns = []string{"location", "address", "lat", "lon", "device", "host", "kind", "is_primary", "notes"}

// RawRow is one data row as read from the file.
type RawRow struct {
	Line  int
	Cells []string
}

// Table is a parsed file.
type Table struct {
	Format, Encoding, Separator string
	Columns                     map[string]int // canonical name → index
	Rows                        []RawRow
}

func fileError(msg string) error { return apperr.Invalid("file", msg) }

// Parse reads a CSV or XLSX file.
func Parse(filename string, data []byte) (*Table, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fileError("Fișierul este gol.")
	}
	var t *Table
	var err error
	if strings.EqualFold(filepath.Ext(filename), ".xlsx") || bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		t, err = parseXLSX(data)
	} else {
		t, err = parseCSV(data)
	}
	if err != nil {
		return nil, err
	}
	if len(t.Rows) > MaxRows {
		return nil, fileError(fmt.Sprintf("Fișierul are mai mult de %d de rânduri.", MaxRows))
	}
	return t, nil
}

// Limits that keep a hostile file from exhausting memory.
const (
	maxCells     = 50      // cells per row (after trailing empty cells)
	maxTextBytes = 8 << 20 // total cell text in one file
)

// collector accumulates non-blank rows within the limits.
type collector struct {
	rows  []RawRow
	bytes int
}

func (c *collector) add(line int, cells []string) error {
	for len(cells) > 0 && strings.TrimSpace(cells[len(cells)-1]) == "" {
		cells = cells[:len(cells)-1]
	}
	if len(cells) == 0 {
		return nil // blank rows are ignored and not counted
	}
	if len(cells) > maxCells {
		return fileError(fmt.Sprintf("Fișierul are prea multe coloane (rândul %d).", line))
	}
	for _, s := range cells {
		c.bytes += len(s)
	}
	if c.bytes > maxTextBytes {
		return fileError("Fișierul conține prea mult text pentru o listă de locații.")
	}
	c.rows = append(c.rows, RawRow{Line: line, Cells: cells})
	if len(c.rows) > MaxRows+1 { // header + MaxRows data rows
		return fileError(fmt.Sprintf("Fișierul are mai mult de %d de rânduri.", MaxRows))
	}
	return nil
}

func parseCSV(data []byte) (*Table, error) {
	head := data
	if len(head) > 8192 {
		head = head[:8192]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, fileError("Fișierul nu poate fi citit ca CSV sau XLSX.")
	}
	text, enc := decodeText(data)
	sep := detectSeparator(text)
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = sep
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.ReuseRecord = false
	var c collector
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fileError("Fișierul nu poate fi citit ca CSV sau XLSX.")
		}
		line, _ := r.FieldPos(0)
		if err := c.add(line, rec); err != nil {
			return nil, err
		}
	}
	return table("csv", enc, string(sep), c.rows)
}

func parseXLSX(data []byte) (*Table, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 20 << 20, UnzipXMLSizeLimit: 10 << 20})
	if err != nil {
		return nil, fileError("Fișierul nu poate fi citit ca CSV sau XLSX.")
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fileError("Fișierul este gol.")
	}
	// Stream row by row so the limits apply before the whole sheet is built.
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, fileError("Fișierul nu poate fi citit ca CSV sau XLSX.")
	}
	defer rows.Close()
	var c collector
	for line := 1; rows.Next(); line++ {
		cells, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fileError("Fișierul nu poate fi citit ca CSV sau XLSX.")
		}
		if err := c.add(line, cells); err != nil {
			return nil, err
		}
	}
	if rows.Error() != nil {
		return nil, fileError("Fișierul nu poate fi citit ca CSV sau XLSX.")
	}
	return table("xlsx", "", "", c.rows)
}

func blank(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func table(format, enc, sep string, records []RawRow) (*Table, error) {
	for len(records) > 0 && blank(records[0].Cells) {
		records = records[1:]
	}
	if len(records) == 0 {
		return nil, fileError("Fișierul este gol.")
	}
	t := &Table{Format: format, Encoding: enc, Separator: sep, Columns: map[string]int{}}
	for i, h := range records[0].Cells {
		h = strings.TrimPrefix(h, string(rune(0xFEFF))) // BOM left by some XLSX producers
		if canon, ok := synonyms[Fold(h)]; ok {
			if _, dup := t.Columns[canon]; !dup {
				t.Columns[canon] = i
			}
		}
	}
	var missing []string
	for _, c := range []string{"location", "device", "host"} {
		if _, ok := t.Columns[c]; !ok {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return nil, fileError("Lipsesc coloanele obligatorii: " + strings.Join(missing, ", ") + ".")
	}
	for _, r := range records[1:] {
		if blank(r.Cells) {
			continue
		}
		for i := range r.Cells {
			r.Cells[i] = fixCedilla(strings.TrimSpace(r.Cells[i]))
		}
		t.Rows = append(t.Rows, r)
	}
	return t, nil
}

// CellError is a validation error on one cell.
type CellError struct {
	Column  string `json:"column"`
	Message string `json:"message"`
}

// PreviewRow is one data row with its errors.
type PreviewRow struct {
	Line   int               `json:"line"`
	Values map[string]string `json:"values"`
	Errors []CellError       `json:"errors"`
}

// Preview is the validation result shown before an import.
type Preview struct {
	Format     string       `json:"format"`
	Encoding   string       `json:"encoding"`
	Separator  string       `json:"separator"`
	Rows       []PreviewRow `json:"rows"`
	Locations  int          `json:"locations"`
	Devices    int          `json:"devices"`
	ErrorCount int          `json:"error_count"`
	Valid      bool         `json:"valid"`
}

func parseCoord(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func parsePrimary(s string) (bool, bool) {
	switch Fold(s) {
	case "1", "da", "true", "x", "yes":
		return true, true
	case "", "0", "nu", "false", "no":
		return false, true
	}
	return false, false
}

type group struct {
	in       domain.LocationInput
	rows     []int // indexes into Preview.Rows, one per device
	firstRow map[string]int
	bad      bool
}

// Build validates the table. existing holds Fold(name) of active locations.
// The locations are returned only when the whole file is valid.
func Build(t *Table, existing map[string]bool, s settings.Settings) (Preview, []domain.LocationInput) {
	p := Preview{Format: t.Format, Encoding: t.Encoding, Separator: t.Separator, Rows: []PreviewRow{}}
	_, hasPrimary := t.Columns["is_primary"]
	groups := map[string]*group{}
	var order []string

	for _, raw := range t.Rows {
		row := PreviewRow{Line: raw.Line, Values: map[string]string{}, Errors: []CellError{}}
		for _, c := range Columns {
			if i, ok := t.Columns[c]; ok && i < len(raw.Cells) {
				row.Values[c] = raw.Cells[i]
			}
		}
		v := row.Values
		bad := func(col, msg string) { row.Errors = append(row.Errors, CellError{col, msg}) }

		name := v["location"]
		if n := utf8.RuneCountInString(name); n < 1 || n > 120 {
			bad("location", "Denumirea locației trebuie să aibă între 1 și 120 de caractere.")
		}
		if n := utf8.RuneCountInString(v["device"]); n < 1 || n > 120 {
			bad("device", "Denumirea echipamentului trebuie să aibă între 1 și 120 de caractere.")
		}
		if !domain.ValidHost(v["host"]) {
			bad("host", "Adresă IP sau nume DNS invalid: "+v["host"])
		}
		if utf8.RuneCountInString(v["kind"]) > 50 {
			bad("kind", "Tipul echipamentului poate avea cel mult 50 de caractere.")
		}
		primary, okP := parsePrimary(v["is_primary"])
		if !okP {
			bad("is_primary", "Valoare neacceptată pentru primar: "+v["is_primary"]+" (folosiți da/nu, 1/0, x).")
		}
		var lat, lon *float64
		if v["lat"] != "" || v["lon"] != "" {
			a, okA := parseCoord(v["lat"])
			b, okB := parseCoord(v["lon"])
			switch {
			case v["lat"] == "":
				bad("lat", "Introduceți ambele coordonate sau niciuna.")
			case v["lon"] == "":
				bad("lon", "Introduceți ambele coordonate sau niciuna.")
			case !okA || a < -90 || a > 90:
				bad("lat", "Latitudinea trebuie să fie un număr între -90 și 90.")
			case !okB || b < -180 || b > 180:
				bad("lon", "Longitudinea trebuie să fie un număr între -180 și 180.")
			default:
				lat, lon = &a, &b
			}
		}
		if utf8.RuneCountInString(v["address"]) > 500 {
			bad("address", "Adresa poate avea cel mult 500 de caractere.")
		}
		if utf8.RuneCountInString(v["notes"]) > 4000 {
			bad("notes", "Observațiile pot avea cel mult 4000 de caractere.")
		}

		key := Fold(name)
		g, seen := groups[key]
		if !seen && name != "" {
			g = &group{in: domain.LocationInput{Name: name, Address: v["address"], Notes: v["notes"], Lat: lat, Lon: lon}, firstRow: map[string]int{}}
			for _, c := range []string{"address", "notes", "lat", "lon"} {
				g.firstRow[c] = raw.Line
			}
			groups[key] = g
			order = append(order, key)
			if existing[key] {
				bad("location", "Locația există deja; importul nu modifică locațiile existente.")
			}
			if len(order) > MaxLocations {
				bad("location", fmt.Sprintf("Fișierul are mai mult de %d de locații.", MaxLocations))
			}
		} else if seen {
			conflict := func(col, have, got string) {
				if got != "" && have != "" && got != have {
					bad(col, fmt.Sprintf("Diferă de rândul %d pentru aceeași locație.", g.firstRow[col]))
				}
			}
			conflict("address", g.in.Address, v["address"])
			conflict("notes", g.in.Notes, v["notes"])
			if lat != nil && g.in.Lat != nil && (*lat != *g.in.Lat || *lon != *g.in.Lon) {
				bad("lat", fmt.Sprintf("Diferă de rândul %d pentru aceeași locație.", g.firstRow["lat"]))
			}
			if g.in.Address == "" && v["address"] != "" {
				g.in.Address, g.firstRow["address"] = v["address"], raw.Line
			}
			if g.in.Notes == "" && v["notes"] != "" {
				g.in.Notes, g.firstRow["notes"] = v["notes"], raw.Line
			}
			if g.in.Lat == nil && lat != nil {
				g.in.Lat, g.in.Lon, g.firstRow["lat"] = lat, lon, raw.Line
			}
		}
		p.Rows = append(p.Rows, row)
		if g != nil {
			g.in.Devices = append(g.in.Devices, domain.DeviceInput{Name: v["device"], Host: v["host"], Kind: v["kind"], Primary: primary})
			g.rows = append(g.rows, len(p.Rows)-1)
			if len(row.Errors) > 0 {
				g.bad = true
			}
		}
	}

	var out []domain.LocationInput
	for _, key := range order {
		g := groups[key]
		if !hasPrimary {
			for i := range g.in.Devices {
				g.in.Devices[i].Primary = i == 0
			}
		}
		p.Devices += len(g.in.Devices)
		if g.bad {
			continue
		}
		if err := g.in.Normalize(s); err != nil {
			var e *apperr.Error
			msg, col, idx := err.Error(), "location", 0
			if errors.As(err, &e) {
				msg = e.Message
				var di int
				var field string
				if n, _ := fmt.Sscanf(strings.Replace(e.Field, ".", " ", 2), "devices %d %s", &di, &field); n == 2 && di < len(g.rows) {
					idx, col = di, importColumn(field)
				} else if e.Field == "devices" {
					col = "is_primary"
				}
			}
			r := &p.Rows[g.rows[idx]]
			r.Errors = append(r.Errors, CellError{col, msg})
			continue
		}
		out = append(out, g.in)
	}
	p.Locations = len(order)
	for _, r := range p.Rows {
		p.ErrorCount += len(r.Errors)
	}
	p.Valid = p.ErrorCount == 0 && len(out) > 0
	if !p.Valid {
		return p, nil
	}
	return p, out
}

func importColumn(field string) string {
	switch field {
	case "name":
		return "device"
	case "host", "kind":
		return field
	}
	return "device"
}
