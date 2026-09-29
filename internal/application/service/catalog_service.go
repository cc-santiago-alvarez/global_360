package service

import (
	"context"

	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/catalog"
)

// CatalogService manages countries and currencies.
type CatalogService struct {
	countries  port.CountryRepository
	currencies port.CurrencyRepository
	d          Deps
}

func NewCatalogService(countries port.CountryRepository, currencies port.CurrencyRepository, d Deps) *CatalogService {
	return &CatalogService{countries: countries, currencies: currencies, d: d}
}

func (s *CatalogService) ListCountries(ctx context.Context, onlyActive bool) ([]dto.Country, error) {
	if _, err := authorizeAny(ctx, access.PermCatalogRead); err != nil {
		return nil, err
	}
	items, err := s.countries.List(ctx, onlyActive)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Country, 0, len(items))
	for _, c := range items {
		out = append(out, dto.NewCountry(c))
	}
	return out, nil
}

func (s *CatalogService) CreateCountry(ctx context.Context, cmd dto.CreateCountry) (dto.Country, error) {
	if _, err := authorizeGlobal(ctx, access.PermCatalogManage); err != nil {
		return dto.Country{}, err
	}
	c, err := catalog.NewCountry(s.d.IDs.NewID(), cmd.ISOCode, cmd.Name, s.d.Clock.Now())
	if err != nil {
		return dto.Country{}, err
	}
	result := dto.NewCountry(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.countries.Create(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCountry, c.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

func (s *CatalogService) UpdateCountry(ctx context.Context, id string, cmd dto.UpdateCountry) (dto.Country, error) {
	if _, err := authorizeGlobal(ctx, access.PermCatalogManage); err != nil {
		return dto.Country{}, err
	}
	c, err := s.countries.FindByID(ctx, id)
	if err != nil {
		return dto.Country{}, err
	}
	before := dto.NewCountry(c)
	if cmd.Name != nil {
		if err := c.Rename(*cmd.Name); err != nil {
			return dto.Country{}, err
		}
	}
	if cmd.Active != nil {
		c.Active = *cmd.Active
	}
	after := dto.NewCountry(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.countries.Update(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCountry, c.ID, audit.ActionUpdate, before, after)
	})
	return after, err
}

func (s *CatalogService) ListCurrencies(ctx context.Context, onlyActive bool) ([]dto.Currency, error) {
	if _, err := authorizeAny(ctx, access.PermCatalogRead); err != nil {
		return nil, err
	}
	items, err := s.currencies.List(ctx, onlyActive)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Currency, 0, len(items))
	for _, c := range items {
		out = append(out, dto.NewCurrency(c))
	}
	return out, nil
}

func (s *CatalogService) CreateCurrency(ctx context.Context, cmd dto.CreateCurrency) (dto.Currency, error) {
	if _, err := authorizeGlobal(ctx, access.PermCatalogManage); err != nil {
		return dto.Currency{}, err
	}
	c, err := catalog.NewCurrency(s.d.IDs.NewID(), cmd.ISOCode, cmd.Name, cmd.Symbol, cmd.Decimals, s.d.Clock.Now())
	if err != nil {
		return dto.Currency{}, err
	}
	result := dto.NewCurrency(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.currencies.Create(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCurrency, c.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

func (s *CatalogService) UpdateCurrency(ctx context.Context, id string, cmd dto.UpdateCurrency) (dto.Currency, error) {
	if _, err := authorizeGlobal(ctx, access.PermCatalogManage); err != nil {
		return dto.Currency{}, err
	}
	c, err := s.currencies.FindByID(ctx, id)
	if err != nil {
		return dto.Currency{}, err
	}
	before := dto.NewCurrency(c)
	if err := c.Update(cmd.Name, cmd.Symbol, cmd.Decimals); err != nil {
		return dto.Currency{}, err
	}
	if cmd.Active != nil {
		c.Active = *cmd.Active
	}
	after := dto.NewCurrency(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.currencies.Update(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCurrency, c.ID, audit.ActionUpdate, before, after)
	})
	return after, err
}
