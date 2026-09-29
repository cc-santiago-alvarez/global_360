package memory

import (
	"context"
	"sort"
	"strings"

	"global_360/internal/application/port"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/shared"
)

// --- categories ---

type CategoryRepo struct{ S *Store }

func (r CategoryRepo) Create(_ context.Context, c *commerce.Category) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Categories {
		if x.Code == c.Code {
			return shared.Conflict("a category with this code already exists")
		}
	}
	r.S.Categories[c.ID] = *c
	return nil
}

func (r CategoryRepo) Update(_ context.Context, c *commerce.Category) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	if _, ok := r.S.Categories[c.ID]; !ok {
		return shared.NotFound("category not found")
	}
	r.S.Categories[c.ID] = *c
	return nil
}

func (r CategoryRepo) FindByID(_ context.Context, id string) (*commerce.Category, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	c, ok := r.S.Categories[id]
	if !ok {
		return nil, shared.NotFound("category not found")
	}
	return &c, nil
}

func (r CategoryRepo) FindByIDs(_ context.Context, ids []string) ([]*commerce.Category, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*commerce.Category
	for _, id := range ids {
		if c, ok := r.S.Categories[id]; ok {
			out = append(out, &c)
		}
	}
	return out, nil
}

func (r CategoryRepo) List(_ context.Context, onlyActive bool) ([]*commerce.Category, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*commerce.Category
	for _, c := range r.S.Categories {
		if !onlyActive || c.Active {
			c := c
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// --- distributor profiles ---

type ProfileRepo struct{ S *Store }

func cloneProfile(p commerce.DistributorProfile) commerce.DistributorProfile {
	p.CategoryIDs = append([]string(nil), p.CategoryIDs...)
	p.CoverageCountryIDs = append([]string(nil), p.CoverageCountryIDs...)
	return p
}

func (r ProfileRepo) Create(_ context.Context, p *commerce.DistributorProfile) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	if _, ok := r.S.Profiles[p.CompanyID]; ok {
		return shared.Conflict("the company already has a distributor profile")
	}
	r.S.Profiles[p.CompanyID] = cloneProfile(*p)
	return nil
}

func (r ProfileRepo) Update(_ context.Context, p *commerce.DistributorProfile) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	if _, ok := r.S.Profiles[p.CompanyID]; !ok {
		return shared.NotFound("distributor profile not found")
	}
	r.S.Profiles[p.CompanyID] = cloneProfile(*p)
	return nil
}

func (r ProfileRepo) FindByCompanyID(_ context.Context, companyID string) (*commerce.DistributorProfile, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	p, ok := r.S.Profiles[companyID]
	if !ok {
		return nil, shared.NotFound("distributor profile not found")
	}
	p = cloneProfile(p)
	return &p, nil
}

// ListListed approximates the Mongo $text search: any query term contained in the texts.
func (r ProfileRepo) ListListed(_ context.Context, f port.MarketplaceFilter) ([]*commerce.DistributorProfile, int64, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*commerce.DistributorProfile
	for _, p := range r.S.Profiles {
		if !p.IsListed() ||
			(f.CategoryID != nil && !contains(p.CategoryIDs, *f.CategoryID)) ||
			(f.CountryID != nil && !contains(p.CoverageCountryIDs, *f.CountryID)) ||
			(f.Query != nil && !matchesText(*f.Query, p.DisplayName, p.Summary, p.Description)) {
			continue
		}
		p := cloneProfile(p)
		out = append(out, &p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	items, total := page(out, f.Page)
	return items, total, nil
}

func matchesText(query string, texts ...string) bool {
	all := strings.ToLower(strings.Join(texts, " "))
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if strings.Contains(all, term) {
			return true
		}
	}
	return false
}

func (r ProfileRepo) ListByStatus(_ context.Context, status *commerce.ProfileStatus, p shared.Pagination) ([]*commerce.DistributorProfile, int64, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*commerce.DistributorProfile
	for _, x := range r.S.Profiles {
		if status == nil || x.Status == *status {
			x := cloneProfile(x)
			out = append(out, &x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	items, total := page(out, p)
	return items, total, nil
}

// --- distributor services ---

type ServiceRepo struct{ S *Store }

func (r ServiceRepo) Create(_ context.Context, s *commerce.DistributorService) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.Services {
		if x.CompanyID == s.CompanyID && x.Name == s.Name {
			return shared.Conflict("the distributor already has a service with this name")
		}
	}
	r.S.Services[s.ID] = *s
	return nil
}

func (r ServiceRepo) Update(_ context.Context, s *commerce.DistributorService) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	if _, ok := r.S.Services[s.ID]; !ok {
		return shared.NotFound("service not found")
	}
	r.S.Services[s.ID] = *s
	return nil
}

func (r ServiceRepo) FindByID(_ context.Context, id string) (*commerce.DistributorService, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	s, ok := r.S.Services[id]
	if !ok {
		return nil, shared.NotFound("service not found")
	}
	return &s, nil
}

func (r ServiceRepo) ListByCompany(_ context.Context, companyID string, onlyActive bool) ([]*commerce.DistributorService, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*commerce.DistributorService
	for _, s := range r.S.Services {
		if s.CompanyID == companyID && (!onlyActive || s.Active) {
			s := s
			out = append(out, &s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// --- contact requests ---

type ContactRequestRepo struct{ S *Store }

// Create mimics the partial unique index: one "new" request per user and distributor.
func (r ContactRequestRepo) Create(_ context.Context, c *commerce.ContactRequest) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	for _, x := range r.S.ContactRequests {
		if x.Status == commerce.ContactRequestNew && x.RequesterUserID == c.RequesterUserID && x.DistributorID == c.DistributorID {
			return shared.Conflict("you already have an open contact request with this distributor")
		}
	}
	r.S.ContactRequests[c.ID] = *c
	return nil
}

func (r ContactRequestRepo) Update(_ context.Context, c *commerce.ContactRequest) error {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	if _, ok := r.S.ContactRequests[c.ID]; !ok {
		return shared.NotFound("contact request not found")
	}
	r.S.ContactRequests[c.ID] = *c
	return nil
}

func (r ContactRequestRepo) FindByID(_ context.Context, id string) (*commerce.ContactRequest, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	c, ok := r.S.ContactRequests[id]
	if !ok {
		return nil, shared.NotFound("contact request not found")
	}
	return &c, nil
}

func (r ContactRequestRepo) List(_ context.Context, f port.ContactRequestFilter) ([]*commerce.ContactRequest, int64, error) {
	r.S.mu.Lock()
	defer r.S.mu.Unlock()
	var out []*commerce.ContactRequest
	for _, c := range r.S.ContactRequests {
		if (f.DistributorID != nil && c.DistributorID != *f.DistributorID) ||
			(f.RestrictToRequesters && !contains(f.RequesterCompanyIDs, c.RequesterCompanyID)) ||
			(f.Status != nil && c.Status != *f.Status) {
			continue
		}
		c := c
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	items, total := page(out, f.Page)
	return items, total, nil
}

var (
	_ port.CategoryRepository           = CategoryRepo{}
	_ port.DistributorProfileRepository = ProfileRepo{}
	_ port.DistributorServiceRepository = ServiceRepo{}
	_ port.ContactRequestRepository     = ContactRequestRepo{}
)
