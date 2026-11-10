// Package export writes statistics and location lists as CSV, XLSX and PDF.
package export

import (
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

// Filter is one active filter, shown in the exported files.
type Filter struct{ Label, Value string }

// Meta describes an export: titles, period, filters, logo.
type Meta struct {
	Title, AppName string
	From, To       time.Time
	Filters        []Filter
	Generated      time.Time
	Logo           []byte // PNG; empty when no logo is configured
}

// Zone is the time zone of every date in the exports.
var Zone = mustZone()

func mustZone() *time.Location {
	z, err := time.LoadLocation("Europe/Chisinau")
	if err != nil {
		panic(err)
	}
	return z
}

// Safe neutralizes values a spreadsheet would run as a formula.
func Safe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func fmtTime(t time.Time) string { return t.In(Zone).Format("02.01.2006 15:04") }

func fmtMs(ms int64) string { return fmtTime(time.UnixMilli(ms)) }

// num formats with two decimals and a decimal comma; nil is empty.
func num(v *float64) string {
	if v == nil {
		return ""
	}
	return comma(*v)
}

func comma(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 2, 64), ".", ",", 1)
}

func hours(sec float64) float64 { return sec / 3600 }

func yesNo(b bool) string {
	if b {
		return "da"
	}
	return "nu"
}

func period(m Meta) string { return fmtTime(m.From) + " – " + fmtTime(m.To) }
