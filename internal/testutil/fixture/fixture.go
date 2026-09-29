// Package fixture wires the real services on top of in-memory adapters for tests.
package fixture

import (
	"context"
	"strings"
	"testing"
	"time"

	"global_360/internal/application/actor"
	"global_360/internal/application/service"
	"global_360/internal/domain/access"
	"global_360/internal/domain/catalog"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
	"global_360/internal/infrastructure/security"
	"global_360/internal/testutil/memory"
)

const MaxFailedLogins = 3

type App struct {
	Store *memory.Store
	Clock *memory.Clock
	IDs   *memory.IDs

	Authz     *service.AuthorizationService
	Auth      *service.AuthService
	Users     *service.UserService
	Companies *service.CompanyService
	Roles     *service.RoleService
	Catalog   *service.CatalogService
	Audit     *service.AuditService

	Categories      *service.CategoryService
	Marketplace     *service.MarketplaceService
	Distributors    *service.DistributorProfileService
	ContactRequests *service.ContactRequestService

	CountryCO, CountryCR, CurrencyCOP string
	RoleIDs                           map[string]string
	CategoryIDs                       map[string]string // by code
}

func New(t *testing.T) *App {
	t.Helper()
	s := memory.NewStore()
	clock := &memory.Clock{T: time.Now().UTC().Truncate(time.Millisecond)}
	ids := &memory.IDs{}
	a := &App{Store: s, Clock: clock, IDs: ids, RoleIDs: map[string]string{}, CategoryIDs: map[string]string{}}

	countries, currencies := memory.CountryRepo{S: s}, memory.CurrencyRepo{S: s}
	persons, users, tokens := memory.PersonRepo{S: s}, memory.UserRepo{S: s}, memory.TokenRepo{S: s}
	companies, roles, perms := memory.CompanyRepo{S: s}, memory.RoleRepo{S: s}, memory.PermissionRepo{S: s}
	assignments, audits := memory.AssignmentRepo{S: s}, memory.AuditRepo{S: s}
	categories, profiles := memory.CategoryRepo{S: s}, memory.ProfileRepo{S: s}
	services, requests := memory.ServiceRepo{S: s}, memory.ContactRequestRepo{S: s}

	a.Audit = service.NewAuditService(audits, clock)
	deps := service.Deps{Tx: memory.Tx{}, Clock: clock, IDs: ids, Audit: a.Audit}
	a.Authz = service.NewAuthorizationService(assignments, roles)
	var err error
	a.Auth, err = service.NewAuthService(users, persons, tokens, roles, memory.Hasher{},
		security.NewJWTIssuer("test-secret-test-secret-test-secret!!", "global360", 15*time.Minute),
		security.RandomSecrets{}, a.Authz, deps,
		service.AuthConfig{MaxFailedLoginAttempts: MaxFailedLogins, RefreshTokenTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	a.Users = service.NewUserService(users, persons, companies, countries, roles, assignments, tokens, memory.Hasher{}, deps)
	a.Companies = service.NewCompanyService(companies, countries, currencies, profiles, deps)
	a.Roles = service.NewRoleService(roles, perms, deps)
	a.Catalog = service.NewCatalogService(countries, currencies, deps)
	a.Categories = service.NewCategoryService(categories, deps)
	a.Marketplace = service.NewMarketplaceService(profiles, services, categories, countries)
	a.Distributors = service.NewDistributorProfileService(profiles, services, companies, categories, countries, deps)
	a.ContactRequests = service.NewContactRequestService(requests, profiles, services, companies, users, persons, deps)

	now := clock.Now()
	co, _ := catalog.NewCountry(ids.NewID(), "CO", "Colombia", now)
	cr, _ := catalog.NewCountry(ids.NewID(), "CR", "Costa Rica", now)
	cop, _ := catalog.NewCurrency(ids.NewID(), "COP", "Peso colombiano", "$", 2, now)
	s.Countries[co.ID], s.Countries[cr.ID], s.Currencies[cop.ID] = *co, *cr, *cop
	a.CountryCO, a.CountryCR, a.CurrencyCOP = co.ID, cr.ID, cop.ID

	for _, p := range access.DefaultPermissions {
		perm, _ := access.NewPermission(ids.NewID(), p.Code, p.Module, p.Description)
		s.Permissions[perm.ID] = *perm
	}
	for _, def := range access.DefaultRoles {
		desc := def.Description
		r, err := access.NewRole(ids.NewID(), def.Name, &desc, def.Scope, true, def.Permissions, now)
		if err != nil {
			t.Fatal(err)
		}
		s.Roles[r.ID] = *r
		a.RoleIDs[r.Name] = r.ID
	}
	for _, code := range []string{"customs_brokerage", "ground_transport", "warehousing"} {
		c, err := commerce.NewCategory(ids.NewID(), code, strings.ReplaceAll(code, "_", " "), nil, now)
		if err != nil {
			t.Fatal(err)
		}
		s.Categories[c.ID] = *c
		a.CategoryIDs[code] = c.ID
	}
	return a
}

// CreateCompany inserts an active client company directly.
func (a *App) CreateCompany(t *testing.T, doc string) string {
	t.Helper()
	return a.CreateCompanyOfType(t, doc, company.TypeClient)
}

// CreateCompanyOfType inserts an active company of the given type directly.
func (a *App) CreateCompanyOfType(t *testing.T, doc string, typ company.Type) string {
	t.Helper()
	c, err := company.New(a.IDs.NewID(), company.Data{
		LegalName: "Company " + doc, DocumentType: "NIT", DocumentNumber: doc, CountryID: a.CountryCO,
		BillingCurrencyID: a.CurrencyCOP, Type: typ,
	}, a.Clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	c.Status = company.StatusActive
	a.Store.Companies[c.ID] = *c
	return c.ID
}

// Grant is a role assignment request for CreateUser.
type Grant struct {
	Role      string
	CompanyID *string
}

// CreateUser inserts an active user with the given roles directly.
func (a *App) CreateUser(t *testing.T, email, password string, companyID *string, grants ...Grant) *identity.User {
	t.Helper()
	now := a.Clock.Now()
	p, err := identity.NewPerson(a.IDs.NewID(), identity.PersonData{
		DocumentType: "CC", DocumentNumber: email, DocumentCountryID: a.CountryCO,
		FirstNames: "Test", LastNames: "User", Email: email,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := memory.Hasher{}.Hash(password)
	u, err := identity.NewUser(a.IDs.NewID(), p.ID, companyID, email, hash, now)
	if err != nil {
		t.Fatal(err)
	}
	u.Status = identity.UserActive
	a.Store.Persons[p.ID], a.Store.Users[u.ID] = *p, *u
	for _, g := range grants {
		role := a.Store.Roles[a.RoleIDs[g.Role]]
		asg, err := access.NewRoleAssignment(a.IDs.NewID(), u.ID, &role, g.CompanyID, nil, now)
		if err != nil {
			t.Fatal(err)
		}
		a.Store.Assignments[asg.ID] = *asg
	}
	return u
}

// As returns a context authenticated as userID with its current grants.
func (a *App) As(t *testing.T, userID string) context.Context {
	t.Helper()
	u := a.Store.Users[userID]
	grants, err := a.Authz.Grants(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return actor.WithActor(context.Background(), actor.Actor{
		UserID: userID, CompanyID: u.CompanyID, Grants: grants, IP: "127.0.0.1", UserAgent: "test",
	})
}

func Ptr[T any](v T) *T { return &v }
