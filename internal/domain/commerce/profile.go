package commerce

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"global_360/internal/domain/company"
	"global_360/internal/domain/shared"
)

// ProfileStatus is the publication state of a distributor profile.
type ProfileStatus string

const (
	ProfileDraft         ProfileStatus = "draft"
	ProfilePendingReview ProfileStatus = "pending_review"
	ProfilePublished     ProfileStatus = "published"
	ProfileRejected      ProfileStatus = "rejected"
	ProfileSuspended     ProfileStatus = "suspended"
)

var ProfileStatuses = []ProfileStatus{ProfileDraft, ProfilePendingReview, ProfilePublished, ProfileRejected, ProfileSuspended}

func (s ProfileStatus) Valid() bool {
	for _, v := range ProfileStatuses {
		if s == v {
			return true
		}
	}
	return false
}

const (
	MaxCategories        = 20
	MaxCoverageCountries = 50
)

var phoneRe = regexp.MustCompile(`^\+?[0-9][0-9 ()-]{5,24}$`)

// CanBeDistributor reports whether a company type may publish a distributor profile.
func CanBeDistributor(t company.Type) bool {
	return t == company.TypeProvider || t == company.TypeBoth
}

// CompanyEligible reports whether the company may appear in the marketplace.
func CompanyEligible(c *company.Company) bool {
	return c.Status == company.StatusActive && CanBeDistributor(c.Type)
}

// DistributorProfile is the public face of a provider company in the marketplace.
// It extends the company: its identifier is the company ID.
type DistributorProfile struct {
	CompanyID          string
	CountryID          string // headquarters, copied from the (immutable) company country
	DisplayName        string
	Summary            string
	Description        string
	LogoURL            *string
	WebsiteURL         *string
	CategoryIDs        []string
	CoverageCountryIDs []string
	ContactEmail       *string
	ContactPhone       *string
	WhatsApp           *string
	CompanyEligible    bool
	Status             ProfileStatus
	StatusReason       *string // why it was rejected or suspended
	SubmittedAt        *time.Time
	ReviewedAt         *time.Time
	ReviewedBy         *string
	PublishedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ProfileContent is the editable content of a profile. Updates replace it entirely.
type ProfileContent struct {
	DisplayName        string
	Summary            string
	Description        string
	LogoURL            *string
	WebsiteURL         *string
	CategoryIDs        []string
	CoverageCountryIDs []string
	ContactEmail       *string
	ContactPhone       *string
	WhatsApp           *string
}

// NewProfile creates a draft profile for a provider company.
func NewProfile(c *company.Company, content ProfileContent, now time.Time) (*DistributorProfile, error) {
	if !CanBeDistributor(c.Type) {
		return nil, shared.Validation("only provider companies can have a distributor profile")
	}
	p := &DistributorProfile{
		CompanyID:       c.ID,
		CountryID:       c.CountryID,
		CompanyEligible: CompanyEligible(c),
		Status:          ProfileDraft,
		CreatedAt:       now,
	}
	if err := p.apply(content, now); err != nil {
		return nil, err
	}
	return p, nil
}

// Editable reports whether the content can change in the current status.
func (p *DistributorProfile) Editable() bool {
	return p.Status == ProfileDraft || p.Status == ProfileRejected || p.Status == ProfilePublished
}

// Update replaces the content. Published profiles stay published; edits are audited.
func (p *DistributorProfile) Update(content ProfileContent, now time.Time) error {
	if !p.Editable() {
		return shared.Validation("profile cannot be edited while %s", p.Status)
	}
	return p.apply(content, now)
}

func (p *DistributorProfile) apply(c ProfileContent, now time.Time) error {
	name, err := shared.RequireText("display_name", c.DisplayName, 150)
	if err != nil {
		return err
	}
	summary, err := optionalString("summary", c.Summary, 280)
	if err != nil {
		return err
	}
	description, err := optionalString("description", c.Description, 5000)
	if err != nil {
		return err
	}
	logo, err := optionalURL("logo_url", c.LogoURL)
	if err != nil {
		return err
	}
	website, err := optionalURL("website_url", c.WebsiteURL)
	if err != nil {
		return err
	}
	categories, err := NormalizeIDs("category_ids", c.CategoryIDs, MaxCategories)
	if err != nil {
		return err
	}
	countries, err := NormalizeIDs("coverage_country_ids", c.CoverageCountryIDs, MaxCoverageCountries)
	if err != nil {
		return err
	}
	email, err := shared.OptionalEmail("contact_email", c.ContactEmail)
	if err != nil {
		return err
	}
	phone, err := optionalPhone("contact_phone", c.ContactPhone)
	if err != nil {
		return err
	}
	whatsapp, err := optionalPhone("whatsapp", c.WhatsApp)
	if err != nil {
		return err
	}
	p.DisplayName, p.Summary, p.Description = name, summary, description
	p.LogoURL, p.WebsiteURL = logo, website
	p.CategoryIDs, p.CoverageCountryIDs = categories, countries
	p.ContactEmail, p.ContactPhone, p.WhatsApp = email, phone, whatsapp
	p.UpdatedAt = now
	return nil
}

// missingForReview lists what must be completed before submitting.
func (p *DistributorProfile) missingForReview() []string {
	var missing []string
	if p.Summary == "" {
		missing = append(missing, "summary")
	}
	if p.Description == "" {
		missing = append(missing, "description")
	}
	if len(p.CategoryIDs) == 0 {
		missing = append(missing, "category_ids")
	}
	if len(p.CoverageCountryIDs) == 0 {
		missing = append(missing, "coverage_country_ids")
	}
	if p.ContactEmail == nil && p.ContactPhone == nil && p.WhatsApp == nil {
		missing = append(missing, "a contact channel (contact_email, contact_phone or whatsapp)")
	}
	return missing
}

// Submit sends the profile to Global 360 for review.
func (p *DistributorProfile) Submit(now time.Time) error {
	if p.Status != ProfileDraft && p.Status != ProfileRejected {
		return shared.Validation("cannot submit a profile that is %s", p.Status)
	}
	if missing := p.missingForReview(); len(missing) > 0 {
		return shared.Validation("profile is incomplete, missing: %s", strings.Join(missing, ", "))
	}
	if !p.CompanyEligible {
		return shared.Validation("the company must be active to submit its profile")
	}
	p.Status = ProfilePendingReview
	p.StatusReason = nil
	p.SubmittedAt = &now
	p.UpdatedAt = now
	return nil
}

// Approve publishes a profile under review.
func (p *DistributorProfile) Approve(reviewer string, now time.Time) error {
	if p.Status != ProfilePendingReview {
		return shared.Validation("only profiles pending review can be approved")
	}
	if !p.CompanyEligible {
		return shared.Validation("the company must be active to publish its profile")
	}
	p.review(ProfilePublished, reviewer, nil, now)
	p.PublishedAt = &now
	return nil
}

// Reject returns a profile under review to its owner with a reason.
func (p *DistributorProfile) Reject(reviewer, reason string, now time.Time) error {
	if p.Status != ProfilePendingReview {
		return shared.Validation("only profiles pending review can be rejected")
	}
	r, err := shared.RequireText("reason", reason, 500)
	if err != nil {
		return err
	}
	p.review(ProfileRejected, reviewer, &r, now)
	return nil
}

// Suspend removes a published profile from the marketplace.
func (p *DistributorProfile) Suspend(reviewer, reason string, now time.Time) error {
	if p.Status != ProfilePublished {
		return shared.Validation("only published profiles can be suspended")
	}
	r, err := shared.RequireText("reason", reason, 500)
	if err != nil {
		return err
	}
	p.review(ProfileSuspended, reviewer, &r, now)
	return nil
}

// Reinstate publishes a suspended profile again.
func (p *DistributorProfile) Reinstate(reviewer string, now time.Time) error {
	if p.Status != ProfileSuspended {
		return shared.Validation("only suspended profiles can be reinstated")
	}
	p.review(ProfilePublished, reviewer, nil, now)
	return nil
}

func (p *DistributorProfile) review(to ProfileStatus, reviewer string, reason *string, now time.Time) {
	p.Status = to
	p.StatusReason = reason
	p.ReviewedBy = &reviewer
	p.ReviewedAt = &now
	p.UpdatedAt = now
}

// SetCompanyEligible mirrors the company status/type. It reports whether it changed.
func (p *DistributorProfile) SetCompanyEligible(eligible bool, now time.Time) bool {
	if p.CompanyEligible == eligible {
		return false
	}
	p.CompanyEligible = eligible
	p.UpdatedAt = now
	return true
}

// IsListed reports whether the profile is visible in the marketplace.
func (p *DistributorProfile) IsListed() bool {
	return p.Status == ProfilePublished && p.CompanyEligible
}

// NormalizeIDs trims, rejects blanks and removes duplicates keeping the order.
func NormalizeIDs(field string, ids []string, max int) ([]string, error) {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		v := strings.TrimSpace(id)
		if v == "" {
			return nil, shared.Validation("%s cannot contain empty values", field)
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) > max {
		return nil, shared.Validation("%s can have at most %d values", field, max)
	}
	return out, nil
}

func optionalString(field, value string, max int) (string, error) {
	v, err := shared.OptionalText(field, &value, max)
	if err != nil || v == nil {
		return "", err
	}
	return *v, nil
}

func optionalURL(field string, value *string) (*string, error) {
	v, err := shared.OptionalText(field, value, 500)
	if err != nil || v == nil {
		return nil, err
	}
	u, err := url.Parse(*v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, shared.Validation("%s must be an http or https URL", field)
	}
	return v, nil
}

func optionalPhone(field string, value *string) (*string, error) {
	v, err := shared.OptionalText(field, value, 30)
	if err != nil || v == nil {
		return nil, err
	}
	if !phoneRe.MatchString(*v) {
		return nil, shared.Validation("%s must be a phone number", field)
	}
	return v, nil
}
