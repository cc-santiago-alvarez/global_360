// Package shared holds domain primitives reused by every module: error kinds,
// pagination and small validation helpers.
package shared

import (
	"errors"
	"fmt"
)

// Error kinds. Adapters map them to transport codes (HTTP 400/401/403/404/409).
var (
	ErrValidation   = errors.New("validation_error")
	ErrNotFound     = errors.New("not_found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

// DomainError carries a human readable message and wraps one of the error kinds.
type DomainError struct {
	kind error
	msg  string
}

func (e *DomainError) Error() string { return e.msg }
func (e *DomainError) Unwrap() error { return e.kind }

func newError(kind error, format string, args ...any) error {
	return &DomainError{kind: kind, msg: fmt.Sprintf(format, args...)}
}

func Validation(format string, args ...any) error { return newError(ErrValidation, format, args...) }
func NotFound(format string, args ...any) error   { return newError(ErrNotFound, format, args...) }
func Conflict(format string, args ...any) error   { return newError(ErrConflict, format, args...) }
func Unauthorized(format string, args ...any) error {
	return newError(ErrUnauthorized, format, args...)
}
func Forbidden(format string, args ...any) error { return newError(ErrForbidden, format, args...) }
