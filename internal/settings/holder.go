package settings

import (
	"context"
	"sync/atomic"
)

// MetaStore is the subset of the store used for persistence.
type MetaStore interface {
	GetMeta(ctx context.Context, key string, v any) (bool, error)
	SetMeta(ctx context.Context, key string, v any) error
}

const metaKey = "settings"

// Holder keeps the current settings, safe for concurrent use.
type Holder struct{ v atomic.Pointer[Settings] }

// NewHolder returns a holder with s (used by tests).
func NewHolder(s Settings) *Holder {
	h := &Holder{}
	h.v.Store(&s)
	return h
}

// Load reads the settings from st, falling back to Defaults.
func Load(ctx context.Context, st MetaStore) (*Holder, error) {
	s := Defaults()
	if _, err := st.GetMeta(ctx, metaKey, &s); err != nil {
		return nil, err
	}
	return NewHolder(s), nil
}

// Get returns the current settings.
func (h *Holder) Get() Settings { return *h.v.Load() }

// Save validates and persists s, then makes it current.
func (h *Holder) Save(ctx context.Context, st MetaStore, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := st.SetMeta(ctx, metaKey, s); err != nil {
		return err
	}
	h.v.Store(&s)
	return nil
}
