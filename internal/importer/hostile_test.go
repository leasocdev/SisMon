package importer

import (
	"archive/zip"
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"netwatch/internal/settings"
)

// richTextBomb builds a small XLSX whose 1000×100 cells all point to one
// shared string made of many rich-text runs.
func richTextBomb(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="S" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/></Relationships>`)
	var si strings.Builder
	for i := 0; i < 20; i++ {
		si.WriteString("<r><t>" + strings.Repeat("x", 4000) + "</t></r>")
	}
	add("xl/sharedStrings.xml", `<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="1"><si>`+si.String()+`</si></sst>`)
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r := 1; r <= 1000; r++ {
		fmt.Fprintf(&sheet, `<row r="%d">`, r)
		for c := 0; c < 100; c++ {
			sheet.WriteString(`<c t="s"><v>0</v></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	add("xl/worksheets/sheet1.xml", sheet.String())
	z.Close()
	return buf.Bytes()
}

func TestXLSXRichTextBombBounded(t *testing.T) {
	data := richTextBomb(t)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := Parse("bomb.xlsx", data)
	runtime.ReadMemStats(&after)
	fileErr(t, err)
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 400<<20 {
		t.Fatalf("parsing a %d KB file allocated %d MB", len(data)>>10, alloc>>20)
	}
}

func TestWideRowsRejected(t *testing.T) {
	line := "location;device;host" + strings.Repeat(";x", 200) + "\n"
	_, err := Parse("a.csv", []byte(line+"A;R;10.0.0.1\n"))
	fileErr(t, err)
}

func TestBlankLinesDoNotDropRows(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("location;device;host\n")
	for i := 0; i < 990; i++ {
		fmt.Fprintf(&sb, "L%d;R;10.1.%d.%d\n", i/5, i/250, i%250+1)
		if i%50 == 0 {
			sb.WriteString(";;\n")
		}
	}
	tab, err := Parse("a.csv", []byte(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Rows) != 990 {
		t.Fatalf("rows=%d", len(tab.Rows))
	}
	p, locs := Build(tab, map[string]bool{}, settings.Defaults())
	if !p.Valid || len(locs) != 198 {
		t.Fatalf("valid=%v locations=%d", p.Valid, len(locs))
	}
	for i := 990; i < 1001; i++ {
		fmt.Fprintf(&sb, "M%d;R;10.2.0.%d\n;;\n", i, i-989)
	}
	_, err = Parse("a.csv", []byte(sb.String()))
	if msg := fileErr(t, err); !strings.Contains(msg, "mai mult de 1000") {
		t.Fatal(msg)
	}
}

func TestSeparatorFromFirstNonBlankLine(t *testing.T) {
	p, locs := build(t, "a.csv", []byte("\n\nlocation,device,host\nA,R,10.0.0.1\n"))
	if !p.Valid || p.Separator != "," || len(locs) != 1 {
		t.Fatalf("%+v", p)
	}
}
