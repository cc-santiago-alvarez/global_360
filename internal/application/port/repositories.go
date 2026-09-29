// Package port declares the interfaces the application layer needs from the
// outside world (driven ports). Infrastructure adapters implement them.
//
// Repository conventions:
//   - Find* return an error wrapping shared.ErrNotFound when nothing matches.
//   - Create/Update return an error wrapping shared.ErrConflict on unique violations.
//   - There are no delete methods: records are never physically removed.
package port

import (
	"context"
	"time"

	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/catalog"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

type CountryRepository interface {
	Create(ctx context.Context, c *catalog.Country) error
	Update(ctx context.Context, c *catalog.Country) error
	FindByID(ctx context.Context, id string) (*catalog.Country, error)
	List(ctx context.Context, onlyActive bool) ([]*catalog.Country, error)
}

type CurrencyRepository interface {
	Create(ctx context.Context, c *catalog.Currency) error
	Update(ctx context.Context, c *catalog.Currency) error
	FindByID(ctx context.Context, id string) (*catalog.Currency, error)
	List(ctx context.Context, onlyActive bool) ([]*catalog.Currency, error)
}

type PersonRepository interface {
	Create(ctx context.Context, p *identity.Person) error
	Update(ctx context.Context, p *identity.Person) error
	FindByID(ctx context.Context, id string) (*identity.Person, error)
	FindByDocument(ctx context.Context, documentType, documentNumber, countryID string) (*identity.Person, error)
}

// CompanyFilter restricts company listings. When RestrictToIDs is true only IDs are returned.
type CompanyFilter struct {
	RestrictToIDs bool
	IDs           []string
	Status        *company.Status
	Type          *company.Type
	Page          shared.Pagination
}

type CompanyRepository interface {
	Create(ctx context.Context, c *company.Company) error
	Update(ctx context.Context, c *company.Company) error
	FindByID(ctx context.Context, id string) (*company.Company, error)
	List(ctx context.Context, f CompanyFilter) ([]*company.Company, int64, error)
}

// UserFilter restricts user listings. When RestrictToCompanies is true only users of CompanyIDs are returned.
type UserFilter struct {
	RestrictToCompanies bool
	CompanyIDs          []string
	Status              *identity.UserStatus
	Page                shared.Pagination
}

type UserRepository interface {
	Create(ctx context.Context, u *identity.User) error
	Update(ctx context.Context, u *identity.User) error
	FindByID(ctx context.Context, id string) (*identity.User, error)
	FindByEmail(ctx context.Context, email string) (*identity.User, error)
	List(ctx context.Context, f UserFilter) ([]*identity.User, int64, error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, t *identity.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*identity.RefreshToken, error)
	// MarkRevoked revokes the token only if it is not revoked yet. It reports
	// whether this call revoked it, which makes rotation safe under concurrency.
	MarkRevoked(ctx context.Context, id string, at time.Time) (bool, error)
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
}

type PermissionRepository interface {
	List(ctx context.Context) ([]*access.Permission, error)
	FindByCodes(ctx context.Context, codes []string) ([]*access.Permission, error)
}

type RoleRepository interface {
	Create(ctx context.Context, r *access.Role) error
	Update(ctx context.Context, r *access.Role) error
	FindByID(ctx context.Context, id string) (*access.Role, error)
	FindByIDs(ctx context.Context, ids []string) ([]*access.Role, error)
	List(ctx context.Context) ([]*access.Role, error)
}

type RoleAssignmentRepository interface {
	Create(ctx context.Context, a *access.RoleAssignment) error
	Update(ctx context.Context, a *access.RoleAssignment) error
	FindByID(ctx context.Context, id string) (*access.RoleAssignment, error)
	ListActiveByUser(ctx context.Context, userID string) ([]*access.RoleAssignment, error)
}

type AuditFilter struct {
	Entity   *string
	EntityID *string
	UserID   *string
	Action   *audit.Action
	From     *time.Time
	To       *time.Time
	Page     shared.Pagination
}

type AuditLogRepository interface {
	Create(ctx context.Context, l *audit.Log) error
	List(ctx context.Context, f AuditFilter) ([]*audit.Log, int64, error)
}

// --- phase 2: marketplace ("commerce") ---

type CategoryRepository interface {
	Create(ctx context.Context, c *commerce.Category) error
	Update(ctx context.Context, c *commerce.Category) error
	FindByID(ctx context.Context, id string) (*commerce.Category, error)
	FindByIDs(ctx context.Context, ids []string) ([]*commerce.Category, error)
	List(ctx context.Context, onlyActive bool) ([]*commerce.Category, error)
}

// MarketplaceFilter searches the profiles visible in the marketplace.
// Query is a full text search; results are sorted by relevance when it is set.
type MarketplaceFilter struct {
	Query      *string
	CategoryID *string
	CountryID  *string // coverage country
	Page       shared.Pagination
}

type DistributorProfileRepository interface {
	Create(ctx context.Context, p *commerce.DistributorProfile) error
	Update(ctx context.Context, p *commerce.DistributorProfile) error
	FindByCompanyID(ctx context.Context, companyID string) (*commerce.DistributorProfile, error)
	// ListListed returns only published profiles of eligible companies.
	ListListed(ctx context.Context, f MarketplaceFilter) ([]*commerce.DistributorProfile, int64, error)
	ListByStatus(ctx context.Context, status *commerce.ProfileStatus, page shared.Pagination) ([]*commerce.DistributorProfile, int64, error)
}

type DistributorServiceRepository interface {
	Create(ctx context.Context, s *commerce.DistributorService) error
	Update(ctx context.Context, s *commerce.DistributorService) error
	FindByID(ctx context.Context, id string) (*commerce.DistributorService, error)
	ListByCompany(ctx context.Context, companyID string, onlyActive bool) ([]*commerce.DistributorService, error)
}

// ContactRequestFilter restricts lead listings. When RestrictToRequesters is true only
// requests sent by RequesterCompanyIDs are returned.
type ContactRequestFilter struct {
	DistributorID        *string
	RestrictToRequesters bool
	RequesterCompanyIDs  []string
	Status               *commerce.ContactRequestStatus
	Page                 shared.Pagination
}

type ContactRequestRepository interface {
	// Create returns a conflict when the requester already has a "new" request for the distributor.
	Create(ctx context.Context, r *commerce.ContactRequest) error
	Update(ctx context.Context, r *commerce.ContactRequest) error
	FindByID(ctx context.Context, id string) (*commerce.ContactRequest, error)
	List(ctx context.Context, f ContactRequestFilter) ([]*commerce.ContactRequest, int64, error)
}
