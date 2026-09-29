// Package dto contains use case inputs (commands) and outputs (results).
// Results never expose secrets such as password hashes.
package dto

import (
	"time"

	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/catalog"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
)

type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

type Country struct {
	ID        string    `json:"id"`
	ISOCode   string    `json:"iso_code"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

func NewCountry(c *catalog.Country) Country {
	return Country{ID: c.ID, ISOCode: c.ISOCode, Name: c.Name, Active: c.Active, CreatedAt: c.CreatedAt}
}

type Currency struct {
	ID        string    `json:"id"`
	ISOCode   string    `json:"iso_code"`
	Name      string    `json:"name"`
	Symbol    string    `json:"symbol"`
	Decimals  int       `json:"decimals"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

func NewCurrency(c *catalog.Currency) Currency {
	return Currency{ID: c.ID, ISOCode: c.ISOCode, Name: c.Name, Symbol: c.Symbol, Decimals: c.Decimals, Active: c.Active, CreatedAt: c.CreatedAt}
}

type Company struct {
	ID                string         `json:"id"`
	LegalName         string         `json:"legal_name"`
	TradeName         *string        `json:"trade_name"`
	DocumentType      string         `json:"document_type"`
	DocumentNumber    string         `json:"document_number"`
	CountryID         string         `json:"country_id"`
	BillingCurrencyID string         `json:"billing_currency_id"`
	CompanyType       company.Type   `json:"company_type"`
	Status            company.Status `json:"status"`
	ContactEmail      *string        `json:"contact_email"`
	ContactPhone      *string        `json:"contact_phone"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func NewCompany(c *company.Company) Company {
	return Company{
		ID: c.ID, LegalName: c.LegalName, TradeName: c.TradeName, DocumentType: c.DocumentType,
		DocumentNumber: c.DocumentNumber, CountryID: c.CountryID, BillingCurrencyID: c.BillingCurrencyID,
		CompanyType: c.Type, Status: c.Status, ContactEmail: c.ContactEmail, ContactPhone: c.ContactPhone,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

type Person struct {
	ID                string    `json:"id"`
	DocumentType      string    `json:"document_type"`
	DocumentNumber    string    `json:"document_number"`
	DocumentCountryID string    `json:"document_country_id"`
	FirstNames        string    `json:"first_names"`
	LastNames         string    `json:"last_names"`
	Email             string    `json:"email"`
	Phone             *string   `json:"phone"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func NewPerson(p *identity.Person) Person {
	return Person{
		ID: p.ID, DocumentType: p.DocumentType, DocumentNumber: p.DocumentNumber, DocumentCountryID: p.DocumentCountryID,
		FirstNames: p.FirstNames, LastNames: p.LastNames, Email: p.Email, Phone: p.Phone,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// User is the public view of a user. It deliberately omits the password hash.
type User struct {
	ID                  string              `json:"id"`
	PersonID            string              `json:"person_id"`
	CompanyID           *string             `json:"company_id"`
	Email               string              `json:"email"`
	Status              identity.UserStatus `json:"status"`
	MFAEnabled          bool                `json:"mfa_enabled"`
	FailedLoginAttempts int                 `json:"failed_login_attempts"`
	LastAccessAt        *time.Time          `json:"last_access_at"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	Person              *Person             `json:"person,omitempty"`
}

func NewUser(u *identity.User, p *identity.Person) User {
	out := User{
		ID: u.ID, PersonID: u.PersonID, CompanyID: u.CompanyID, Email: u.Email, Status: u.Status,
		MFAEnabled: u.MFAEnabled, FailedLoginAttempts: u.FailedLoginAttempts, LastAccessAt: u.LastAccessAt,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
	if p != nil {
		pv := NewPerson(p)
		out.Person = &pv
	}
	return out
}

type Permission struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Module      string `json:"module"`
	Description string `json:"description"`
}

func NewPermission(p *access.Permission) Permission {
	return Permission{ID: p.ID, Code: p.Code, Module: p.Module, Description: p.Description}
}

type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	Scope       access.Scope `json:"scope"`
	IsSystem    bool         `json:"is_system"`
	Active      bool         `json:"active"`
	Permissions []string     `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

func NewRole(r *access.Role) Role {
	perms := r.Permissions
	if perms == nil {
		perms = []string{}
	}
	return Role{
		ID: r.ID, Name: r.Name, Description: r.Description, Scope: r.Scope, IsSystem: r.IsSystem,
		Active: r.Active, Permissions: perms, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type RoleAssignment struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	RoleID     string     `json:"role_id"`
	RoleName   string     `json:"role_name,omitempty"`
	CompanyID  *string    `json:"company_id"`
	Active     bool       `json:"active"`
	AssignedAt time.Time  `json:"assigned_at"`
	AssignedBy *string    `json:"assigned_by"`
	RevokedAt  *time.Time `json:"revoked_at"`
	RevokedBy  *string    `json:"revoked_by"`
}

func NewRoleAssignment(a *access.RoleAssignment, roleName string) RoleAssignment {
	return RoleAssignment{
		ID: a.ID, UserID: a.UserID, RoleID: a.RoleID, RoleName: roleName, CompanyID: a.CompanyID,
		Active: a.Active, AssignedAt: a.AssignedAt, AssignedBy: a.AssignedBy, RevokedAt: a.RevokedAt, RevokedBy: a.RevokedBy,
	}
}

type AuditLog struct {
	ID        string         `json:"id"`
	UserID    *string        `json:"user_id"`
	Entity    string         `json:"entity"`
	EntityID  string         `json:"entity_id"`
	Action    audit.Action   `json:"action"`
	OldValues map[string]any `json:"old_values"`
	NewValues map[string]any `json:"new_values"`
	SourceIP  *string        `json:"source_ip"`
	UserAgent *string        `json:"user_agent"`
	CreatedAt time.Time      `json:"created_at"`
}

func NewAuditLog(l *audit.Log) AuditLog {
	return AuditLog{
		ID: l.ID, UserID: l.UserID, Entity: l.Entity, EntityID: l.EntityID, Action: l.Action,
		OldValues: l.OldValues, NewValues: l.NewValues, SourceIP: l.SourceIP, UserAgent: l.UserAgent, CreatedAt: l.CreatedAt,
	}
}

type TokenPair struct {
	TokenType             string    `json:"token_type"`
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

// Me describes the authenticated user and its effective permissions.
type Me struct {
	User               User                `json:"user"`
	RoleAssignments    []RoleAssignment    `json:"role_assignments"`
	GlobalPermissions  []string            `json:"global_permissions"`
	CompanyPermissions map[string][]string `json:"company_permissions"`
}
