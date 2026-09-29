package dto

import (
	"time"

	"global_360/internal/domain/catalog"
	"global_360/internal/domain/commerce"
)

type Category struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewCategory(c *commerce.Category) Category {
	return Category{
		ID: c.ID, Code: c.Code, Name: c.Name, Description: c.Description, Active: c.Active,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// DistributorProfile is the management view of a profile (owner and Global 360).
type DistributorProfile struct {
	CompanyID          string                 `json:"company_id"`
	CountryID          string                 `json:"country_id"`
	DisplayName        string                 `json:"display_name"`
	Summary            string                 `json:"summary"`
	Description        string                 `json:"description"`
	LogoURL            *string                `json:"logo_url"`
	WebsiteURL         *string                `json:"website_url"`
	CategoryIDs        []string               `json:"category_ids"`
	CoverageCountryIDs []string               `json:"coverage_country_ids"`
	ContactEmail       *string                `json:"contact_email"`
	ContactPhone       *string                `json:"contact_phone"`
	WhatsApp           *string                `json:"whatsapp"`
	Status             commerce.ProfileStatus `json:"status"`
	StatusReason       *string                `json:"status_reason"`
	CompanyEligible    bool                   `json:"company_eligible"`
	Listed             bool                   `json:"listed"`
	SubmittedAt        *time.Time             `json:"submitted_at"`
	ReviewedAt         *time.Time             `json:"reviewed_at"`
	ReviewedBy         *string                `json:"reviewed_by"`
	PublishedAt        *time.Time             `json:"published_at"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

func NewDistributorProfile(p *commerce.DistributorProfile) DistributorProfile {
	return DistributorProfile{
		CompanyID: p.CompanyID, CountryID: p.CountryID, DisplayName: p.DisplayName, Summary: p.Summary,
		Description: p.Description, LogoURL: p.LogoURL, WebsiteURL: p.WebsiteURL,
		CategoryIDs: nonNilStrings(p.CategoryIDs), CoverageCountryIDs: nonNilStrings(p.CoverageCountryIDs),
		ContactEmail: p.ContactEmail, ContactPhone: p.ContactPhone, WhatsApp: p.WhatsApp,
		Status: p.Status, StatusReason: p.StatusReason, CompanyEligible: p.CompanyEligible, Listed: p.IsListed(),
		SubmittedAt: p.SubmittedAt, ReviewedAt: p.ReviewedAt, ReviewedBy: p.ReviewedBy, PublishedAt: p.PublishedAt,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

type DistributorService struct {
	ID                   string    `json:"id"`
	CompanyID            string    `json:"company_id"`
	CategoryID           string    `json:"category_id"`
	Name                 string    `json:"name"`
	Description          *string   `json:"description"`
	OriginCountryID      *string   `json:"origin_country_id"`
	DestinationCountryID *string   `json:"destination_country_id"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func NewDistributorService(s *commerce.DistributorService) DistributorService {
	return DistributorService{
		ID: s.ID, CompanyID: s.CompanyID, CategoryID: s.CategoryID, Name: s.Name, Description: s.Description,
		OriginCountryID: s.OriginCountryID, DestinationCountryID: s.DestinationCountryID, Active: s.Active,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

// --- marketplace (public) views ---

type CategoryRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type CountryRef struct {
	ID      string `json:"id"`
	ISOCode string `json:"iso_code"`
	Name    string `json:"name"`
}

// Lookups resolves catalog references for marketplace views.
type Lookups struct {
	Categories map[string]*commerce.Category
	Countries  map[string]*catalog.Country
}

func (l Lookups) Category(id string) *CategoryRef {
	c, ok := l.Categories[id]
	if !ok {
		return nil
	}
	return &CategoryRef{ID: c.ID, Code: c.Code, Name: c.Name}
}

func (l Lookups) Country(id *string) *CountryRef {
	if id == nil {
		return nil
	}
	c, ok := l.Countries[*id]
	if !ok {
		return nil
	}
	return &CountryRef{ID: c.ID, ISOCode: c.ISOCode, Name: c.Name}
}

// categories keeps only active categories: inactive ones are hidden from the marketplace.
func (l Lookups) categories(ids []string) []CategoryRef {
	out := make([]CategoryRef, 0, len(ids))
	for _, id := range ids {
		if c, ok := l.Categories[id]; ok && c.Active {
			out = append(out, CategoryRef{ID: c.ID, Code: c.Code, Name: c.Name})
		}
	}
	return out
}

func (l Lookups) countries(ids []string) []CountryRef {
	out := make([]CountryRef, 0, len(ids))
	for _, id := range ids {
		if ref := l.Country(&id); ref != nil {
			out = append(out, *ref)
		}
	}
	return out
}

// DistributorCard is a marketplace listing entry.
type DistributorCard struct {
	CompanyID         string        `json:"company_id"`
	DisplayName       string        `json:"display_name"`
	Summary           string        `json:"summary"`
	LogoURL           *string       `json:"logo_url"`
	Country           *CountryRef   `json:"country"`
	Categories        []CategoryRef `json:"categories"`
	CoverageCountries []CountryRef  `json:"coverage_countries"`
	PublishedAt       *time.Time    `json:"published_at"`
}

func NewDistributorCard(p *commerce.DistributorProfile, l Lookups) DistributorCard {
	return DistributorCard{
		CompanyID: p.CompanyID, DisplayName: p.DisplayName, Summary: p.Summary, LogoURL: p.LogoURL,
		Country: l.Country(&p.CountryID), Categories: l.categories(p.CategoryIDs),
		CoverageCountries: l.countries(p.CoverageCountryIDs), PublishedAt: p.PublishedAt,
	}
}

// ServiceListing is a distributor service as shown in the marketplace.
type ServiceListing struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	Category    *CategoryRef `json:"category"`
	Origin      *CountryRef  `json:"origin_country"`
	Destination *CountryRef  `json:"destination_country"`
}

// ContactInfo is only disclosed to authenticated users.
type ContactInfo struct {
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	WhatsApp *string `json:"whatsapp"`
}

// DistributorDetail is the public profile page. Contact is nil for anonymous visitors.
type DistributorDetail struct {
	DistributorCard
	Description string           `json:"description"`
	WebsiteURL  *string          `json:"website_url"`
	Services    []ServiceListing `json:"services"`
	Contact     *ContactInfo     `json:"contact"`
}

func NewDistributorDetail(p *commerce.DistributorProfile, services []*commerce.DistributorService, l Lookups, withContact bool) DistributorDetail {
	d := DistributorDetail{
		DistributorCard: NewDistributorCard(p, l),
		Description:     p.Description,
		WebsiteURL:      p.WebsiteURL,
		Services:        make([]ServiceListing, 0, len(services)),
	}
	for _, s := range services {
		d.Services = append(d.Services, ServiceListing{
			ID: s.ID, Name: s.Name, Description: s.Description, Category: l.Category(s.CategoryID),
			Origin: l.Country(s.OriginCountryID), Destination: l.Country(s.DestinationCountryID),
		})
	}
	if withContact {
		d.Contact = &ContactInfo{Email: p.ContactEmail, Phone: p.ContactPhone, WhatsApp: p.WhatsApp}
	}
	return d
}

type ContactRequest struct {
	ID                   string                        `json:"id"`
	DistributorID        string                        `json:"distributor_id"`
	DistributorName      string                        `json:"distributor_name"`
	ServiceID            *string                       `json:"service_id"`
	RequesterCompanyID   string                        `json:"requester_company_id"`
	RequesterCompanyName string                        `json:"requester_company_name"`
	RequesterUserID      string                        `json:"requester_user_id"`
	ContactName          string                        `json:"contact_name"`
	ContactEmail         string                        `json:"contact_email"`
	ContactPhone         *string                       `json:"contact_phone"`
	Message              string                        `json:"message"`
	Status               commerce.ContactRequestStatus `json:"status"`
	DistributorNotes     *string                       `json:"distributor_notes"`
	CreatedAt            time.Time                     `json:"created_at"`
	UpdatedAt            time.Time                     `json:"updated_at"`
}

// NewContactRequest builds the view. Distributor notes are internal to the distributor
// and are only included when withNotes is true.
func NewContactRequest(r *commerce.ContactRequest, withNotes bool) ContactRequest {
	out := ContactRequest{
		ID: r.ID, DistributorID: r.DistributorID, DistributorName: r.DistributorName, ServiceID: r.ServiceID,
		RequesterCompanyID: r.RequesterCompanyID, RequesterCompanyName: r.RequesterCompanyName,
		RequesterUserID: r.RequesterUserID, ContactName: r.ContactName, ContactEmail: r.ContactEmail,
		ContactPhone: r.ContactPhone, Message: r.Message, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if withNotes {
		out.DistributorNotes = r.DistributorNotes
	}
	return out
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
