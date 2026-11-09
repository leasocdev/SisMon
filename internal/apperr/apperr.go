// Package apperr defines user-facing errors with an HTTP status and a
// Romanian message.
package apperr

import "net/http"

// Error is an error safe to show to the user.
type Error struct {
	Status  int
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// Invalid is a 400 validation error.
func Invalid(field, msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Field: field, Message: msg}
}

// NotFound is a 404 error.
func NotFound(msg string) *Error { return &Error{Status: http.StatusNotFound, Message: msg} }

// Conflict is a 409 error.
func Conflict(field, msg string) *Error {
	return &Error{Status: http.StatusConflict, Field: field, Message: msg}
}

// Forbidden is a 403 error.
func Forbidden(msg string) *Error { return &Error{Status: http.StatusForbidden, Message: msg} }
