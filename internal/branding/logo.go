// Package branding validates and stores the uploaded logo.
package branding

import (
	"bytes"
	"errors"
	"image/png"
	"os"
	"path/filepath"

	"netwatch/internal/apperr"
)

const (
	// MaxBytes is the largest accepted logo file.
	MaxBytes = 500 << 10
	// MaxSide is the largest accepted width or height, in pixels.
	MaxSide = 1024
)

// Prepare validates a PNG (size, dimensions before decoding, structure and
// CRCs) and re-encodes it, dropping any metadata chunks.
func Prepare(data []byte) ([]byte, error) {
	if len(data) > MaxBytes {
		return nil, apperr.Invalid("logo", "Logo-ul poate avea cel mult 500 KB.")
	}
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, apperr.Invalid("logo", "Logo-ul trebuie să fie o imagine PNG.")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, apperr.Invalid("logo", "Fișierul PNG este deteriorat.")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxSide || cfg.Height > MaxSide {
		return nil, apperr.Invalid("logo", "Logo-ul poate avea cel mult 1024 × 1024 pixeli.")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, apperr.Invalid("logo", "Fișierul PNG este deteriorat.")
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Path is where the logo is stored.
func Path(dataDir string) string { return filepath.Join(dataDir, "branding", "logo.png") }

// Save writes the (already prepared) logo atomically.
func Save(dataDir string, data []byte) error {
	p := Path(dataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".logo-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Load returns the logo, or nil when none is stored.
func Load(dataDir string) ([]byte, error) {
	b, err := os.ReadFile(Path(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// Delete removes the logo; a missing logo is not an error.
func Delete(dataDir string) error {
	err := os.Remove(Path(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
