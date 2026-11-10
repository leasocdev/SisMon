package export

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"netwatch/internal/importer"
	"netwatch/internal/settings"
	"netwatch/internal/stats"
	"netwatch/internal/store"
)

func f(v float64) *float64 { return &v }

func sampleReport() stats.Report {
	return stats.Report{
		From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), To: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC).UnixMilli(),
		Summary: stats.Summary{Availability: f(99.5), Coverage: f(100), Incidents: 1, DownS: 3600, MTTRS: f(1800)},
		Devices: []stats.DeviceRow{
			{ID: 1, LocationID: 1, Location: "Cahul", Name: "Router", Host: "10.0.1.1", Kind: "Router", Primary: true, Durations: stats.Durations{Up: 3600 * 167, Down: 3600},
				Availability: f(99.5), Coverage: f(100), LatencyMs: f(4.25), LossPct: f(0.5), Incidents: 1, MTTRS: f(1800)},
			{ID: 2, LocationID: 2, Location: "+1", Name: `=HYPERLINK("http://x")`, Host: "10.0.2.1", Kind: "@cmd"},
		},
		Locations: []stats.LocationRow{{ID: 1, Name: "Cahul", Durations: stats.Durations{Up: 3600 * 167, Down: 3600}, Availability: f(99.5), Coverage: f(100)}},
		Incidents: []stats.IncidentRow{{ID: 1, DeviceID: 1, LocationID: 1, Location: "Cahul", Device: "Router", ObservedAt: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).UnixMilli(),
			ConfirmedAt: time.Date(2026, 11, 2, 10, 3, 0, 0, time.UTC).UnixMilli(), EndedAt: nil, ReasonText: "În curs", DurationS: 3600}},
	}
}

func sampleMeta() Meta {
	return Meta{Title: "Raport de disponibilitate", AppName: "NetWatch", From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC),
		Filters: []Filter{{"Locație", "Toate"}}, Generated: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
}

func TestStatsCSVFormat(t *testing.T) {
	var b bytes.Buffer
	if err := StatsCSV(&b, sampleReport(), sampleMeta()); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if !strings.HasPrefix(s, "\xEF\xBB\xBF") {
		t.Fatal("BOM")
	}
	lines := strings.Split(strings.TrimPrefix(s, "\xEF\xBB\xBF"), "\r\n")
	if !strings.Contains(lines[0], "Perioadă și filtre") {
		t.Fatal(lines[0])
	}
	if !strings.Contains(s, "Cahul;Router;10.0.1.1;Router;da;99,50;100,00") {
		t.Fatalf("%s", s)
	}
	if !strings.Contains(s, "01.11.2026 02:00") || !strings.Contains(s, "Europe/Chisinau") {
		t.Fatal("period in Chisinau time")
	}
	if strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") {
		t.Fatal("bare LF")
	}
}

func TestFormulaInjectionStats(t *testing.T) {
	var b bytes.Buffer
	StatsCSV(&b, sampleReport(), sampleMeta())
	s := b.String()
	for _, want := range []string{`"'=HYPERLINK(""http://x"")"`, ";'+1;", ";'@cmd;"} {
		if !strings.Contains(s, want) && !strings.Contains(s, strings.Trim(want, ";")) {
			t.Errorf("missing %s in CSV", want)
		}
	}
	var x bytes.Buffer
	if err := StatsXLSX(&x, sampleReport(), sampleMeta()); err != nil {
		t.Fatal(err)
	}
	fx, err := excelize.OpenReader(&x)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := fx.GetRows("Echipamente")
	got := strings.Join(rows[2], "|")
	if !strings.Contains(got, `'=HYPERLINK("http://x")`) || !strings.Contains(got, "'+1") || !strings.Contains(got, "'@cmd") {
		t.Fatal(got)
	}
}

func TestStatsXLSXSheets(t *testing.T) {
	var x bytes.Buffer
	StatsXLSX(&x, sampleReport(), sampleMeta())
	fx, err := excelize.OpenReader(&x)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(fx.GetSheetList(), ","); got != "Echipamente,Locații,Incidente,Perioadă și filtre" {
		t.Fatal(got)
	}
	typ, _ := fx.GetCellType("Echipamente", "F2")
	v, _ := fx.GetCellValue("Echipamente", "F2", excelize.Options{RawCellValue: true})
	if typ == excelize.CellTypeSharedString || typ == excelize.CellTypeInlineString || v != "99.5" {
		t.Fatalf("F2 must be numeric 99.5, got %v %q", typ, v)
	}
}

func sampleLocations() []store.Location {
	return []store.Location{
		{ID: 1, Name: "Ștefan Vodă", Address: "str. Țării 1; bloc 2", Notes: "Acces \"pe scară\"", Lat: f(46.123456789), Lon: f(29.66), Devices: []store.Device{
			{Name: "Router", Host: "10.0.0.1", Kind: "Router", Primary: true}, {Name: "AP", Host: "10.0.0.2", Kind: "Radiolink"}}},
		{ID: 2, Name: "=SUM(A1)", Devices: []store.Device{{Name: "R", Host: "r1.local", Kind: "Altele", Primary: true}}},
		{ID: 3, Name: "Bălți", Devices: []store.Device{{Name: "+R", Host: "10.0.2.1", Kind: "Router", Primary: true}}},
	}
}

func checkRoundTrip(t *testing.T, name string, data []byte) {
	t.Helper()
	tab, err := importer.Parse(name, data)
	if err != nil {
		t.Fatal(err)
	}
	p, ins := importer.Build(tab, map[string]bool{}, settings.Defaults())
	if !p.Valid {
		t.Fatalf("%+v", p.Rows)
	}
	src := sampleLocations()
	if len(ins) != len(src) {
		t.Fatal(len(ins))
	}
	for i, l := range src {
		in := ins[i]
		if in.Name != l.Name || in.Address != l.Address || in.Notes != l.Notes || fmt.Sprint(in.Lat) != fmt.Sprint(l.Lat) && *in.Lat != *l.Lat {
			t.Fatalf("%s: %+v vs %+v", name, in, l)
		}
		if (l.Lat == nil) != (in.Lat == nil) || l.Lat != nil && (*in.Lat != *l.Lat || *in.Lon != *l.Lon) {
			t.Fatalf("coords %s", l.Name)
		}
		for j, d := range l.Devices {
			g := in.Devices[j]
			if g.Name != d.Name || g.Host != d.Host || g.Kind != d.Kind || g.Primary != d.Primary {
				t.Fatalf("%+v vs %+v", g, d)
			}
		}
	}
}

func TestLocationsRoundTrip(t *testing.T) {
	var c bytes.Buffer
	if err := LocationsCSV(&c, sampleLocations()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimPrefix(c.String(), "\xEF\xBB\xBF"), "Locație;Adresă;Latitudine;Longitudine;Echipament;Host;Tip;Primar;Observații") {
		t.Fatal(c.String()[:80])
	}
	if strings.Contains(c.String(), "'=SUM") {
		t.Fatal("locations export must not escape formulas")
	}
	checkRoundTrip(t, "l.csv", c.Bytes())
	var x bytes.Buffer
	if err := LocationsXLSX(&x, sampleLocations()); err != nil {
		t.Fatal(err)
	}
	checkRoundTrip(t, "l.xlsx", x.Bytes())
	fx, _ := excelize.OpenReader(bytes.NewReader(x.Bytes()))
	sh := fx.GetSheetName(0)
	if typ, _ := fx.GetCellType(sh, "A3"); typ != excelize.CellTypeSharedString && typ != excelize.CellTypeInlineString {
		t.Fatalf("A3 must be text, got %v", typ)
	}
	if typ, _ := fx.GetCellType(sh, "C2"); typ != excelize.CellTypeSharedString && typ != excelize.CellTypeInlineString {
		t.Fatalf("coordinates must be text, got %v", typ)
	}
}

func TestStatsEmptyReport(t *testing.T) {
	var c, x bytes.Buffer
	empty := stats.Report{Devices: []stats.DeviceRow{}, Locations: []stats.LocationRow{}, Incidents: []stats.IncidentRow{}}
	if err := StatsCSV(&c, empty, sampleMeta()); err != nil || !strings.Contains(c.String(), "Echipamente") {
		t.Fatal(err)
	}
	if err := StatsXLSX(&x, empty, sampleMeta()); err != nil {
		t.Fatal(err)
	}
	if _, err := excelize.OpenReader(&x); err != nil {
		t.Fatal(err)
	}
}

func TestSafe(t *testing.T) {
	for in, want := range map[string]string{"=1": "'=1", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx", "Cahul": "Cahul", "": ""} {
		if got := Safe(in); got != want {
			t.Errorf("Safe(%q)=%q", in, got)
		}
	}
}

func TestLocationsCSVDecimalComma(t *testing.T) {
	var c bytes.Buffer
	LocationsCSV(&c, sampleLocations())
	if !strings.Contains(c.String(), ";46,123456789;29,66;") {
		t.Fatalf("coordinates must use a decimal comma (Excel RO reads 29.66 as a date): %s", c.String())
	}
}
