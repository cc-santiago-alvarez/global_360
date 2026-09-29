package shared

import (
	"net/mail"
	"strings"
)

// Pagination is a 1-based page request.
type Pagination struct {
	Page     int
	PageSize int
}

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Normalize applies defaults and bounds.
func (p Pagination) Normalize() Pagination {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		p.PageSize = MaxPageSize
	}
	return p
}

// Offset returns the number of records to skip.
func (p Pagination) Offset() int64 { return int64((p.Page - 1) * p.PageSize) }

// RequireText trims value and fails when it is empty or longer than max.
func RequireText(field, value string, max int) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", Validation("%s is required", field)
	}
	if len([]rune(v)) > max {
		return "", Validation("%s must have at most %d characters", field, max)
	}
	return v, nil
}

// OptionalText trims value; nil or blank values become nil.
func OptionalText(field string, value *string, max int) (*string, error) {
	if value == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*value)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > max {
		return nil, Validation("%s must have at most %d characters", field, max)
	}
	return &v, nil
}

// NormalizeEmail lowercases and validates an email address.
func NormalizeEmail(field, value string) (string, error) {
	v, err := RequireText(field, value, 150)
	if err != nil {
		return "", err
	}
	v = strings.ToLower(v)
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v {
		return "", Validation("%s is not a valid email address", field)
	}
	return v, nil
}

// OptionalEmail validates an optional email address.
func OptionalEmail(field string, value *string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	v, err := NormalizeEmail(field, *value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
