package service

import (
	"context"
	"errors"

	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/shared"
)

// CompanyService manages companies (clients, providers and Global 360 itself).
type CompanyService struct {
	companies  port.CompanyRepository
	countries  port.CountryRepository
	currencies port.CurrencyRepository
	profiles   port.DistributorProfileRepository
	d          Deps
}

func NewCompanyService(companies port.CompanyRepository, countries port.CountryRepository, currencies port.CurrencyRepository,
	profiles port.DistributorProfileRepository, d Deps) *CompanyService {
	return &CompanyService{companies: companies, countries: countries, currencies: currencies, profiles: profiles, d: d}
}

// Create registers a company in pending_validation. Requires global company.manage.
func (s *CompanyService) Create(ctx context.Context, cmd dto.CreateCompany) (dto.Company, error) {
	if _, err := authorizeGlobal(ctx, access.PermCompanyManage); err != nil {
		return dto.Company{}, err
	}
	country, err := s.countries.FindByID(ctx, cmd.CountryID)
	if err != nil {
		return dto.Company{}, refErr(err, "country_id")
	}
	if !country.Active {
		return dto.Company{}, shared.Validation("country_id refers to an inactive country")
	}
	if err := s.ensureActiveCurrency(ctx, cmd.BillingCurrencyID); err != nil {
		return dto.Company{}, err
	}
	c, err := company.New(s.d.IDs.NewID(), company.Data{
		LegalName: cmd.LegalName, TradeName: cmd.TradeName, DocumentType: cmd.DocumentType,
		DocumentNumber: cmd.DocumentNumber, CountryID: cmd.CountryID, BillingCurrencyID: cmd.BillingCurrencyID,
		Type: cmd.CompanyType, ContactEmail: cmd.ContactEmail, ContactPhone: cmd.ContactPhone,
	}, s.d.Clock.Now())
	if err != nil {
		return dto.Company{}, err
	}
	result := dto.NewCompany(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.companies.Create(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCompany, c.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

func (s *CompanyService) Get(ctx context.Context, id string) (dto.Company, error) {
	if _, err := authorize(ctx, access.PermCompanyRead, &id); err != nil {
		return dto.Company{}, err
	}
	c, err := s.companies.FindByID(ctx, id)
	if err != nil {
		return dto.Company{}, err
	}
	return dto.NewCompany(c), nil
}

// List returns all companies with global company.read, otherwise only the
// companies where the actor holds company.read.
func (s *CompanyService) List(ctx context.Context, q dto.ListCompanies) (dto.Page[dto.Company], error) {
	a, err := authorizeAny(ctx, access.PermCompanyRead)
	if err != nil {
		return dto.Page[dto.Company]{}, err
	}
	if q.Status != nil && !q.Status.Valid() {
		return dto.Page[dto.Company]{}, shared.Validation("invalid company status %q", *q.Status)
	}
	if q.CompanyType != nil && !q.CompanyType.Valid() {
		return dto.Page[dto.Company]{}, shared.Validation("invalid company_type %q", *q.CompanyType)
	}
	page := q.Page.Normalize()
	all, ids := companyScope(a.Grants, access.PermCompanyRead)
	companies, total, err := s.companies.List(ctx, port.CompanyFilter{
		RestrictToIDs: !all, IDs: ids, Status: q.Status, Type: q.CompanyType, Page: page,
	})
	if err != nil {
		return dto.Page[dto.Company]{}, err
	}
	items := make([]dto.Company, 0, len(companies))
	for _, c := range companies {
		items = append(items, dto.NewCompany(c))
	}
	return dto.Page[dto.Company]{Items: items, Total: total, Page: page.Page, PageSize: page.PageSize}, nil
}

func (s *CompanyService) Update(ctx context.Context, id string, cmd dto.UpdateCompany) (dto.Company, error) {
	if _, err := authorize(ctx, access.PermCompanyManage, &id); err != nil {
		return dto.Company{}, err
	}
	c, err := s.companies.FindByID(ctx, id)
	if err != nil {
		return dto.Company{}, err
	}
	if cmd.BillingCurrencyID != nil && *cmd.BillingCurrencyID != c.BillingCurrencyID {
		if err := s.ensureActiveCurrency(ctx, *cmd.BillingCurrencyID); err != nil {
			return dto.Company{}, err
		}
	}
	before := dto.NewCompany(c)
	err = c.Apply(company.Changes{
		LegalName: cmd.LegalName, TradeName: cmd.TradeName, BillingCurrencyID: cmd.BillingCurrencyID,
		Type: cmd.CompanyType, ContactEmail: cmd.ContactEmail, ContactPhone: cmd.ContactPhone,
	}, s.d.Clock.Now())
	if err != nil {
		return dto.Company{}, err
	}
	return s.save(ctx, c, before)
}

func (s *CompanyService) ChangeStatus(ctx context.Context, id string, status company.Status) (dto.Company, error) {
	if _, err := authorize(ctx, access.PermCompanyManage, &id); err != nil {
		return dto.Company{}, err
	}
	c, err := s.companies.FindByID(ctx, id)
	if err != nil {
		return dto.Company{}, err
	}
	before := dto.NewCompany(c)
	if err := c.ChangeStatus(status, s.d.Clock.Now()); err != nil {
		return dto.Company{}, err
	}
	return s.save(ctx, c, before)
}

func (s *CompanyService) save(ctx context.Context, c *company.Company, before dto.Company) (dto.Company, error) {
	after := dto.NewCompany(c)
	err := s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.companies.Update(ctx, c); err != nil {
			return err
		}
		if err := s.d.Audit.Record(ctx, audit.EntityCompany, c.ID, audit.ActionUpdate, before, after); err != nil {
			return err
		}
		return s.syncDistributorProfile(ctx, c)
	})
	if err != nil {
		return dto.Company{}, err
	}
	return after, nil
}

// syncDistributorProfile keeps the marketplace visibility of the company's distributor
// profile in line with the company status and type.
func (s *CompanyService) syncDistributorProfile(ctx context.Context, c *company.Company) error {
	p, err := s.profiles.FindByCompanyID(ctx, c.ID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	before := dto.NewDistributorProfile(p)
	if !p.SetCompanyEligible(commerce.CompanyEligible(c), s.d.Clock.Now()) {
		return nil
	}
	if err := s.profiles.Update(ctx, p); err != nil {
		return err
	}
	return s.d.Audit.Record(ctx, audit.EntityDistributorProfile, p.CompanyID, audit.ActionUpdate, before, dto.NewDistributorProfile(p))
}

func (s *CompanyService) ensureActiveCurrency(ctx context.Context, id string) error {
	cur, err := s.currencies.FindByID(ctx, id)
	if err != nil {
		return refErr(err, "billing_currency_id")
	}
	if !cur.Active {
		return shared.Validation("billing_currency_id refers to an inactive currency")
	}
	return nil
}
