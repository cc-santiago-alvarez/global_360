package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"global_360/internal/application/dto"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
	"global_360/internal/testutil/fixture"
)

const password = "correct-horse-battery"

func expectKind(t *testing.T, err, kind error) {
	t.Helper()
	if !errors.Is(err, kind) {
		t.Fatalf("expected %v, got %v", kind, err)
	}
}

func TestLoginSuccessAuditsAndIssuesTokens(t *testing.T) {
	app := fixture.New(t)
	u := app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})

	pair, err := app.Auth.Login(context.Background(), " ADMIN@global360.com ", password)
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.TokenType != "Bearer" {
		t.Fatalf("incomplete token pair: %+v", pair)
	}
	for _, tok := range app.Store.Tokens {
		if tok.TokenHash == pair.RefreshToken {
			t.Fatal("refresh token stored in plain text")
		}
	}
	stored := app.Store.Users[u.ID]
	if stored.LastAccessAt == nil {
		t.Fatal("last_access_at not updated")
	}
	logs := app.Store.AuditEntries()
	if len(logs) != 1 || logs[0].Action != audit.ActionLogin || logs[0].UserID == nil || *logs[0].UserID != u.ID {
		t.Fatalf("login not audited correctly: %+v", logs)
	}

	a, err := app.Auth.Authenticate(context.Background(), pair.AccessToken)
	if err != nil || a.UserID != u.ID || !a.Grants.CanGlobally("role.manage") {
		t.Fatalf("authenticate: %v %+v", err, a)
	}
}

func TestLoginBlocksAfterRepeatedFailures(t *testing.T) {
	app := fixture.New(t)
	u := app.CreateUser(t, "user@acme.com", password, nil)
	ctx := context.Background()

	for i := 0; i < fixture.MaxFailedLogins; i++ {
		_, err := app.Auth.Login(ctx, "user@acme.com", "wrong-password")
		expectKind(t, err, shared.ErrUnauthorized)
	}
	if st := app.Store.Users[u.ID].Status; st != identity.UserBlocked {
		t.Fatalf("user should be blocked, got %s", st)
	}
	_, err := app.Auth.Login(ctx, "user@acme.com", password)
	expectKind(t, err, shared.ErrForbidden)

	_, err = app.Auth.Login(ctx, "nobody@acme.com", password)
	expectKind(t, err, shared.ErrUnauthorized)
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	app := fixture.New(t)
	app.CreateUser(t, "user@acme.com", password, nil)
	ctx := context.Background()

	first, err := app.Auth.Login(ctx, "user@acme.com", password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.Auth.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	// Reusing the old token is treated as theft: every session is revoked.
	_, err = app.Auth.Refresh(ctx, first.RefreshToken)
	expectKind(t, err, shared.ErrUnauthorized)
	_, err = app.Auth.Refresh(ctx, second.RefreshToken)
	expectKind(t, err, shared.ErrUnauthorized)
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	app := fixture.New(t)
	u := app.CreateUser(t, "user@acme.com", password, nil)
	pair, err := app.Auth.Login(context.Background(), "user@acme.com", password)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Auth.Logout(app.As(t, u.ID), pair.RefreshToken); err != nil {
		t.Fatal(err)
	}
	_, err = app.Auth.Refresh(context.Background(), pair.RefreshToken)
	expectKind(t, err, shared.ErrUnauthorized)
}

func TestCreateCompanyRequiresGlobalPermissionAndIsAudited(t *testing.T) {
	app := fixture.New(t)
	admin := app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})
	acme := app.CreateCompany(t, "900")
	clientAdmin := app.CreateUser(t, "boss@acme.com", password, &acme, fixture.Grant{Role: "client_admin", CompanyID: &acme})

	cmd := dto.CreateCompany{
		LegalName: "Beta SAS", DocumentType: "NIT", DocumentNumber: "901", CountryID: app.CountryCO,
		BillingCurrencyID: app.CurrencyCOP, CompanyType: company.TypeProvider,
	}
	_, err := app.Companies.Create(app.As(t, clientAdmin.ID), cmd)
	expectKind(t, err, shared.ErrForbidden)

	c, err := app.Companies.Create(app.As(t, admin.ID), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != company.StatusPendingValidation {
		t.Fatalf("status = %s", c.Status)
	}
	_, err = app.Companies.Create(app.As(t, admin.ID), cmd)
	expectKind(t, err, shared.ErrConflict)

	updated, err := app.Companies.ChangeStatus(app.As(t, admin.ID), c.ID, company.StatusActive)
	if err != nil || updated.Status != company.StatusActive {
		t.Fatalf("change status: %v", err)
	}
	logs, err := app.Audit.List(app.As(t, admin.ID), dto.ListAuditLogs{Entity: fixture.Ptr(audit.EntityCompany), EntityID: &c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if logs.Total != 2 || logs.Items[0].Action != audit.ActionUpdate || logs.Items[0].OldValues["status"] != "pending_validation" || logs.Items[0].NewValues["status"] != "active" {
		t.Fatalf("unexpected audit trail: %+v", logs.Items)
	}
	_, err = app.Audit.List(app.As(t, clientAdmin.ID), dto.ListAuditLogs{})
	expectKind(t, err, shared.ErrForbidden)
}

func TestCreateUserNeverLeaksPasswordHash(t *testing.T) {
	app := fixture.New(t)
	admin := app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})
	acme := app.CreateCompany(t, "900")

	out, err := app.Users.Create(app.As(t, admin.ID), dto.CreateUser{
		Email: "Ana@Acme.com", Password: password, CompanyID: &acme,
		Person: dto.PersonInput{DocumentType: "cc", DocumentNumber: "123", DocumentCountryID: app.CountryCO,
			FirstNames: "Ana", LastNames: "Pérez", Email: "ana@acme.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Email != "ana@acme.com" || out.Status != identity.UserPendingVerification || out.Person == nil {
		t.Fatalf("unexpected user: %+v", out)
	}
	stored := app.Store.Users[out.ID]
	payload, _ := json.Marshal(out)
	trail, _ := json.Marshal(app.Store.AuditEntries())
	if strings.Contains(string(payload), stored.PasswordHash) || strings.Contains(string(trail), stored.PasswordHash) ||
		strings.Contains(string(trail), "password_hash") {
		t.Fatal("password hash leaked")
	}
	entities := map[string]bool{}
	for _, l := range app.Store.AuditEntries() {
		entities[l.Entity] = true
	}
	if !entities[audit.EntityPerson] || !entities[audit.EntityUser] {
		t.Fatalf("person and user creation must be audited: %v", entities)
	}

	_, err = app.Users.Create(app.As(t, admin.ID), dto.CreateUser{
		Email: "ana@acme.com", Password: password,
		Person: dto.PersonInput{DocumentType: "CC", DocumentNumber: "999", DocumentCountryID: app.CountryCO,
			FirstNames: "Ana", LastNames: "Dup", Email: "ana@acme.com"},
	})
	expectKind(t, err, shared.ErrConflict)
}

func TestCompanyAdminIsScopedToItsCompany(t *testing.T) {
	app := fixture.New(t)
	acme, beta := app.CreateCompany(t, "900"), app.CreateCompany(t, "901")
	boss := app.CreateUser(t, "boss@acme.com", password, &acme, fixture.Grant{Role: "client_admin", CompanyID: &acme})
	worker := app.CreateUser(t, "worker@acme.com", password, &acme)
	outsider := app.CreateUser(t, "someone@beta.com", password, &beta)
	ctx := app.As(t, boss.ID)

	page, err := app.Users.List(ctx, dto.ListUsers{})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range page.Items {
		if u.CompanyID == nil || *u.CompanyID != acme {
			t.Fatalf("user from another scope visible: %+v", u)
		}
	}
	_, err = app.Users.Get(ctx, outsider.ID)
	expectKind(t, err, shared.ErrForbidden)
	_, err = app.Companies.Get(ctx, beta)
	expectKind(t, err, shared.ErrForbidden)
	companies, err := app.Companies.List(ctx, dto.ListCompanies{})
	if err != nil || companies.Total != 1 || companies.Items[0].ID != acme {
		t.Fatalf("company list not scoped: %v %+v", err, companies)
	}

	// Can grant company roles in its own company to its own users...
	asg, err := app.Users.AssignRole(ctx, worker.ID, dto.AssignRole{RoleID: app.RoleIDs["client_operator"], CompanyID: &acme})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.Users.AssignRole(ctx, worker.ID, dto.AssignRole{RoleID: app.RoleIDs["client_operator"], CompanyID: &acme})
	expectKind(t, err, shared.ErrConflict)
	// ...but not global roles, other companies, or outsiders.
	_, err = app.Users.AssignRole(ctx, worker.ID, dto.AssignRole{RoleID: app.RoleIDs["superadmin"]})
	expectKind(t, err, shared.ErrForbidden)
	_, err = app.Users.AssignRole(ctx, worker.ID, dto.AssignRole{RoleID: app.RoleIDs["client_operator"], CompanyID: &beta})
	expectKind(t, err, shared.ErrForbidden)
	_, err = app.Users.AssignRole(ctx, outsider.ID, dto.AssignRole{RoleID: app.RoleIDs["client_operator"], CompanyID: &acme})
	expectKind(t, err, shared.ErrForbidden)

	// Revocation keeps the record (logical delete).
	if err := app.Users.RevokeRole(ctx, worker.ID, asg.ID); err != nil {
		t.Fatal(err)
	}
	stored := app.Store.Assignments[asg.ID]
	if stored.Active || stored.RevokedAt == nil || stored.RevokedBy == nil || *stored.RevokedBy != boss.ID {
		t.Fatalf("assignment not logically revoked: %+v", stored)
	}
}

func TestUserCanHoldRolesInSeveralCompanies(t *testing.T) {
	app := fixture.New(t)
	admin := app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})
	acme, beta := app.CreateCompany(t, "900"), app.CreateCompany(t, "901")
	consultant := app.CreateUser(t, "consultant@mail.com", password, nil)
	ctx := app.As(t, admin.ID)

	for _, c := range []string{acme, beta} {
		c := c
		if _, err := app.Users.AssignRole(ctx, consultant.ID, dto.AssignRole{RoleID: app.RoleIDs["client_operator"], CompanyID: &c}); err != nil {
			t.Fatal(err)
		}
	}
	roles, err := app.Users.ListRoles(ctx, consultant.ID)
	if err != nil || len(roles) != 2 {
		t.Fatalf("expected 2 assignments, got %d (%v)", len(roles), err)
	}
	g, _ := app.Authz.Grants(context.Background(), consultant.ID)
	if !g.Can("company.read", &acme) || !g.Can("company.read", &beta) || g.CanGlobally("company.read") {
		t.Fatal("grants do not reflect per company roles")
	}
}

func TestChangeStatusRevokesSessions(t *testing.T) {
	app := fixture.New(t)
	admin := app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})
	u := app.CreateUser(t, "user@acme.com", password, nil)
	pair, _ := app.Auth.Login(context.Background(), "user@acme.com", password)

	_, err := app.Users.ChangeStatus(app.As(t, admin.ID), admin.ID, identity.UserInactive)
	expectKind(t, err, shared.ErrValidation)

	if _, err := app.Users.ChangeStatus(app.As(t, admin.ID), u.ID, identity.UserInactive); err != nil {
		t.Fatal(err)
	}
	_, err = app.Auth.Refresh(context.Background(), pair.RefreshToken)
	expectKind(t, err, shared.ErrUnauthorized)
	_, err = app.Auth.Authenticate(context.Background(), pair.AccessToken)
	expectKind(t, err, shared.ErrUnauthorized)
}

func TestSelfPasswordChangeRequiresCurrentPassword(t *testing.T) {
	app := fixture.New(t)
	u := app.CreateUser(t, "user@acme.com", password, nil)
	ctx := app.As(t, u.ID)

	err := app.Users.ChangePassword(ctx, u.ID, dto.ChangePassword{CurrentPassword: "nope", NewPassword: "another-long-password"})
	expectKind(t, err, shared.ErrValidation)
	if err := app.Users.ChangePassword(ctx, u.ID, dto.ChangePassword{CurrentPassword: password, NewPassword: "another-long-password"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Auth.Login(context.Background(), "user@acme.com", "another-long-password"); err != nil {
		t.Fatal(err)
	}
}

func TestRolePermissionsAreValidatedAgainstCatalog(t *testing.T) {
	app := fixture.New(t)
	admin := app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})
	ctx := app.As(t, admin.ID)

	_, err := app.Roles.Create(ctx, dto.CreateRole{Name: "auditor", Scope: "global", Permissions: []string{"audit.read", "quote.approve"}})
	expectKind(t, err, shared.ErrValidation)
	r, err := app.Roles.Create(ctx, dto.CreateRole{Name: "auditor", Scope: "global", Permissions: []string{"audit.read"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.Roles.ReplacePermissions(ctx, app.RoleIDs["superadmin"], []string{"audit.read"})
	expectKind(t, err, shared.ErrValidation)
	_, err = app.Roles.Update(ctx, app.RoleIDs["finance"], dto.UpdateRole{Active: fixture.Ptr(false)})
	expectKind(t, err, shared.ErrValidation)
	out, err := app.Roles.ReplacePermissions(ctx, r.ID, []string{"user.read", "audit.read", "user.read"})
	if err != nil || len(out.Permissions) != 2 {
		t.Fatalf("replace permissions: %v %+v", err, out.Permissions)
	}
}
