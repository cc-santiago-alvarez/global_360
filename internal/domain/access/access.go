// Package access models role based access control: permissions, roles and the
// assignment of roles to users (globally or scoped to a company).
package access

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"global_360/internal/domain/shared"
)

// SuperadminRoleName is the system role that always holds every permission.
const SuperadminRoleName = "superadmin"

var (
	permissionCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	roleNameRe       = regexp.MustCompile(`^[a-z][a-z0-9_]{2,79}$`)
)

// Permission is an atomic capability, e.g. "user.manage".
type Permission struct {
	ID          string
	Code        string
	Module      string
	Description string
}

func NewPermission(id, code, module, description string) (*Permission, error) {
	c := strings.TrimSpace(code)
	if !permissionCodeRe.MatchString(c) {
		return nil, shared.Validation("permission code must look like module.action")
	}
	m, err := shared.RequireText("module", module, 60)
	if err != nil {
		return nil, err
	}
	return &Permission{ID: id, Code: c, Module: m, Description: strings.TrimSpace(description)}, nil
}

// Scope defines where a role can be assigned.
type Scope string

const (
	ScopeGlobal  Scope = "global"  // Global 360 internal roles
	ScopeCompany Scope = "company" // client / provider roles, bound to a company
)

var Scopes = []Scope{ScopeGlobal, ScopeCompany}

func (s Scope) Valid() bool { return s == ScopeGlobal || s == ScopeCompany }

// Role groups permission codes.
type Role struct {
	ID          string
	Name        string
	Description *string
	Scope       Scope
	IsSystem    bool
	Active      bool
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewRole(id, name string, description *string, scope Scope, isSystem bool, permissions []string, now time.Time) (*Role, error) {
	if !scope.Valid() {
		return nil, shared.Validation("invalid role scope %q", scope)
	}
	r := &Role{ID: id, Scope: scope, IsSystem: isSystem, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := r.setName(name); err != nil {
		return nil, err
	}
	if err := r.SetDescription(description, now); err != nil {
		return nil, err
	}
	if err := r.replacePermissions(permissions); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Role) setName(name string) error {
	n := strings.ToLower(strings.TrimSpace(name))
	if !roleNameRe.MatchString(n) {
		return shared.Validation("role name must be 3-80 lowercase letters, digits or underscores")
	}
	r.Name = n
	return nil
}

// Rename changes the role name. System roles cannot be renamed.
func (r *Role) Rename(name string, now time.Time) error {
	if r.IsSystem {
		return shared.Validation("system roles cannot be renamed")
	}
	if err := r.setName(name); err != nil {
		return err
	}
	r.UpdatedAt = now
	return nil
}

func (r *Role) SetDescription(description *string, now time.Time) error {
	d, err := shared.OptionalText("description", description, 255)
	if err != nil {
		return err
	}
	r.Description = d
	r.UpdatedAt = now
	return nil
}

// SetActive toggles the role. System roles cannot be deactivated.
func (r *Role) SetActive(active bool, now time.Time) error {
	if r.IsSystem && !active {
		return shared.Validation("system roles cannot be deactivated")
	}
	r.Active = active
	r.UpdatedAt = now
	return nil
}

// ReplacePermissions sets the permission codes. Superadmin permissions are managed by the system.
func (r *Role) ReplacePermissions(codes []string, now time.Time) error {
	if r.Name == SuperadminRoleName {
		return shared.Validation("superadmin permissions are managed by the system")
	}
	if err := r.replacePermissions(codes); err != nil {
		return err
	}
	r.UpdatedAt = now
	return nil
}

func (r *Role) replacePermissions(codes []string) error {
	normalized, err := NormalizePermissionCodes(codes)
	if err != nil {
		return err
	}
	r.Permissions = normalized
	return nil
}

// HasPermission reports whether the role grants code.
func (r *Role) HasPermission(code string) bool {
	for _, p := range r.Permissions {
		if p == code {
			return true
		}
	}
	return false
}

// NormalizePermissionCodes trims, validates, deduplicates and sorts permission codes.
func NormalizePermissionCodes(codes []string) ([]string, error) {
	set := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if !permissionCodeRe.MatchString(c) {
			return nil, shared.Validation("invalid permission code %q", c)
		}
		set[c] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

// RoleAssignment grants a role to a user. CompanyID nil means a global assignment.
// Assignments are never deleted: revoking sets Active=false and records who and when.
type RoleAssignment struct {
	ID         string
	UserID     string
	RoleID     string
	CompanyID  *string
	Active     bool
	AssignedAt time.Time
	AssignedBy *string
	RevokedAt  *time.Time
	RevokedBy  *string
}

// NewRoleAssignment validates that the company scope matches the role scope.
func NewRoleAssignment(id, userID string, role *Role, companyID *string, assignedBy *string, now time.Time) (*RoleAssignment, error) {
	if !role.Active {
		return nil, shared.Validation("role %s is inactive", role.Name)
	}
	switch role.Scope {
	case ScopeGlobal:
		if companyID != nil {
			return nil, shared.Validation("global role %s cannot be bound to a company", role.Name)
		}
	case ScopeCompany:
		if companyID == nil {
			return nil, shared.Validation("company role %s requires company_id", role.Name)
		}
	}
	return &RoleAssignment{
		ID:         id,
		UserID:     userID,
		RoleID:     role.ID,
		CompanyID:  companyID,
		Active:     true,
		AssignedAt: now,
		AssignedBy: assignedBy,
	}, nil
}

func (a *RoleAssignment) Revoke(by *string, now time.Time) error {
	if !a.Active {
		return shared.Validation("role assignment is already revoked")
	}
	a.Active = false
	a.RevokedAt = &now
	a.RevokedBy = by
	return nil
}
