package api

import (
	"context"
	"strings"
	"unicode/utf8"

	"sismon/internal/apperr"
	"sismon/internal/store"
)

// Identity is the application branding, stored in meta "identity".
type Identity struct {
	Name        string `json:"name"`
	Subtitle    string `json:"subtitle"`
	ReportTitle string `json:"report_title"`
	LogoVersion int64  `json:"logo_version"`
}

// DefaultIdentity is used until the administrator changes it.
func DefaultIdentity() Identity {
	return Identity{Name: "SisMon", Subtitle: "NETWORK MONITOR", ReportTitle: "Raport de disponibilitate"}
}

// LoadIdentity reads the identity, falling back to the default.
func LoadIdentity(ctx context.Context, st *store.Store) (Identity, error) {
	id := DefaultIdentity()
	_, err := st.GetMeta(ctx, "identity", &id)
	return id, err
}

// Normalize trims and validates the text fields.
func (id *Identity) Normalize() error {
	id.Name, id.Subtitle, id.ReportTitle = strings.TrimSpace(id.Name), strings.TrimSpace(id.Subtitle), strings.TrimSpace(id.ReportTitle)
	if n := utf8.RuneCountInString(id.Name); n < 1 || n > 60 {
		return apperr.Invalid("name", "Denumirea aplicației trebuie să aibă între 1 și 60 de caractere.")
	}
	if utf8.RuneCountInString(id.Subtitle) > 80 {
		return apperr.Invalid("subtitle", "Subtitlul poate avea cel mult 80 de caractere.")
	}
	if n := utf8.RuneCountInString(id.ReportTitle); n < 1 || n > 120 {
		return apperr.Invalid("report_title", "Titlul raportului trebuie să aibă între 1 și 120 de caractere.")
	}
	return nil
}
