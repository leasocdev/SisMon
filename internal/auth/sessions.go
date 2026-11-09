package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"netwatch/internal/store"
)

// Sessions are administrator sessions stored server-side (hashed tokens).
type Sessions struct {
	Idle, Absolute time.Duration
	st             *store.Store
}

// NewSessions uses 8 h idle and 24 h absolute expiry.
func NewSessions(st *store.Store) *Sessions {
	return &Sessions{Idle: 8 * time.Hour, Absolute: 24 * time.Hour, st: st}
}

func randToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(tok string) []byte {
	h := sha256.Sum256([]byte(tok))
	return h[:]
}

// Create starts a session and returns its token and CSRF token.
func (s *Sessions) Create(ctx context.Context, now time.Time) (string, string, error) {
	tok, err := randToken()
	if err != nil {
		return "", "", err
	}
	csrf, err := randToken()
	if err != nil {
		return "", "", err
	}
	_, err = s.st.DB.ExecContext(ctx, `INSERT INTO sessions(token_hash,created_at,last_seen_at,csrf) VALUES(?,?,?,?)`, hashToken(tok), store.Ms(now), store.Ms(now), csrf)
	return tok, csrf, err
}

// Validate checks a token and refreshes its activity (at most once a minute).
func (s *Sessions) Validate(ctx context.Context, tok string, now time.Time) (string, bool, error) {
	if tok == "" {
		return "", false, nil
	}
	h := hashToken(tok)
	var created, seen int64
	var csrf string
	err := s.st.DB.QueryRowContext(ctx, `SELECT created_at,last_seen_at,csrf FROM sessions WHERE token_hash=?`, h).Scan(&created, &seen, &csrf)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	n := store.Ms(now)
	if n-seen > s.Idle.Milliseconds() || n-created > s.Absolute.Milliseconds() {
		_, err := s.st.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, h)
		return "", false, err
	}
	if n-seen > time.Minute.Milliseconds() {
		if _, err := s.st.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE token_hash=?`, n, h); err != nil {
			return "", false, err
		}
	}
	return csrf, true, nil
}

// Delete ends one session.
func (s *Sessions) Delete(ctx context.Context, tok string) error {
	_, err := s.st.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, hashToken(tok))
	return err
}

// DeleteAll ends every session.
func (s *Sessions) DeleteAll(ctx context.Context) error {
	_, err := s.st.DB.ExecContext(ctx, `DELETE FROM sessions`)
	return err
}

// Cleanup removes expired sessions.
func (s *Sessions) Cleanup(ctx context.Context, now time.Time) error {
	n := store.Ms(now)
	_, err := s.st.DB.ExecContext(ctx, `DELETE FROM sessions WHERE ?-last_seen_at > ? OR ?-created_at > ?`, n, s.Idle.Milliseconds(), n, s.Absolute.Milliseconds())
	return err
}
