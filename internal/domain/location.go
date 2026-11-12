package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"sismon/internal/apperr"
	"sismon/internal/settings"
)

// DeviceInput is one device as submitted by the administrator.
type DeviceInput struct {
	ID        *int64             `json:"id"`
	Name      string             `json:"name"`
	Host      string             `json:"host"`
	Kind      string             `json:"kind"`
	Primary   bool               `json:"is_primary"`
	Overrides settings.Overrides `json:"overrides"`
	// Disabled devices are not monitored; DisabledUntil (ms, optional)
	// re-enables them automatically.
	Disabled      bool   `json:"disabled"`
	DisabledUntil *int64 `json:"disabled_until"`
}

// LocationInput is a location with its devices as submitted.
type LocationInput struct {
	Name    string        `json:"name"`
	Address string        `json:"address"`
	Notes   string        `json:"notes"`
	Lat     *float64      `json:"lat"`
	Lon     *float64      `json:"lon"`
	Devices []DeviceInput `json:"devices"`
}

// DefaultKind is used when a device has no type.
const DefaultKind = "Altele"

func length(field, label, v string, lo, hi int) error {
	n := utf8.RuneCountInString(v)
	if n < lo || n > hi {
		if lo == 0 {
			return apperr.Invalid(field, fmt.Sprintf("%s poate avea cel mult %d caractere.", label, hi))
		}
		return apperr.Invalid(field, fmt.Sprintf("%s trebuie să aibă între %d și %d caractere.", label, lo, hi))
	}
	return nil
}

// Normalize trims and validates the input in place.
func (in *LocationInput) Normalize(s settings.Settings) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	in.Notes = strings.TrimSpace(in.Notes)
	if err := length("name", "Denumirea locației", in.Name, 1, 120); err != nil {
		return err
	}
	if err := length("address", "Adresa", in.Address, 0, 500); err != nil {
		return err
	}
	if err := length("notes", "Observațiile", in.Notes, 0, 4000); err != nil {
		return err
	}
	if (in.Lat == nil) != (in.Lon == nil) {
		f := "lat"
		if in.Lat != nil {
			f = "lon"
		}
		return apperr.Invalid(f, "Introduceți ambele coordonate sau niciuna.")
	}
	if in.Lat != nil {
		lat, lon := *in.Lat, *in.Lon
		if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
			return apperr.Invalid("lat", "Latitudinea trebuie să fie între -90 și 90.")
		}
		if math.IsNaN(lon) || math.IsInf(lon, 0) || lon < -180 || lon > 180 {
			return apperr.Invalid("lon", "Longitudinea trebuie să fie între -180 și 180.")
		}
	}
	if len(in.Devices) < 1 || len(in.Devices) > 50 {
		return apperr.Invalid("devices", "Adăugați între 1 și 50 de echipamente.")
	}
	hosts := map[string]bool{}
	ids := map[int64]bool{}
	primary := false
	for i := range in.Devices {
		d := &in.Devices[i]
		p := fmt.Sprintf("devices.%d.", i)
		d.Name, d.Host, d.Kind = strings.TrimSpace(d.Name), strings.TrimSpace(d.Host), strings.TrimSpace(d.Kind)
		if d.Kind == "" {
			d.Kind = DefaultKind
		}
		if !d.Disabled {
			d.DisabledUntil = nil
		}
		if err := length(p+"name", "Denumirea echipamentului", d.Name, 1, 120); err != nil {
			return err
		}
		if err := length(p+"kind", "Tipul echipamentului", d.Kind, 1, 50); err != nil {
			return err
		}
		if !ValidHost(d.Host) {
			return apperr.Invalid(p+"host", "Adresă IP sau nume DNS invalid: "+d.Host)
		}
		key := strings.ToLower(d.Host)
		if hosts[key] {
			return apperr.Invalid(p+"host", "Adresă duplicată în locație: "+d.Host)
		}
		hosts[key] = true
		if d.ID != nil {
			if ids[*d.ID] {
				return apperr.Invalid(p+"id", "Identificator de echipament duplicat.")
			}
			ids[*d.ID] = true
		}
		if _, err := s.Apply(d.Overrides); err != nil {
			var e *apperr.Error
			if errors.As(err, &e) {
				return apperr.Invalid(p+e.Field, e.Message)
			}
			return err
		}
		primary = primary || d.Primary
	}
	if !primary {
		return apperr.Invalid("devices", "Bifați cel puțin un echipament primar.")
	}
	return nil
}
