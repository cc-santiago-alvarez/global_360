// Package catalog models the base catalogs: countries and currencies.
package catalog

import (
	"regexp"
	"strings"
	"time"

	"global_360/internal/domain/shared"
)

var (
	countryCodeRe  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyCodeRe = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Country is identified by its ISO 3166-1 alpha-2 code.
type Country struct {
	ID        string
	ISOCode   string
	Name      string
	Active    bool
	CreatedAt time.Time
}

func NewCountry(id, isoCode, name string, now time.Time) (*Country, error) {
	code := strings.ToUpper(strings.TrimSpace(isoCode))
	if !countryCodeRe.MatchString(code) {
		return nil, shared.Validation("iso_code must be an ISO 3166-1 alpha-2 code")
	}
	n, err := shared.RequireText("name", name, 100)
	if err != nil {
		return nil, err
	}
	return &Country{ID: id, ISOCode: code, Name: n, Active: true, CreatedAt: now}, nil
}

func (c *Country) Rename(name string) error {
	n, err := shared.RequireText("name", name, 100)
	if err != nil {
		return err
	}
	c.Name = n
	return nil
}

// Currency is identified by its ISO 4217 code.
type Currency struct {
	ID        string
	ISOCode   string
	Name      string
	Symbol    string
	Decimals  int
	Active    bool
	CreatedAt time.Time
}

func NewCurrency(id, isoCode, name, symbol string, decimals int, now time.Time) (*Currency, error) {
	code := strings.ToUpper(strings.TrimSpace(isoCode))
	if !currencyCodeRe.MatchString(code) {
		return nil, shared.Validation("iso_code must be an ISO 4217 code")
	}
	c := &Currency{ID: id, ISOCode: code, Active: true, CreatedAt: now}
	if err := c.Update(&name, &symbol, &decimals); err != nil {
		return nil, err
	}
	return c, nil
}

// Update changes the provided (non nil) fields.
func (c *Currency) Update(name, symbol *string, decimals *int) error {
	if name != nil {
		n, err := shared.RequireText("name", *name, 60)
		if err != nil {
			return err
		}
		c.Name = n
	}
	if symbol != nil {
		s, err := shared.RequireText("symbol", *symbol, 5)
		if err != nil {
			return err
		}
		c.Symbol = s
	}
	if decimals != nil {
		if *decimals < 0 || *decimals > 4 {
			return shared.Validation("decimals must be between 0 and 4")
		}
		c.Decimals = *decimals
	}
	return nil
}
