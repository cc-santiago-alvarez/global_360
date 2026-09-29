package access

// Permission codes checked by the backend. The authoritative catalog lives in
// the database.
const (
	PermCatalogRead   = "catalog.read"
	PermCatalogManage = "catalog.manage"
	PermCompanyRead   = "company.read"
	PermCompanyManage = "company.manage"
	PermUserRead      = "user.read"
	PermUserManage    = "user.manage"
	PermRoleRead      = "role.read"
	PermRoleManage    = "role.manage"
	PermAuditRead     = "audit.read"

	// Phase 2: marketplace ("commerce").
	PermCommerceManage    = "commerce.manage"
	PermCommerceContact   = "commerce.contact"
	PermDistributorManage = "distributor.manage"
	PermLeadRead          = "lead.read"
	PermLeadManage        = "lead.manage"
)

// PermissionDefinition describes a permission seeded at startup.
type PermissionDefinition struct {
	Code        string
	Module      string
	Description string
}

// DefaultPermissions is the permission catalog.
var DefaultPermissions = []PermissionDefinition{
	{PermCatalogRead, "administration", "Read countries and currencies"},
	{PermCatalogManage, "administration", "Create and update countries and currencies"},
	{PermCompanyRead, "companies", "Read companies"},
	{PermCompanyManage, "companies", "Create and update companies and their status"},
	{PermUserRead, "administration", "Read users and their role assignments"},
	{PermUserManage, "administration", "Create and update users, their status and role assignments"},
	{PermRoleRead, "administration", "Read roles and permissions"},
	{PermRoleManage, "administration", "Create and update roles and assign any role"},
	{PermAuditRead, "administration", "Read the audit trail"},
	{PermCommerceManage, "commerce", "Manage categories, review distributor profiles and read every lead"},
	{PermCommerceContact, "commerce", "Send contact requests to distributors and read the ones sent"},
	{PermDistributorManage, "commerce", "Edit the distributor profile and services of the company"},
	{PermLeadRead, "commerce", "Read the contact requests received by the company"},
	{PermLeadManage, "commerce", "Handle the contact requests received by the company"},
}

// Phase2RolePermissions are the permissions phase 2 adds to each system role. The seed
// applies them once to databases created before phase 2.
var Phase2RolePermissions = map[string][]string{
	"global360_admin":   {PermCommerceManage},
	"operations":        {PermCommerceManage},
	"client_admin":      {PermCommerceContact},
	"client_operator":   {PermCommerceContact},
	"provider_admin":    {PermDistributorManage, PermLeadRead, PermLeadManage},
	"provider_operator": {PermLeadRead, PermLeadManage},
}

// RoleDefinition describes a system role seeded at startup.
type RoleDefinition struct {
	Name        string
	Description string
	Scope       Scope
	Permissions []string
}

// AllPermissionCodes returns every default permission code.
func AllPermissionCodes() []string {
	codes := make([]string, 0, len(DefaultPermissions))
	for _, p := range DefaultPermissions {
		codes = append(codes, p.Code)
	}
	return codes
}

// DefaultRoles are the system roles of the master document.
var DefaultRoles = []RoleDefinition{
	{SuperadminRoleName, "Full control of the platform", ScopeGlobal, AllPermissionCodes()},
	{"global360_admin", "Operation and configuration according to scope", ScopeGlobal, []string{
		PermCatalogRead, PermCatalogManage, PermCompanyRead, PermCompanyManage,
		PermUserRead, PermUserManage, PermRoleRead, PermAuditRead,
		PermCommerceManage,
	}},
	{"operations", "Quotes, orders, shipments and providers", ScopeGlobal, []string{
		PermCatalogRead, PermCompanyRead, PermCompanyManage, PermUserRead, PermRoleRead, PermAuditRead,
		PermCommerceManage,
	}},
	{"finance", "Invoices, payments and reconciliation", ScopeGlobal, []string{
		PermCatalogRead, PermCompanyRead, PermUserRead, PermRoleRead, PermAuditRead,
	}},
	{"client_admin", "Users and operations of their company", ScopeCompany, []string{
		PermCatalogRead, PermCompanyRead, PermUserRead, PermUserManage, PermRoleRead,
		PermCommerceContact,
	}},
	{"client_operator", "Limited access according to permissions", ScopeCompany, []string{
		PermCatalogRead, PermCompanyRead,
		PermCommerceContact,
	}},
	{"provider_admin", "Profile and assigned operations", ScopeCompany, []string{
		PermCatalogRead, PermCompanyRead, PermUserRead, PermUserManage, PermRoleRead,
		PermDistributorManage, PermLeadRead, PermLeadManage,
	}},
	{"provider_operator", "Assigned operations", ScopeCompany, []string{
		PermCatalogRead, PermCompanyRead,
		PermLeadRead, PermLeadManage,
	}},
}
