package domain_test

import (
	"errors"
	"testing"
	"time"

	"global_360/internal/domain/access"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

var now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func TestUserBlocksAfterMaxFailedLogins(t *testing.T) {
	u, err := identity.NewUser("u1", "p1", nil, " Admin@Global360.COM ", "hash", now)
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "admin@global360.com" || u.Status != identity.UserPendingVerification {
		t.Fatalf("unexpected new user: %+v", u)
	}
	if u.CanLogin() {
		t.Fatal("pending user must not log in")
	}
	_ = u.ChangeStatus(identity.UserActive, now)
	for i := 1; i <= 3; i++ {
		blocked := u.RegisterFailedLogin(3, now)
		if blocked != (i == 3) {
			t.Fatalf("attempt %d: blocked=%v", i, blocked)
		}
	}
	if u.Status != identity.UserBlocked || u.CanLogin() {
		t.Fatalf("user should be blocked, got %s", u.Status)
	}
	if err := u.ChangeStatus(identity.UserActive, now); err != nil || u.FailedLoginAttempts != 0 {
		t.Fatalf("reactivation should reset attempts: err=%v attempts=%d", err, u.FailedLoginAttempts)
	}
	if err := u.ChangeStatus("deleted", now); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("invalid status accepted: %v", err)
	}
}

func TestPasswordPolicy(t *testing.T) {
	if err := identity.ValidatePassword("short"); !errors.Is(err, shared.ErrValidation) {
		t.Fatal("short password accepted")
	}
	if err := identity.ValidatePassword("long-enough-password"); err != nil {
		t.Fatal(err)
	}
}

func TestCompanyStatusTransitions(t *testing.T) {
	c, err := company.New("c1", company.Data{
		LegalName: "ACME", DocumentType: "nit", DocumentNumber: "900", CountryID: "co", BillingCurrencyID: "cop", Type: company.TypeClient,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != company.StatusPendingValidation || c.DocumentType != "NIT" {
		t.Fatalf("unexpected company: %+v", c)
	}
	if err := c.ChangeStatus(company.StatusSuspended, now); err == nil {
		t.Fatal("pending_validation -> suspended must be rejected")
	}
	for _, s := range []company.Status{company.StatusActive, company.StatusSuspended, company.StatusActive, company.StatusInactive, company.StatusActive} {
		if err := c.ChangeStatus(s, now); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	if _, err := company.New("c2", company.Data{LegalName: "X", DocumentType: "NIT", DocumentNumber: "1", CountryID: "co", BillingCurrencyID: "cop", Type: "partner"}, now); err == nil {
		t.Fatal("invalid company type accepted")
	}
}

func TestRoleAssignmentScope(t *testing.T) {
	global, _ := access.NewRole("r1", "operations", nil, access.ScopeGlobal, true, []string{"company.read"}, now)
	scoped, _ := access.NewRole("r2", "client_admin", nil, access.ScopeCompany, true, []string{"user.manage"}, now)
	companyID := "c1"

	if _, err := access.NewRoleAssignment("a1", "u1", global, &companyID, nil, now); err == nil {
		t.Fatal("global role bound to a company must be rejected")
	}
	if _, err := access.NewRoleAssignment("a2", "u1", scoped, nil, nil, now); err == nil {
		t.Fatal("company role without company must be rejected")
	}
	a, err := access.NewRoleAssignment("a3", "u1", scoped, &companyID, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Revoke(nil, now); err != nil || a.Active || a.RevokedAt == nil {
		t.Fatalf("revoke failed: %v %+v", err, a)
	}
	if err := a.Revoke(nil, now); err == nil {
		t.Fatal("double revoke must fail")
	}
}

func TestSystemRoleProtections(t *testing.T) {
	sa, _ := access.NewRole("r1", access.SuperadminRoleName, nil, access.ScopeGlobal, true, access.AllPermissionCodes(), now)
	if err := sa.Rename("root", now); err == nil {
		t.Fatal("system role renamed")
	}
	if err := sa.SetActive(false, now); err == nil {
		t.Fatal("system role deactivated")
	}
	if err := sa.ReplacePermissions([]string{"user.read"}, now); err == nil {
		t.Fatal("superadmin permissions changed")
	}
	if _, err := access.NormalizePermissionCodes([]string{"Bad Code"}); err == nil {
		t.Fatal("invalid permission code accepted")
	}
}

func TestGrantsResolution(t *testing.T) {
	c1, c2 := "c1", "c2"
	admin, _ := access.NewRole("r1", "client_admin", nil, access.ScopeCompany, true, []string{"user.manage", "user.read"}, now)
	ops, _ := access.NewRole("r2", "operations", nil, access.ScopeGlobal, true, []string{"company.read"}, now)
	inactive, _ := access.NewRole("r3", "custom_role", nil, access.ScopeGlobal, false, []string{"audit.read"}, now)
	_ = inactive.SetActive(false, now)

	g := access.BuildGrants([]*access.RoleAssignment{
		{RoleID: "r1", CompanyID: &c1, Active: true},
		{RoleID: "r2", Active: true},
		{RoleID: "r3", Active: true},
		{RoleID: "r1", CompanyID: &c2, Active: false},
	}, []*access.Role{admin, ops, inactive})

	cases := []struct {
		code    string
		company *string
		want    bool
	}{
		{"user.manage", &c1, true},
		{"user.manage", &c2, false}, // revoked assignment
		{"user.manage", nil, false}, // company grant is not global
		{"company.read", &c2, true}, // global grant applies everywhere
		{"audit.read", nil, false},  // inactive role
	}
	for _, tc := range cases {
		if got := g.Can(tc.code, tc.company); got != tc.want {
			t.Errorf("Can(%s, %v) = %v, want %v", tc.code, tc.company, got, tc.want)
		}
	}
	if !g.HasAny("user.manage") || g.CanGlobally("user.manage") {
		t.Error("HasAny/CanGlobally mismatch")
	}
	if ids := g.CompaniesWith("user.read"); len(ids) != 1 || ids[0] != c1 {
		t.Errorf("CompaniesWith = %v", ids)
	}
}
