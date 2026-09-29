// Package company models organizations: clients, providers and Global 360 itself.
package company

import (
	"strings"
	"time"

	"global_360/internal/domain/shared"
)

// Type classifies the company relationship with Global 360.
type Type string

const (
	TypeClient   Type = "client"
	TypeProvider Type = "provider"
	TypeBoth     Type = "both"
	TypeInternal Type = "internal" // Global 360 itself
)

var Types = []Type{TypeClient, TypeProvider, TypeBoth, TypeInternal}

func (t Type) Valid() bool {
	for _, v := range Types {
		if t == v {
			return true
		}
	}
	return false
}

// Status is the company lifecycle state.
type Status string

const (
	StatusPendingValidation Status = "pending_validation"
	StatusActive            Status = "active"
	StatusSuspended         Status = "suspended"
	StatusInactive          Status = "inactive"
)

var Statuses = []Status{StatusPendingValidation, StatusActive, StatusSuspended, StatusInactive}

func (s Status) Valid() bool {
	for _, v := range Statuses {
		if s == v {
			return true
		}
	}
	return false
}

// allowedTransitions defines the company status state machine.
var allowedTransitions = map[Status][]Status{
	StatusPendingValidation: {StatusActive, StatusInactive},
	StatusActive:            {StatusSuspended, StatusInactive},
	StatusSuspended:         {StatusActive, StatusInactive},
	StatusInactive:          {StatusActive},
}

// CanTransition reports whether from -> to is allowed.
func CanTransition(from, to Status) bool {
	for _, s := range allowedTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Company is the base organization record. Client and provider profiles (phase 2)
// extend it by sharing its ID.
type Company struct {
	ID                string
	LegalName         string
	TradeName         *string
	DocumentType      string
	DocumentNumber    string
	CountryID         string
	BillingCurrencyID string
	Type              Type
	Status            Status
	ContactEmail      *string
	ContactPhone      *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Data holds the attributes provided when a company is created.
type Data struct {
	LegalName         string
	TradeName         *string
	DocumentType      string
	DocumentNumber    string
	CountryID         string
	BillingCurrencyID string
	Type              Type
	ContactEmail      *string
	ContactPhone      *string
}

// Changes holds optional updates; nil fields are left untouched.
type Changes struct {
	LegalName         *string
	TradeName         *string
	BillingCurrencyID *string
	Type              *Type
	ContactEmail      *string
	ContactPhone      *string
}

func New(id string, data Data, now time.Time) (*Company, error) {
	docType, err := shared.RequireText("document_type", data.DocumentType, 20)
	if err != nil {
		return nil, err
	}
	docNumber, err := shared.RequireText("document_number", data.DocumentNumber, 30)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(data.CountryID) == "" {
		return nil, shared.Validation("country_id is required")
	}
	c := &Company{
		ID:             id,
		DocumentType:   strings.ToUpper(docType),
		DocumentNumber: docNumber,
		CountryID:      data.CountryID,
		Status:         StatusPendingValidation,
		CreatedAt:      now,
	}
	err = c.Apply(Changes{
		LegalName:         &data.LegalName,
		TradeName:         data.TradeName,
		BillingCurrencyID: &data.BillingCurrencyID,
		Type:              &data.Type,
		ContactEmail:      data.ContactEmail,
		ContactPhone:      data.ContactPhone,
	}, now)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Apply updates the provided fields. Document identity and country are immutable.
func (c *Company) Apply(ch Changes, now time.Time) error {
	if ch.LegalName != nil {
		v, err := shared.RequireText("legal_name", *ch.LegalName, 200)
		if err != nil {
			return err
		}
		c.LegalName = v
	}
	if ch.TradeName != nil {
		v, err := shared.OptionalText("trade_name", ch.TradeName, 200)
		if err != nil {
			return err
		}
		c.TradeName = v
	}
	if ch.BillingCurrencyID != nil {
		if strings.TrimSpace(*ch.BillingCurrencyID) == "" {
			return shared.Validation("billing_currency_id is required")
		}
		c.BillingCurrencyID = *ch.BillingCurrencyID
	}
	if ch.Type != nil {
		if !ch.Type.Valid() {
			return shared.Validation("invalid company_type %q", *ch.Type)
		}
		c.Type = *ch.Type
	}
	if ch.ContactEmail != nil {
		v, err := shared.OptionalEmail("contact_email", ch.ContactEmail)
		if err != nil {
			return err
		}
		c.ContactEmail = v
	}
	if ch.ContactPhone != nil {
		v, err := shared.OptionalText("contact_phone", ch.ContactPhone, 30)
		if err != nil {
			return err
		}
		c.ContactPhone = v
	}
	c.UpdatedAt = now
	return nil
}

// ChangeStatus moves the company through its state machine.
func (c *Company) ChangeStatus(to Status, now time.Time) error {
	if !to.Valid() {
		return shared.Validation("invalid company status %q", to)
	}
	if !CanTransition(c.Status, to) {
		return shared.Validation("cannot change company status from %s to %s", c.Status, to)
	}
	c.Status = to
	c.UpdatedAt = now
	return nil
}

// AcceptsUsers reports whether users or role assignments may be attached to it.
func (c *Company) AcceptsUsers() bool { return c.Status != StatusInactive }
