package importer

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"netwatch/internal/apperr"
	"netwatch/internal/domain"
	"netwatch/internal/settings"
)

func enc1250(t *testing.T, s string) []byte {
	t.Helper()
	b, err := charmap.Windows1250.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func build(t *testing.T, name string, data []byte, existing ...string) (Preview, []domain.LocationInput) {
	t.Helper()
	tab, err := Parse(name, data)
	if err != nil {
		t.Fatal(err)
	}
	ex := map[string]bool{}
	for _, e := range existing {
		ex[Fold(e)] = true
	}
	return Build(tab, ex, settings.Defaults())
}

func fileErr(t *testing.T, err error) string {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Status != 400 || e.Field != "file" {
		t.Fatalf("want a 400 file error, got %v", err)
	}
	return e.Message
}

func rowErrors(p Preview) []string {
	var out []string
	for _, r := range p.Rows {
		for _, e := range r.Errors {
			out = append(out, fmt.Sprintf("%d:%s:%s", r.Line, e.Column, e.Message))
		}
	}
	return out
}

func TestParseExcelRomanian(t *testing.T) {
	// Excel with Romanian regional settings: ';', Windows-1250, cedilla letters, comma decimals.
	src := "Locaţie;Adresă;Latitudine;Longitudine;Echipament;IP;Tip;Primar\r\nŞtefan Vodă;str. Ţării 1;46,51;29,66;Router;10.0.0.1;Router;da\r\n"
	tab, err := Parse("export.csv", enc1250(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if tab.Encoding != "windows-1250" || tab.Separator != ";" {
		t.Fatalf("%s %q", tab.Encoding, tab.Separator)
	}
	p, locs := Build(tab, map[string]bool{}, settings.Defaults())
	if !p.Valid || len(locs) != 1 {
		t.Fatalf("%+v", rowErrors(p))
	}
	l := locs[0]
	if l.Name != "Ștefan Vodă" || l.Address != "str. Țării 1" || l.Lat == nil || *l.Lat != 46.51 || *l.Lon != 29.66 {
		t.Fatalf("%+v", l)
	}
	if !l.Devices[0].Primary || l.Devices[0].Host != "10.0.0.1" {
		t.Fatalf("%+v", l.Devices)
	}
}

func TestParseUTF8BOMComma(t *testing.T) {
	src := "\xEF\xBB\xBFlocation,lat,lon,device,host\nCahul,\"45,9075\",\"28,1944\",Router,10.0.1.1\n"
	p, locs := build(t, "a.csv", []byte(src))
	if !p.Valid || p.Encoding != "utf-8" || p.Separator != "," || *locs[0].Lat != 45.9075 {
		t.Fatalf("%+v %v", p, rowErrors(p))
	}
}

func TestParseTab(t *testing.T) {
	p, locs := build(t, "a.txt", []byte("location\tdevice\thost\nOrhei\tR\t10.0.3.1\n"))
	if !p.Valid || p.Separator != "\t" || locs[0].Name != "Orhei" {
		t.Fatalf("%+v", p)
	}
}

func TestParseWindows1252(t *testing.T) {
	b, _ := charmap.Windows1252.NewEncoder().Bytes([]byte("location;device;host\nCafé;R;10.0.0.9\n"))
	p, locs := build(t, "a.csv", b)
	if p.Encoding != "windows-1252" || locs[0].Name != "Café" {
		t.Fatalf("%+v %+v", p, locs)
	}
}

func TestHeaderSynonyms(t *testing.T) {
	p, _ := build(t, "a.csv", []byte("LOCATIE;Echipament;adresa ip;Observații\nX;R;10.0.0.1;nota\n"))
	if !p.Valid {
		t.Fatalf("%v", rowErrors(p))
	}
	_, err := Parse("a.csv", []byte("locatie;echipament\nX;R\n"))
	if msg := fileErr(t, err); !strings.Contains(msg, "host") {
		t.Fatal(msg)
	}
}

func TestBinaryRejected(t *testing.T) {
	b := make([]byte, 2048)
	rand.New(rand.NewSource(1)).Read(b)
	b[10] = 0
	_, err := Parse("x.csv", b)
	fileErr(t, err)
	_, err = Parse("x.csv", nil)
	fileErr(t, err)
}

func TestCorruptXLSX(t *testing.T) {
	_, err := Parse("x.xlsx", append([]byte("PK\x03\x04"), bytes.Repeat([]byte{7}, 500)...))
	fileErr(t, err)
}

func TestXLSXRoundTrip(t *testing.T) {
	f := excelize.NewFile()
	sh := f.GetSheetName(0)
	rows := [][]any{{"Locație", "Latitudine", "Longitudine", "Echipament", "Host", "Primar"}, {"Cahul", 45.9075, 28.1944, "Router", "10.0.1.1", "x"}, {"Cahul", 45.9075, 28.1944, "AP", "10.0.1.2", ""}}
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		f.SetSheetRow(sh, cell, &r)
	}
	var buf bytes.Buffer
	f.Write(&buf)
	px, lx := build(t, "a.xlsx", buf.Bytes())
	pc, lc := build(t, "a.csv", []byte("Locație;Latitudine;Longitudine;Echipament;Host;Primar\nCahul;45,9075;28,1944;Router;10.0.1.1;x\nCahul;45,9075;28,1944;AP;10.0.1.2;\n"))
	if !px.Valid || !pc.Valid || px.Format != "xlsx" {
		t.Fatalf("%v %v", rowErrors(px), rowErrors(pc))
	}
	a, b := fmt.Sprintf("%+v %v %v", lx[0].Devices, *lx[0].Lat, *lx[0].Lon), fmt.Sprintf("%+v %v %v", lc[0].Devices, *lc[0].Lat, *lc[0].Lon)
	if a != b {
		t.Fatalf("xlsx %s\ncsv  %s", a, b)
	}
}

func TestTooManyRows(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("location;device;host\n")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&sb, "L%d;R;10.1.%d.%d\n", i/5, i/250, i%250+1)
	}
	if _, err := Parse("a.csv", []byte(sb.String())); err != nil {
		t.Fatal(err)
	}
	sb.WriteString("Lx;R;10.9.9.9\n")
	_, err := Parse("a.csv", []byte(sb.String()))
	if msg := fileErr(t, err); !strings.Contains(msg, "mai mult de 1000") {
		t.Fatal(msg)
	}
}

func TestBuildGroupsAndPrimary(t *testing.T) {
	_, locs := build(t, "a.csv", []byte("location;device;host\nA;R1;10.0.0.1\nB;R2;10.0.0.2\nA;S1;10.0.0.3\nB;S2;10.0.0.4\n"))
	if len(locs) != 2 || locs[0].Name != "A" || len(locs[0].Devices) != 2 || !locs[0].Devices[0].Primary || locs[0].Devices[1].Primary {
		t.Fatalf("%+v", locs)
	}
	_, locs = build(t, "a.csv", []byte("location;device;host;is_primary\nA;R1;10.0.0.1;\nA;R2;10.0.0.2;X\n"))
	if locs[0].Devices[0].Primary || !locs[0].Devices[1].Primary {
		t.Fatalf("%+v", locs[0].Devices)
	}
	p, _ := build(t, "a.csv", []byte("location;device;host;is_primary\nA;R1;10.0.0.1;poate\n"))
	if errs := rowErrors(p); len(errs) != 1 || !strings.Contains(errs[0], "2:is_primary:Valoare neacceptată pentru primar") {
		t.Fatal(errs)
	}
}

func TestBuildCellErrors(t *testing.T) {
	src := "location;address;lat;lon;device;host\n" +
		"Nou;str. 1;;;R;999.1.1.1\n" + // line 2: bad host
		"Nou;str. 2;;;S;10.0.0.2\n" + // line 3: address conflicts with line 2
		"Geo;;46;;R;10.0.0.3\n" + // line 4: lat without lon
		"Cahul;;;;R;10.0.0.4\n" + // line 5: existing location
		"Dup;;;;R;10.0.0.5\n" +
		"Dup;;;;S;10.0.0.5\n" // line 7: duplicate host in location
	p, locs := build(t, "a.csv", []byte(src), "cahul")
	got := strings.Join(rowErrors(p), "\n")
	for _, want := range []string{"2:host:", "3:address:Diferă de rândul 2", "4:lon:", "5:location:Locația există deja", "7:host:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if p.Valid || locs != nil || p.ErrorCount != 5 {
		t.Fatalf("valid=%v errors=%d", p.Valid, p.ErrorCount)
	}
}

func TestBuildEmptyRowsIgnored(t *testing.T) {
	p, locs := build(t, "a.csv", []byte("location;device;host\n\n;;\nA;R;10.0.0.1\n ; ; \n"))
	if !p.Valid || len(p.Rows) != 1 || len(locs) != 1 || p.Rows[0].Line != 4 {
		t.Fatalf("%+v", p)
	}
}

func TestTooManyLocations(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("location;device;host\n")
	for i := 0; i < 201; i++ {
		fmt.Fprintf(&sb, "L%d;R;10.2.%d.%d\n", i, i/250, i%250+1)
	}
	p, locs := build(t, "a.csv", []byte(sb.String()))
	if p.Valid || locs != nil || !strings.Contains(strings.Join(rowErrors(p), ""), "mai mult de 200 de locații") {
		t.Fatalf("%v", rowErrors(p))
	}
}
