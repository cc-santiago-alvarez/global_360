// Package identity models persons, users (credentials) and their sessions.
package identity

import (
	"strings"
	"time"

	"global_360/internal/domain/shared"
)

// Person is an individual identity. It is independent from users: a person may
// exist without system access, and may own more than one user.
type Person struct {
	ID                string
	DocumentType      string
	DocumentNumber    string
	DocumentCountryID string
	FirstNames        string
	LastNames         string
	Email             string
	Phone             *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PersonData holds the editable person attributes.
type PersonData struct {
	DocumentType      string
	DocumentNumber    string
	DocumentCountryID string
	FirstNames        string
	LastNames         string
	Email             string
	Phone             *string
}

func NewPerson(id string, data PersonData, now time.Time) (*Person, error) {
	docType, err := shared.RequireText("document_type", data.DocumentType, 20)
	if err != nil {
		return nil, err
	}
	docNumber, err := shared.RequireText("document_number", data.DocumentNumber, 30)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(data.DocumentCountryID) == "" {
		return nil, shared.Validation("document_country_id is required")
	}
	p := &Person{
		ID:                id,
		DocumentType:      strings.ToUpper(docType),
		DocumentNumber:    docNumber,
		DocumentCountryID: data.DocumentCountryID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := p.Update(&data.FirstNames, &data.LastNames, &data.Email, data.Phone, now); err != nil {
		return nil, err
	}
	return p, nil
}

// Update changes the provided (non nil) contact attributes. Document identity is immutable.
func (p *Person) Update(firstNames, lastNames, email, phone *string, now time.Time) error {
	if firstNames != nil {
		v, err := shared.RequireText("first_names", *firstNames, 120)
		if err != nil {
			return err
		}
		p.FirstNames = v
	}
	if lastNames != nil {
		v, err := shared.RequireText("last_names", *lastNames, 120)
		if err != nil {
			return err
		}
		p.LastNames = v
	}
	if email != nil {
		v, err := shared.NormalizeEmail("email", *email)
		if err != nil {
			return err
		}
		p.Email = v
	}
	if phone != nil {
		v, err := shared.OptionalText("phone", phone, 30)
		if err != nil {
			return err
		}
		p.Phone = v
	}
	p.UpdatedAt = now
	return nil
}
