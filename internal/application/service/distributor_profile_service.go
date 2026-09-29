package service

import (
	"context"
	"errors"

	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/shared"
)

// DistributorProfileService lets provider companies manage their marketplace profile and
// services, and lets Global 360 review (approve, reject, suspend) the profiles.
type DistributorProfileService struct {
	profiles  port.DistributorProfileRepository
	services  port.DistributorServiceRepository
	companies port.CompanyRepository
	refs      commerceRefs
	d         Deps
}

func NewDistributorProfileService(profiles port.DistributorProfileRepository, services port.DistributorServiceRepository,
	companies port.CompanyRepository, categories port.CategoryRepository, countries port.CountryRepository, d Deps) *DistributorProfileService {
	return &DistributorProfileService{
		profiles: profiles, services: services, companies: companies, refs: commerceRefs{categories, countries}, d: d,
	}
}

func (s *DistributorProfileService) GetProfile(ctx context.Context, companyID string) (dto.DistributorProfile, error) {
	if _, err := authorizeDistributor(ctx, access.PermDistributorManage, companyID); err != nil {
		return dto.DistributorProfile{}, err
	}
	p, err := s.findProfile(ctx, companyID)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	return dto.NewDistributorProfile(p), nil
}

// UpsertProfile creates the draft profile on first use and replaces its content afterwards.
// It reports whether the profile was created.
func (s *DistributorProfileService) UpsertProfile(ctx context.Context, companyID string, in dto.DistributorProfileInput) (dto.DistributorProfile, bool, error) {
	if _, err := authorizeDistributor(ctx, access.PermDistributorManage, companyID); err != nil {
		return dto.DistributorProfile{}, false, err
	}
	c, err := s.companies.FindByID(ctx, companyID)
	if err != nil {
		return dto.DistributorProfile{}, false, err
	}
	content := commerce.ProfileContent(in)
	if err := s.refs.ensureCategories(ctx, "category_ids", content.CategoryIDs); err != nil {
		return dto.DistributorProfile{}, false, err
	}
	if err := s.refs.ensureCountries(ctx, "coverage_country_ids", content.CoverageCountryIDs); err != nil {
		return dto.DistributorProfile{}, false, err
	}
	now := s.d.Clock.Now()
	p, err := s.profiles.FindByCompanyID(ctx, companyID)
	if errors.Is(err, shared.ErrNotFound) {
		p, err = commerce.NewProfile(c, content, now)
		if err != nil {
			return dto.DistributorProfile{}, false, err
		}
		result := dto.NewDistributorProfile(p)
		err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
			if err := s.profiles.Create(ctx, p); err != nil {
				return err
			}
			return s.d.Audit.Record(ctx, audit.EntityDistributorProfile, p.CompanyID, audit.ActionCreate, nil, result)
		})
		return result, true, err
	}
	if err != nil {
		return dto.DistributorProfile{}, false, err
	}
	before := dto.NewDistributorProfile(p)
	p.SetCompanyEligible(commerce.CompanyEligible(c), now)
	if err := p.Update(content, now); err != nil {
		return dto.DistributorProfile{}, false, err
	}
	result, err := s.save(ctx, p, before, audit.ActionUpdate)
	return result, false, err
}

// Submit sends the profile to Global 360 for review.
func (s *DistributorProfileService) Submit(ctx context.Context, companyID string) (dto.DistributorProfile, error) {
	if _, err := authorizeDistributor(ctx, access.PermDistributorManage, companyID); err != nil {
		return dto.DistributorProfile{}, err
	}
	return s.transition(ctx, companyID, audit.ActionUpdate, func(p *commerce.DistributorProfile) error {
		return p.Submit(s.d.Clock.Now())
	})
}

func (s *DistributorProfileService) Approve(ctx context.Context, companyID string) (dto.DistributorProfile, error) {
	a, err := authorizeGlobal(ctx, access.PermCommerceManage)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	return s.transition(ctx, companyID, audit.ActionApprove, func(p *commerce.DistributorProfile) error {
		return p.Approve(a.UserID, s.d.Clock.Now())
	})
}

func (s *DistributorProfileService) Reject(ctx context.Context, companyID, reason string) (dto.DistributorProfile, error) {
	a, err := authorizeGlobal(ctx, access.PermCommerceManage)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	return s.transition(ctx, companyID, audit.ActionReject, func(p *commerce.DistributorProfile) error {
		return p.Reject(a.UserID, reason, s.d.Clock.Now())
	})
}

func (s *DistributorProfileService) Suspend(ctx context.Context, companyID, reason string) (dto.DistributorProfile, error) {
	a, err := authorizeGlobal(ctx, access.PermCommerceManage)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	return s.transition(ctx, companyID, audit.ActionUpdate, func(p *commerce.DistributorProfile) error {
		return p.Suspend(a.UserID, reason, s.d.Clock.Now())
	})
}

func (s *DistributorProfileService) Reinstate(ctx context.Context, companyID string) (dto.DistributorProfile, error) {
	a, err := authorizeGlobal(ctx, access.PermCommerceManage)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	return s.transition(ctx, companyID, audit.ActionUpdate, func(p *commerce.DistributorProfile) error {
		return p.Reinstate(a.UserID, s.d.Clock.Now())
	})
}

// ListProfiles is the Global 360 review queue (any status, or filtered by one).
func (s *DistributorProfileService) ListProfiles(ctx context.Context, q dto.ListDistributorProfiles) (dto.Page[dto.DistributorProfile], error) {
	if _, err := authorizeGlobal(ctx, access.PermCommerceManage); err != nil {
		return dto.Page[dto.DistributorProfile]{}, err
	}
	if q.Status != nil && !q.Status.Valid() {
		return dto.Page[dto.DistributorProfile]{}, shared.Validation("invalid profile status %q", *q.Status)
	}
	page := q.Page.Normalize()
	profiles, total, err := s.profiles.ListByStatus(ctx, q.Status, page)
	if err != nil {
		return dto.Page[dto.DistributorProfile]{}, err
	}
	items := make([]dto.DistributorProfile, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, dto.NewDistributorProfile(p))
	}
	return dto.Page[dto.DistributorProfile]{Items: items, Total: total, Page: page.Page, PageSize: page.PageSize}, nil
}

// transition refreshes the company eligibility, applies fn and persists the result.
func (s *DistributorProfileService) transition(ctx context.Context, companyID string, action audit.Action,
	fn func(*commerce.DistributorProfile) error) (dto.DistributorProfile, error) {
	p, err := s.findProfile(ctx, companyID)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	c, err := s.companies.FindByID(ctx, companyID)
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	before := dto.NewDistributorProfile(p)
	p.SetCompanyEligible(commerce.CompanyEligible(c), s.d.Clock.Now())
	if err := fn(p); err != nil {
		return dto.DistributorProfile{}, err
	}
	return s.save(ctx, p, before, action)
}

func (s *DistributorProfileService) save(ctx context.Context, p *commerce.DistributorProfile, before dto.DistributorProfile,
	action audit.Action) (dto.DistributorProfile, error) {
	after := dto.NewDistributorProfile(p)
	err := s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.profiles.Update(ctx, p); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityDistributorProfile, p.CompanyID, action, before, after)
	})
	if err != nil {
		return dto.DistributorProfile{}, err
	}
	return after, nil
}

func (s *DistributorProfileService) findProfile(ctx context.Context, companyID string) (*commerce.DistributorProfile, error) {
	p, err := s.profiles.FindByCompanyID(ctx, companyID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, shared.NotFound("distributor profile not found")
	}
	return p, err
}

// --- services offered by the distributor ---

// ListServices returns every service of the distributor, including inactive ones.
func (s *DistributorProfileService) ListServices(ctx context.Context, companyID string) ([]dto.DistributorService, error) {
	if _, err := authorizeDistributor(ctx, access.PermDistributorManage, companyID); err != nil {
		return nil, err
	}
	list, err := s.services.ListByCompany(ctx, companyID, false)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DistributorService, 0, len(list))
	for _, svc := range list {
		out = append(out, dto.NewDistributorService(svc))
	}
	return out, nil
}

func (s *DistributorProfileService) CreateService(ctx context.Context, companyID string, in dto.DistributorServiceInput) (dto.DistributorService, error) {
	if _, err := authorizeDistributor(ctx, access.PermDistributorManage, companyID); err != nil {
		return dto.DistributorService{}, err
	}
	if _, err := s.profiles.FindByCompanyID(ctx, companyID); errors.Is(err, shared.ErrNotFound) {
		return dto.DistributorService{}, shared.Validation("create the distributor profile before adding services")
	} else if err != nil {
		return dto.DistributorService{}, err
	}
	if err := s.ensureServiceRefs(ctx, in); err != nil {
		return dto.DistributorService{}, err
	}
	svc, err := commerce.NewDistributorService(s.d.IDs.NewID(), companyID, commerce.ServiceChanges(in), s.d.Clock.Now())
	if err != nil {
		return dto.DistributorService{}, err
	}
	result := dto.NewDistributorService(svc)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.services.Create(ctx, svc); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityDistributorService, svc.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

func (s *DistributorProfileService) UpdateService(ctx context.Context, companyID, serviceID string, in dto.DistributorServiceInput) (dto.DistributorService, error) {
	if _, err := authorizeDistributor(ctx, access.PermDistributorManage, companyID); err != nil {
		return dto.DistributorService{}, err
	}
	svc, err := s.services.FindByID(ctx, serviceID)
	if err != nil {
		return dto.DistributorService{}, err
	}
	if svc.CompanyID != companyID {
		return dto.DistributorService{}, shared.NotFound("service not found")
	}
	if err := s.ensureServiceRefs(ctx, in); err != nil {
		return dto.DistributorService{}, err
	}
	before := dto.NewDistributorService(svc)
	if err := svc.Update(commerce.ServiceChanges(in), s.d.Clock.Now()); err != nil {
		return dto.DistributorService{}, err
	}
	after := dto.NewDistributorService(svc)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.services.Update(ctx, svc); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityDistributorService, svc.ID, audit.ActionUpdate, before, after)
	})
	if err != nil {
		return dto.DistributorService{}, err
	}
	return after, nil
}

func (s *DistributorProfileService) ensureServiceRefs(ctx context.Context, in dto.DistributorServiceInput) error {
	if err := s.refs.ensureCategories(ctx, "category_id", optionalRef(in.CategoryID)); err != nil {
		return err
	}
	if err := s.refs.ensureCountries(ctx, "origin_country_id", optionalRef(in.OriginCountryID)); err != nil {
		return err
	}
	return s.refs.ensureCountries(ctx, "destination_country_id", optionalRef(in.DestinationCountryID))
}
