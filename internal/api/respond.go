package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"netwatch/internal/apperr"
)

// MaxJSONBody bounds JSON request bodies.
const MaxJSONBody = 1 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type errBody struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

// writeError renders apperr errors with their status; anything else is a
// logged 500 with a generic Romanian message.
func writeError(w http.ResponseWriter, err error) {
	var e *apperr.Error
	if errors.As(err, &e) {
		writeJSON(w, e.Status, errBody{e.Message, e.Field})
		return
	}
	slog.Error("eroare internă", "err", err)
	writeJSON(w, http.StatusInternalServerError, errBody{Error: "Eroare internă a serverului."})
}

// decodeJSON strictly decodes exactly one JSON object into dst.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxJSONBody))
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			return &apperr.Error{Status: http.StatusRequestEntityTooLarge, Message: "Cererea depășește 1 MB."}
		}
		return apperr.Invalid("", "Corpul cererii nu a putut fi citit.")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return apperr.Invalid("", "Corpul cererii lipsește.")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var te *json.UnmarshalTypeError
		switch {
		case errors.As(err, &te):
			return apperr.Invalid(te.Field, "Tip de valoare invalid pentru câmpul „"+te.Field+"”.")
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			f := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
			return apperr.Invalid(f, "Câmp necunoscut: „"+f+"”.")
		}
		return apperr.Invalid("", "JSON invalid.")
	}
	if dec.More() {
		return apperr.Invalid("", "Cererea conține date suplimentare după obiectul JSON.")
	}
	return nil
}
