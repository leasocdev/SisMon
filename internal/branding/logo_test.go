package branding

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

	"netwatch/internal/apperr"
)

func chunk(typ string, data []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(len(data)))
	b.WriteString(typ)
	b.Write(data)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	return b.Bytes()
}

func pngWith(w, h uint32, extra ...[]byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2 // 8-bit RGB
	b.Write(chunk("IHDR", ihdr))
	for _, e := range extra {
		b.Write(e)
	}
	b.Write(chunk("IDAT", []byte{0x78, 0x9c, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01}))
	b.Write(chunk("IEND", nil))
	return b.Bytes()
}

func validPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{200, 30, 30, 255})
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func msg(t *testing.T, err error) string {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Field != "logo" || e.Status != 400 {
		t.Fatalf("want logo 400 error, got %v", err)
	}
	return e.Message
}

func TestPrepareRejectsBomb(t *testing.T) {
	start := time.Now()
	_, err := Prepare(pngWith(50000, 50000))
	if m := msg(t, err); !strings.Contains(m, "1024 × 1024") {
		t.Fatal(m)
	}
	if time.Since(start) > time.Second {
		t.Fatal("too slow: image was decoded")
	}
}

func TestPrepareRejectsCorruptCRC(t *testing.T) {
	b := validPNG(t, 40, 20)
	i := bytes.Index(b, []byte("IDAT"))
	b[i+6] ^= 0xFF
	if m := msg(t, mustErr(Prepare(b))); !strings.Contains(m, "deteriorat") {
		t.Fatal(m)
	}
}

func mustErr(_ []byte, err error) error { return err }

func TestPrepareRejectsJPEG(t *testing.T) {
	var b bytes.Buffer
	jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 10, 10)), nil)
	if m := msg(t, mustErr(Prepare(b.Bytes()))); m != "Logo-ul trebuie să fie o imagine PNG." {
		t.Fatal(m)
	}
	if m := msg(t, mustErr(Prepare([]byte("nu e imagine")))); m != "Logo-ul trebuie să fie o imagine PNG." {
		t.Fatal(m)
	}
}

func TestPrepareRejectsLarge(t *testing.T) {
	big := append(validPNG(t, 10, 10), bytes.Repeat([]byte{0}, 600<<10)...)
	if m := msg(t, mustErr(Prepare(big))); !strings.Contains(m, "500 KB") {
		t.Fatal(m)
	}
}

func TestPrepareReencodes(t *testing.T) {
	src := validPNG(t, 100, 40)
	// insert a tEXt chunk after IHDR (8-byte signature + 25-byte IHDR chunk)
	withText := append(append(append([]byte{}, src[:33]...), chunk("tEXt", []byte("Comment\x00secret"))...), src[33:]...)
	out, err := Prepare(withText)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("tEXt")) || bytes.Contains(out, []byte("secret")) {
		t.Fatal("metadata kept")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 100 || cfg.Height != 40 {
		t.Fatal(cfg, err)
	}
}

func TestSaveLoadDelete(t *testing.T) {
	dir := t.TempDir()
	if b, err := Load(dir); b != nil || err != nil {
		t.Fatal(b, err)
	}
	p := validPNG(t, 8, 8)
	if err := Save(dir, p); err != nil {
		t.Fatal(err)
	}
	if b, _ := Load(dir); !bytes.Equal(b, p) {
		t.Fatal("load")
	}
	if err := Delete(dir); err != nil {
		t.Fatal(err)
	}
	if err := Delete(dir); err != nil {
		t.Fatal("delete twice", err)
	}
}
