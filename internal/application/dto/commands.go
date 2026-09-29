package dto

import (
	"time"

	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

// Optional fields use pointers: nil means "leave unchanged".

type CreateCountry struct {
	ISOCode string
	Name    string
}

type UpdateCountry struct {
	Name   *string
	Active *bool
}

type CreateCurrency struct {
	ISOCode  string
	Name     string
	Symbol   string
	Decimals int
}

type UpdateCurrency struct {
	Name     *string
	Symbol   *string
	Decimals *int
	Active   *bool
}

type CreateCompany struct {
	LegalName         string
	TradeName         *string
	DocumentType      string
	DocumentNumber    string
	CountryID         string
	BillingCurrencyID string
	CompanyType       company.Type
	ContactEmail      *string
	ContactPhone      *string
}

type UpdateCompany struct {
	LegalName         *string
	TradeName         *string
	BillingCurrencyID *string
	CompanyType       *company.Type
	ContactEmail      *string
	ContactPhone      *string
}

type ListCompanies struct {
	Status      *company.Status
	CompanyType *company.Type
	Page        shared.Pagination
}

type PersonInput struct {
	DocumentType      string
	DocumentNumber    string
	DocumentCountryID string
	FirstNames        string
	LastNames         string
	Email             string
	Phone             *string
}

type CreateUser struct {
	Person    PersonInput
	CompanyID *string
	Email     string
	Password  string
}

type UpdateUser struct {
	Email       *string
	FirstNames  *string
	LastNames   *string
	PersonEmail *string
	Phone       *string
}

type ListUsers struct {
	CompanyID *string
	Status    *identity.UserStatus
	Page      shared.Pagination
}

type ChangePassword struct {
	CurrentPassword string // required when users change their own password
	NewPassword     string
}

type AssignRole struct {
	RoleID    string
	CompanyID *string
}

type CreateRole struct {
	Name        string
	Description *string
	Scope       access.Scope
	Permissions []string
}

type UpdateRole struct {
	Name        *string
	Description *string
	Active      *bool
}

type ListAuditLogs struct {
	Entity   *string
	EntityID *string
	UserID   *string
	Action   *audit.Action
	From     *time.Time
	To       *time.Time
	Page     shared.Pagination
}
