// Package memory provides in-memory implementations of the application ports for tests.
package memory

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/catalog"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

// Store holds every collection. Entities are copied in and out to mimic a database.
type Store struct {
	mu          sync.Mutex
	Countries   map[string]catalog.Country
	Currencies  map[string]catalog.Currency
	Persons     map[string]identity.Person
	Users       map[string]identity.User
	Tokens      map[string]identity.RefreshToken
	Companies   map[string]company.Company
	Permissions map[string]access.Permission
	Roles       map[string]access.Role
	Assignments map[string]access.RoleAssignment
	AuditLogs   []audit.Log

	Categories      map[string]commerce.Category
	Profiles        map[string]commerce.DistributorProfile
	Services        map[string]commerce.DistributorService
	ContactRequests map[string]commerce.ContactRequest
}

func NewStore() *Store {
	return &Store{
		Countries: map[string]catalog.Country{}, Currencies: map[string]catalog.Currency{},
		Persons: map[string]identity.Person{}, Users: map[string]identity.User{}, Tokens: map[string]identity.RefreshToken{},
		Companies: map[string]company.Company{}, Permissions: map[string]access.Permission{},
		Roles: map[string]access.Role{}, Assignments: map[string]access.RoleAssignment{},
		Categories: map[string]commerce.Category{}, Profiles: map[string]commerce.DistributorProfile{},
		Services: map[string]commerce.DistributorService{}, ContactRequests: map[string]commerce.ContactRequest{},
	}
}

// AuditEntries returns a copy of the audit trail.
func (s *Store) AuditEntries() []audit.Log {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]audit.Log(nil), s.AuditLogs...)
}

func page[T any](items []T, p shared.Pagination) ([]T, int64) {
	total := int64(len(items))
	start := int(p.Offset())
	if start > len(items) {
		start = len(items)
	}
	end := start + p.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total
}

// --- countries / currencies ---

type CountryRepo struct{ S *Store }

func (r CountryRepo) Create(_ context.Context, c *catalog.Country) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Countries {
		if x.ISOCode == c.ISOCode {
			return shared.Conflict("a country with this iso_code already exists")
		}
	}
	r.S.Countries[c.ID] = *c
	return nil
}

func (r CountryRepo) Update(_ context.Context, c *catalog.Country) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	if _, ok := r.S.Countries[c.ID]; !ok {
		return shared.NotFound("country not found")
	}
	r.S.Countries[c.ID] = *c
	return nil
}

func (r CountryRepo) FindByID(_ context.Context, id string) (*catalog.Country, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	c, ok := r.S.Countries[id]
	if !ok {
		return nil, shared.NotFound("country not found")
	}
	return &c, nil
}

func (r CountryRepo) List(_ context.Context, onlyActive bool) ([]*catalog.Country, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*catalog.Country
	for _, c := range r.S.Countries {
		if !onlyActive || c.Active {
			c := c
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type CurrencyRepo struct{ S *Store }

func (r CurrencyRepo) Create(_ context.Context, c *catalog.Currency) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Currencies[c.ID] = *c
	return nil
}

func (r CurrencyRepo) Update(_ context.Context, c *catalog.Currency) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Currencies[c.ID] = *c
	return nil
}

func (r CurrencyRepo) FindByID(_ context.Context, id string) (*catalog.Currency, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	c, ok := r.S.Currencies[id]
	if !ok {
		return nil, shared.NotFound("currency not found")
	}
	return &c, nil
}

func (r CurrencyRepo) List(_ context.Context, onlyActive bool) ([]*catalog.Currency, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*catalog.Currency
	for _, c := range r.S.Currencies {
		if !onlyActive || c.Active {
			c := c
			out = append(out, &c)
		}
	}
	return out, nil
}

// --- persons / users / tokens ---

type PersonRepo struct{ S *Store }

func (r PersonRepo) Create(_ context.Context, p *identity.Person) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Persons {
		if x.DocumentType == p.DocumentType && x.DocumentNumber == p.DocumentNumber && x.DocumentCountryID == p.DocumentCountryID {
			return shared.Conflict("a person with this document already exists")
		}
	}
	r.S.Persons[p.ID] = *p
	return nil
}

func (r PersonRepo) Update(_ context.Context, p *identity.Person) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Persons[p.ID] = *p
	return nil
}

func (r PersonRepo) FindByID(_ context.Context, id string) (*identity.Person, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	p, ok := r.S.Persons[id]
	if !ok {
		return nil, shared.NotFound("person not found")
	}
	return &p, nil
}

func (r PersonRepo) FindByDocument(_ context.Context, docType, docNumber, countryID string) (*identity.Person, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, p := range r.S.Persons {
		if p.DocumentType == docType && p.DocumentNumber == docNumber && p.DocumentCountryID == countryID {
			p := p
			return &p, nil
		}
	}
	return nil, shared.NotFound("person not found")
}

type UserRepo struct{ S *Store }

func (r UserRepo) Create(_ context.Context, u *identity.User) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Users {
		if x.Email == u.Email {
			return shared.Conflict("a user with this email already exists")
		}
	}
	r.S.Users[u.ID] = *u
	return nil
}

func (r UserRepo) Update(_ context.Context, u *identity.User) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Users {
		if x.Email == u.Email && x.ID != u.ID {
			return shared.Conflict("a user with this email already exists")
		}
	}
	r.S.Users[u.ID] = *u
	return nil
}

func (r UserRepo) FindByID(_ context.Context, id string) (*identity.User, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	u, ok := r.S.Users[id]
	if !ok {
		return nil, shared.NotFound("user not found")
	}
	return &u, nil
}

func (r UserRepo) FindByEmail(_ context.Context, email string) (*identity.User, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, u := range r.S.Users {
		if u.Email == email {
			u := u
			return &u, nil
		}
	}
	return nil, shared.NotFound("user not found")
}

func (r UserRepo) List(_ context.Context, f port.UserFilter) ([]*identity.User, int64, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*identity.User
	for _, u := range r.S.Users {
		if f.RestrictToCompanies && (u.CompanyID == nil || !contains(f.CompanyIDs, *u.CompanyID)) {
			continue
		}
		if f.Status != nil && u.Status != *f.Status {
			continue
		}
		u := u
		out = append(out, &u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	items, total := page(out, f.Page)
	return items, total, nil
}

type TokenRepo struct{ S *Store }

func (r TokenRepo) Create(_ context.Context, t *identity.RefreshToken) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Tokens[t.ID] = *t
	return nil
}

func (r TokenRepo) FindByHash(_ context.Context, hash string) (*identity.RefreshToken, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, t := range r.S.Tokens {
		if t.TokenHash == hash {
			t := t
			return &t, nil
		}
	}
	return nil, shared.NotFound("refresh token not found")
}

func (r TokenRepo) MarkRevoked(_ context.Context, id string, at time.Time) (bool, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	t, ok := r.S.Tokens[id]
	if !ok || t.RevokedAt != nil {
		return false, nil
	}
	t.RevokedAt = &at
	r.S.Tokens[id] = t
	return true, nil
}

func (r TokenRepo) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for id, t := range r.S.Tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &at
			r.S.Tokens[id] = t
		}
	}
	return nil
}

// --- companies ---

type CompanyRepo struct{ S *Store }

func (r CompanyRepo) Create(_ context.Context, c *company.Company) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Companies {
		if x.DocumentType == c.DocumentType && x.DocumentNumber == c.DocumentNumber && x.CountryID == c.CountryID {
			return shared.Conflict("a company with this document already exists in this country")
		}
	}
	r.S.Companies[c.ID] = *c
	return nil
}

func (r CompanyRepo) Update(_ context.Context, c *company.Company) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Companies[c.ID] = *c
	return nil
}

func (r CompanyRepo) FindByID(_ context.Context, id string) (*company.Company, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	c, ok := r.S.Companies[id]
	if !ok {
		return nil, shared.NotFound("company not found")
	}
	return &c, nil
}

func (r CompanyRepo) List(_ context.Context, f port.CompanyFilter) ([]*company.Company, int64, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*company.Company
	for _, c := range r.S.Companies {
		if f.RestrictToIDs && !contains(f.IDs, c.ID) {
			continue
		}
		if f.Status != nil && c.Status != *f.Status {
			continue
		}
		if f.Type != nil && c.Type != *f.Type {
			continue
		}
		c := c
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LegalName < out[j].LegalName })
	items, total := page(out, f.Page)
	return items, total, nil
}

// --- access ---

type PermissionRepo struct{ S *Store }

func (r PermissionRepo) List(_ context.Context) ([]*access.Permission, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*access.Permission
	for _, p := range r.S.Permissions {
		p := p
		out = append(out, &p)
	}
	return out, nil
}

func (r PermissionRepo) FindByCodes(_ context.Context, codes []string) ([]*access.Permission, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*access.Permission
	for _, p := range r.S.Permissions {
		if contains(codes, p.Code) {
			p := p
			out = append(out, &p)
		}
	}
	return out, nil
}

type RoleRepo struct{ S *Store }

func (r RoleRepo) Create(_ context.Context, role *access.Role) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Roles {
		if x.Name == role.Name {
			return shared.Conflict("a role with this name already exists")
		}
	}
	r.S.Roles[role.ID] = cloneRole(*role)
	return nil
}

func (r RoleRepo) Update(_ context.Context, role *access.Role) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Roles[role.ID] = cloneRole(*role)
	return nil
}

func (r RoleRepo) FindByID(_ context.Context, id string) (*access.Role, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	role, ok := r.S.Roles[id]
	if !ok {
		return nil, shared.NotFound("role not found")
	}
	c := cloneRole(role)
	return &c, nil
}

func (r RoleRepo) FindByIDs(_ context.Context, ids []string) ([]*access.Role, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*access.Role
	for _, id := range ids {
		if role, ok := r.S.Roles[id]; ok {
			c := cloneRole(role)
			out = append(out, &c)
		}
	}
	return out, nil
}

func (r RoleRepo) List(_ context.Context) ([]*access.Role, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*access.Role
	for _, role := range r.S.Roles {
		c := cloneRole(role)
		out = append(out, &c)
	}
	return out, nil
}

func cloneRole(r access.Role) access.Role {
	r.Permissions = append([]string(nil), r.Permissions...)
	return r
}

type AssignmentRepo struct{ S *Store }

func (r AssignmentRepo) Create(_ context.Context, a *access.RoleAssignment) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Assignments {
		if x.Active && x.UserID == a.UserID && x.RoleID == a.RoleID && eqPtr(x.CompanyID, a.CompanyID) {
			return shared.Conflict("the user already has this role in this scope")
		}
	}
	r.S.Assignments[a.ID] = *a
	return nil
}

func (r AssignmentRepo) Update(_ context.Context, a *access.RoleAssignment) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	r.S.Assignments[a.ID] = *a
	return nil
}

func (r AssignmentRepo) FindByID(_ context.Context, id string) (*access.RoleAssignment, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	a, ok := r.S.Assignments[id]
	if !ok {
		return nil, shared.NotFound("role assignment not found")
	}
	return &a, nil
}

func (r AssignmentRepo) ListActiveByUser(_ context.Context, userID string) ([]*access.RoleAssignment, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*access.RoleAssignment
	for _, a := range r.S.Assignments {
		if a.UserID == userID && a.Active {
			a := a
			out = append(out, &a)
		}
	}
	return out, nil
}

// --- audit ---

type AuditRepo struct{ S *Store }

func (r AuditRepo) Create(_ context.Context, l *audit.Log) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	l.ID = strconv.Itoa(len(r.S.AuditLogs) + 1)
	r.S.AuditLogs = append(r.S.AuditLogs, *l)
	return nil
}

func (r AuditRepo) List(_ context.Context, f port.AuditFilter) ([]*audit.Log, int64, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*audit.Log
	for i := len(r.S.AuditLogs) - 1; i >= 0; i-- {
		l := r.S.AuditLogs[i]
		if (f.Entity != nil && l.Entity != *f.Entity) || (f.EntityID != nil && l.EntityID != *f.EntityID) ||
			(f.UserID != nil && !eqPtr(l.UserID, f.UserID)) || (f.Action != nil && l.Action != *f.Action) {
			continue
		}
		out = append(out, &l)
	}
	items, total := page(out, f.Page)
	return items, total, nil
}

// --- non repository ports ---

// Tx runs the function directly (no rollback in memory).
type Tx struct{}

func (Tx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// Clock returns a fixed, manually advanced time.
type Clock struct{ T time.Time }

func (c *Clock) Now() time.Time          { return c.T }
func (c *Clock) Advance(d time.Duration) { c.T = c.T.Add(d) }

// IDs returns sequential ids.
type IDs struct {
	mu sync.Mutex
	n  int
}

func (g *IDs) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return "id-" + strconv.Itoa(g.n)
}

// Hasher uses bcrypt at minimum cost to keep tests fast.
type Hasher struct{}

func (Hasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	return string(b), err
}

func (Hasher) Compare(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
