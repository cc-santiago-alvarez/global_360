// Package commerce models the B2B marketplace: distributor profiles, the services they
// offer, the categories used to discover them and the contact requests (leads) clients send.
package commerce

import (
	"regexp"
	"strings"
	"time"

	"global_360/internal/domain/shared"
)

var categoryCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{2,49}$`)

// Category groups distributors and services (customs brokerage, ground transport...).
type Category struct {
	ID          string
	Code        string
	Name        string
	Description *string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewCategory(id, code, name string, description *string, now time.Time) (*Category, error) {
	c := strings.ToLower(strings.TrimSpace(code))
	if !categoryCodeRe.MatchString(c) {
		return nil, shared.Validation("code must be 3-50 lowercase letters, digits or underscores")
	}
	cat := &Category{ID: id, Code: c, Active: true, CreatedAt: now}
	if err := cat.Update(&name, description, nil, now); err != nil {
		return nil, err
	}
	return cat, nil
}

// Update changes the provided (non nil) fields. The code is immutable.
func (c *Category) Update(name, description *string, active *bool, now time.Time) error {
	if name != nil {
		n, err := shared.RequireText("name", *name, 100)
		if err != nil {
			return err
		}
		c.Name = n
	}
	if description != nil {
		d, err := shared.OptionalText("description", description, 300)
		if err != nil {
			return err
		}
		c.Description = d
	}
	if active != nil {
		c.Active = *active
	}
	c.UpdatedAt = now
	return nil
}
