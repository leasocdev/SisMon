package export

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"sismon/internal/stats"
)

func pages(b []byte) int {
	return bytes.Count(b, []byte("/Type /Page\n")) + bytes.Count(b, []byte("/Type /Page ")) + bytes.Count(b, []byte("/Type /Page/"))
}

func TestPDFLarge(t *testing.T) {
	rep := sampleReport()
	long := strings.Repeat("Ștefan cel Mare și Sfânt — țară ", 7)
	for i := 0; i < 500; i++ {
		rep.Devices = append(rep.Devices, stats.DeviceRow{ID: int64(i + 10), Location: long, Name: fmt.Sprintf("Echipament %d %s", i, long), Host: "10.0.0.1", Kind: "Router", Availability: f(99.9)})
	}
	for i := 0; i < 300; i++ {
		e := time.Now().UnixMilli()
		rep.Incidents = append(rep.Incidents, stats.IncidentRow{ID: int64(i + 10), Location: long, Device: "R", ObservedAt: e, EndedAt: &e, ReasonText: "Revenire", DurationS: 60})
	}
	var b bytes.Buffer
	if err := StatsPDF(&b, rep, sampleMeta()); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b.Bytes(), []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	if n := pages(b.Bytes()); n < 10 {
		t.Fatalf("pages=%d", n)
	}
}

func TestPDFEmpty(t *testing.T) {
	var b bytes.Buffer
	empty := stats.Report{Devices: []stats.DeviceRow{}, Locations: []stats.LocationRow{}, Incidents: []stats.IncidentRow{}}
	if err := StatsPDF(&b, empty, sampleMeta()); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b.Bytes(), []byte("%PDF-")) || pages(b.Bytes()) != 1 {
		t.Fatalf("pages=%d", pages(b.Bytes()))
	}
}

func TestPDFWithLogo(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(3, 3, color.RGBA{255, 0, 0, 255})
	var p bytes.Buffer
	png.Encode(&p, img)
	m := sampleMeta()
	m.Logo = p.Bytes()
	var b bytes.Buffer
	if err := StatsPDF(&b, sampleReport(), m); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Bytes(), []byte("/Subtype /Image")) {
		t.Fatal("logo missing")
	}
}

func TestPDFIncidentCap(t *testing.T) {
	var rows []stats.IncidentRow
	for i := 0; i < MaxPDFIncidents+500; i++ {
		rows = append(rows, stats.IncidentRow{ID: int64(i), ObservedAt: int64(i) * 1000})
	}
	shown, omitted := pdfIncidents(rows)
	if len(shown) != MaxPDFIncidents || omitted != 500 {
		t.Fatal(len(shown), omitted)
	}
	if shown[0].ID != 500 || shown[len(shown)-1].ID != int64(MaxPDFIncidents+499) {
		t.Fatal("must keep the most recent incidents in their order", shown[0].ID)
	}
	if s, o := pdfIncidents(rows[:10]); len(s) != 10 || o != 0 {
		t.Fatal("small lists unchanged")
	}
}

func TestLogoBox(t *testing.T) {
	for _, c := range []struct{ w, h, maxW, maxH float64 }{{1024, 64, 40, 14}, {64, 64, 40, 14}, {100, 1000, 40, 14}} {
		w, h := logoBox(c.w, c.h)
		if w > c.maxW+1e-9 || h > c.maxH+1e-9 || w <= 0 || h <= 0 {
			t.Fatalf("%v → %v×%v", c, w, h)
		}
		if r := w / h; r < c.w/c.h*0.99 || r > c.w/c.h*1.01 {
			t.Fatalf("aspect ratio lost for %v: %v×%v", c, w, h)
		}
	}
}
