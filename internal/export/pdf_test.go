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

	"netwatch/internal/stats"
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
		rep.Incidents = append(rep.Incidents, stats.IncidentRow{ID: int64(i + 10), Location: long, Device: "R", ObservedAt: e, ConfirmedAt: e, EndedAt: &e, ReasonText: "Revenire", DurationS: 60})
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
