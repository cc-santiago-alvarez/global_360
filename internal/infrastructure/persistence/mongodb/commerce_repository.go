package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"global_360/internal/application/port"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/shared"
)

// --- categories ---

type categoryDoc struct {
	ID          string    `bson:"_id"`
	Code        string    `bson:"code"`
	Name        string    `bson:"name"`
	Description *string   `bson:"description"`
	Active      bool      `bson:"active"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

func toCategoryDoc(c *commerce.Category) categoryDoc {
	return categoryDoc{
		ID: c.ID, Code: c.Code, Name: c.Name, Description: c.Description, Active: c.Active,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func (d *categoryDoc) toDomain() *commerce.Category {
	return &commerce.Category{
		ID: d.ID, Code: d.Code, Name: d.Name, Description: d.Description, Active: d.Active,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type CategoryRepository struct{ s *Store }

func NewCategoryRepository(s *Store) *CategoryRepository { return &CategoryRepository{s: s} }

const categoryConflict = "a category with this code already exists"

func (r *CategoryRepository) Create(ctx context.Context, c *commerce.Category) error {
	return insertOne(ctx, r.s.coll(CollCategories), toCategoryDoc(c), categoryConflict)
}

func (r *CategoryRepository) Update(ctx context.Context, c *commerce.Category) error {
	return replaceByID(ctx, r.s.coll(CollCategories), c.ID, toCategoryDoc(c), categoryConflict, "category not found")
}

func (r *CategoryRepository) FindByID(ctx context.Context, id string) (*commerce.Category, error) {
	d, err := findOne[categoryDoc](ctx, r.s.coll(CollCategories), bson.M{"_id": id}, "category not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *CategoryRepository) FindByIDs(ctx context.Context, ids []string) ([]*commerce.Category, error) {
	docs, err := findMany[categoryDoc](ctx, r.s.coll(CollCategories), bson.M{"_id": bson.M{"$in": nonNil(ids)}})
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*categoryDoc).toDomain), nil
}

func (r *CategoryRepository) List(ctx context.Context, onlyActive bool) ([]*commerce.Category, error) {
	filter := bson.M{}
	if onlyActive {
		filter["active"] = true
	}
	docs, err := findMany[categoryDoc](ctx, r.s.coll(CollCategories), filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*categoryDoc).toDomain), nil
}

// --- distributor profiles ---

type distributorProfileDoc struct {
	CompanyID          string     `bson:"_id"`
	CountryID          string     `bson:"country_id"`
	DisplayName        string     `bson:"display_name"`
	Summary            string     `bson:"summary"`
	Description        string     `bson:"description"`
	LogoURL            *string    `bson:"logo_url"`
	WebsiteURL         *string    `bson:"website_url"`
	CategoryIDs        []string   `bson:"category_ids"`
	CoverageCountryIDs []string   `bson:"coverage_country_ids"`
	ContactEmail       *string    `bson:"contact_email"`
	ContactPhone       *string    `bson:"contact_phone"`
	WhatsApp           *string    `bson:"whatsapp"`
	CompanyEligible    bool       `bson:"company_eligible"`
	Status             string     `bson:"status"`
	StatusReason       *string    `bson:"status_reason"`
	SubmittedAt        *time.Time `bson:"submitted_at"`
	ReviewedAt         *time.Time `bson:"reviewed_at"`
	ReviewedBy         *string    `bson:"reviewed_by"`
	PublishedAt        *time.Time `bson:"published_at"`
	CreatedAt          time.Time  `bson:"created_at"`
	UpdatedAt          time.Time  `bson:"updated_at"`
}

func toProfileDoc(p *commerce.DistributorProfile) distributorProfileDoc {
	return distributorProfileDoc{
		CompanyID: p.CompanyID, CountryID: p.CountryID, DisplayName: p.DisplayName, Summary: p.Summary,
		Description: p.Description, LogoURL: p.LogoURL, WebsiteURL: p.WebsiteURL,
		CategoryIDs: nonNil(p.CategoryIDs), CoverageCountryIDs: nonNil(p.CoverageCountryIDs),
		ContactEmail: p.ContactEmail, ContactPhone: p.ContactPhone, WhatsApp: p.WhatsApp,
		CompanyEligible: p.CompanyEligible, Status: string(p.Status), StatusReason: p.StatusReason,
		SubmittedAt: p.SubmittedAt, ReviewedAt: p.ReviewedAt, ReviewedBy: p.ReviewedBy, PublishedAt: p.PublishedAt,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (d *distributorProfileDoc) toDomain() *commerce.DistributorProfile {
	return &commerce.DistributorProfile{
		CompanyID: d.CompanyID, CountryID: d.CountryID, DisplayName: d.DisplayName, Summary: d.Summary,
		Description: d.Description, LogoURL: d.LogoURL, WebsiteURL: d.WebsiteURL,
		CategoryIDs: d.CategoryIDs, CoverageCountryIDs: d.CoverageCountryIDs,
		ContactEmail: d.ContactEmail, ContactPhone: d.ContactPhone, WhatsApp: d.WhatsApp,
		CompanyEligible: d.CompanyEligible, Status: commerce.ProfileStatus(d.Status), StatusReason: d.StatusReason,
		SubmittedAt: utcPtr(d.SubmittedAt), ReviewedAt: utcPtr(d.ReviewedAt), ReviewedBy: d.ReviewedBy,
		PublishedAt: utcPtr(d.PublishedAt), CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type DistributorProfileRepository struct{ s *Store }

func NewDistributorProfileRepository(s *Store) *DistributorProfileRepository {
	return &DistributorProfileRepository{s: s}
}

const (
	profileConflict = "the company already has a distributor profile"
	profileNotFound = "distributor profile not found"
)

func (r *DistributorProfileRepository) Create(ctx context.Context, p *commerce.DistributorProfile) error {
	return insertOne(ctx, r.s.coll(CollDistributorProfiles), toProfileDoc(p), profileConflict)
}

func (r *DistributorProfileRepository) Update(ctx context.Context, p *commerce.DistributorProfile) error {
	return replaceByID(ctx, r.s.coll(CollDistributorProfiles), p.CompanyID, toProfileDoc(p), profileConflict, profileNotFound)
}

func (r *DistributorProfileRepository) FindByCompanyID(ctx context.Context, companyID string) (*commerce.DistributorProfile, error) {
	d, err := findOne[distributorProfileDoc](ctx, r.s.coll(CollDistributorProfiles), bson.M{"_id": companyID}, profileNotFound)
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *DistributorProfileRepository) ListListed(ctx context.Context, f port.MarketplaceFilter) ([]*commerce.DistributorProfile, int64, error) {
	filter := bson.M{"status": string(commerce.ProfilePublished), "company_eligible": true}
	if f.CategoryID != nil {
		filter["category_ids"] = *f.CategoryID
	}
	if f.CountryID != nil {
		filter["coverage_country_ids"] = *f.CountryID
	}
	sort := bson.D{{Key: "published_at", Value: -1}, {Key: "_id", Value: 1}}
	if f.Query != nil {
		filter["$text"] = bson.M{"$search": *f.Query}
		sort = bson.D{{Key: "score", Value: bson.M{"$meta": "textScore"}}, {Key: "_id", Value: 1}}
	}
	docs, total, err := findPage[distributorProfileDoc](ctx, r.s.coll(CollDistributorProfiles), filter, sort, f.Page)
	if err != nil {
		return nil, 0, err
	}
	return mapAll(docs, (*distributorProfileDoc).toDomain), total, nil
}

func (r *DistributorProfileRepository) ListByStatus(ctx context.Context, status *commerce.ProfileStatus, page shared.Pagination) ([]*commerce.DistributorProfile, int64, error) {
	filter := bson.M{}
	if status != nil {
		filter["status"] = string(*status)
	}
	// Oldest submissions first: it is a review queue.
	sort := bson.D{{Key: "submitted_at", Value: 1}, {Key: "created_at", Value: 1}}
	docs, total, err := findPage[distributorProfileDoc](ctx, r.s.coll(CollDistributorProfiles), filter, sort, page)
	if err != nil {
		return nil, 0, err
	}
	return mapAll(docs, (*distributorProfileDoc).toDomain), total, nil
}

// --- distributor services ---

type distributorServiceDoc struct {
	ID                   string    `bson:"_id"`
	CompanyID            string    `bson:"company_id"`
	CategoryID           string    `bson:"category_id"`
	Name                 string    `bson:"name"`
	Description          *string   `bson:"description"`
	OriginCountryID      *string   `bson:"origin_country_id"`
	DestinationCountryID *string   `bson:"destination_country_id"`
	Active               bool      `bson:"active"`
	CreatedAt            time.Time `bson:"created_at"`
	UpdatedAt            time.Time `bson:"updated_at"`
}

func toServiceDoc(s *commerce.DistributorService) distributorServiceDoc {
	return distributorServiceDoc{
		ID: s.ID, CompanyID: s.CompanyID, CategoryID: s.CategoryID, Name: s.Name, Description: s.Description,
		OriginCountryID: s.OriginCountryID, DestinationCountryID: s.DestinationCountryID, Active: s.Active,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

func (d *distributorServiceDoc) toDomain() *commerce.DistributorService {
	return &commerce.DistributorService{
		ID: d.ID, CompanyID: d.CompanyID, CategoryID: d.CategoryID, Name: d.Name, Description: d.Description,
		OriginCountryID: d.OriginCountryID, DestinationCountryID: d.DestinationCountryID, Active: d.Active,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type DistributorServiceRepository struct{ s *Store }

func NewDistributorServiceRepository(s *Store) *DistributorServiceRepository {
	return &DistributorServiceRepository{s: s}
}

const serviceConflict = "the distributor already has a service with this name"

func (r *DistributorServiceRepository) Create(ctx context.Context, s *commerce.DistributorService) error {
	return insertOne(ctx, r.s.coll(CollDistributorServices), toServiceDoc(s), serviceConflict)
}

func (r *DistributorServiceRepository) Update(ctx context.Context, s *commerce.DistributorService) error {
	return replaceByID(ctx, r.s.coll(CollDistributorServices), s.ID, toServiceDoc(s), serviceConflict, "service not found")
}

func (r *DistributorServiceRepository) FindByID(ctx context.Context, id string) (*commerce.DistributorService, error) {
	d, err := findOne[distributorServiceDoc](ctx, r.s.coll(CollDistributorServices), bson.M{"_id": id}, "service not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *DistributorServiceRepository) ListByCompany(ctx context.Context, companyID string, onlyActive bool) ([]*commerce.DistributorService, error) {
	filter := bson.M{"company_id": companyID}
	if onlyActive {
		filter["active"] = true
	}
	docs, err := findMany[distributorServiceDoc](ctx, r.s.coll(CollDistributorServices), filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*distributorServiceDoc).toDomain), nil
}

// --- contact requests ---

type contactRequestDoc struct {
	ID                   string    `bson:"_id"`
	DistributorID        string    `bson:"distributor_id"`
	DistributorName      string    `bson:"distributor_name"`
	ServiceID            *string   `bson:"service_id"`
	RequesterCompanyID   string    `bson:"requester_company_id"`
	RequesterCompanyName string    `bson:"requester_company_name"`
	RequesterUserID      string    `bson:"requester_user_id"`
	ContactName          string    `bson:"contact_name"`
	ContactEmail         string    `bson:"contact_email"`
	ContactPhone         *string   `bson:"contact_phone"`
	Message              string    `bson:"message"`
	Status               string    `bson:"status"`
	DistributorNotes     *string   `bson:"distributor_notes"`
	CreatedAt            time.Time `bson:"created_at"`
	UpdatedAt            time.Time `bson:"updated_at"`
}

func toContactRequestDoc(r *commerce.ContactRequest) contactRequestDoc {
	return contactRequestDoc{
		ID: r.ID, DistributorID: r.DistributorID, DistributorName: r.DistributorName, ServiceID: r.ServiceID,
		RequesterCompanyID: r.RequesterCompanyID, RequesterCompanyName: r.RequesterCompanyName,
		RequesterUserID: r.RequesterUserID, ContactName: r.ContactName, ContactEmail: r.ContactEmail,
		ContactPhone: r.ContactPhone, Message: r.Message, Status: string(r.Status), DistributorNotes: r.DistributorNotes,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (d *contactRequestDoc) toDomain() *commerce.ContactRequest {
	return &commerce.ContactRequest{
		ID: d.ID, DistributorID: d.DistributorID, DistributorName: d.DistributorName, ServiceID: d.ServiceID,
		RequesterCompanyID: d.RequesterCompanyID, RequesterCompanyName: d.RequesterCompanyName,
		RequesterUserID: d.RequesterUserID, ContactName: d.ContactName, ContactEmail: d.ContactEmail,
		ContactPhone: d.ContactPhone, Message: d.Message, Status: commerce.ContactRequestStatus(d.Status),
		DistributorNotes: d.DistributorNotes, CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type ContactRequestRepository struct{ s *Store }

func NewContactRequestRepository(s *Store) *ContactRequestRepository {
	return &ContactRequestRepository{s: s}
}

const contactRequestConflict = "you already have an open contact request with this distributor"

func (r *ContactRequestRepository) Create(ctx context.Context, c *commerce.ContactRequest) error {
	return insertOne(ctx, r.s.coll(CollContactRequests), toContactRequestDoc(c), contactRequestConflict)
}

func (r *ContactRequestRepository) Update(ctx context.Context, c *commerce.ContactRequest) error {
	return replaceByID(ctx, r.s.coll(CollContactRequests), c.ID, toContactRequestDoc(c), contactRequestConflict, "contact request not found")
}

func (r *ContactRequestRepository) FindByID(ctx context.Context, id string) (*commerce.ContactRequest, error) {
	d, err := findOne[contactRequestDoc](ctx, r.s.coll(CollContactRequests), bson.M{"_id": id}, "contact request not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *ContactRequestRepository) List(ctx context.Context, f port.ContactRequestFilter) ([]*commerce.ContactRequest, int64, error) {
	filter := bson.M{}
	if f.DistributorID != nil {
		filter["distributor_id"] = *f.DistributorID
	}
	if f.RestrictToRequesters {
		filter["requester_company_id"] = bson.M{"$in": nonNil(f.RequesterCompanyIDs)}
	}
	if f.Status != nil {
		filter["status"] = string(*f.Status)
	}
	docs, total, err := findPage[contactRequestDoc](ctx, r.s.coll(CollContactRequests), filter, bson.D{{Key: "created_at", Value: -1}}, f.Page)
	if err != nil {
		return nil, 0, err
	}
	return mapAll(docs, (*contactRequestDoc).toDomain), total, nil
}

var (
	_ port.CategoryRepository           = (*CategoryRepository)(nil)
	_ port.DistributorProfileRepository = (*DistributorProfileRepository)(nil)
	_ port.DistributorServiceRepository = (*DistributorServiceRepository)(nil)
	_ port.ContactRequestRepository     = (*ContactRequestRepository)(nil)
)
