package service

import (
	"context"

	"global_360/internal/application/actor"
	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/catalog"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/shared"
)

// authorizeDistributor allows Global 360 staff (global commerce.manage) or holders of
// code in the distributor company.
func authorizeDistributor(ctx context.Context, code, companyID string) (actor.Actor, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return a, err
	}
	if a.Grants.CanGlobally(access.PermCommerceManage) || a.Grants.Can(code, &companyID) {
		return a, nil
	}
	return a, shared.Forbidden("missing permission %s", code)
}

// commerceRefs validates and resolves the catalog references used by the marketplace.
type commerceRefs struct {
	categories port.CategoryRepository
	countries  port.CountryRepository
}

// ensureCategories checks every id is an existing, active category.
func (r commerceRefs) ensureCategories(ctx context.Context, field string, ids []string) error {
	ids, err := commerce.NormalizeIDs(field, ids, len(ids))
	if err != nil || len(ids) == 0 {
		return err
	}
	found, err := r.categories.FindByIDs(ctx, ids)
	if err != nil {
		return err
	}
	active := make(map[string]bool, len(found))
	for _, c := range found {
		active[c.ID] = c.Active
	}
	for _, id := range ids {
		if !active[id] {
			return shared.Validation("%s contains an unknown or inactive category: %s", field, id)
		}
	}
	return nil
}

// ensureCountries checks every id is an existing, active country.
func (r commerceRefs) ensureCountries(ctx context.Context, field string, ids []string) error {
	ids, err := commerce.NormalizeIDs(field, ids, len(ids))
	if err != nil || len(ids) == 0 {
		return err
	}
	countries, err := r.countries.List(ctx, true)
	if err != nil {
		return err
	}
	active := make(map[string]bool, len(countries))
	for _, c := range countries {
		active[c.ID] = true
	}
	for _, id := range ids {
		if !active[id] {
			return shared.Validation("%s contains an unknown or inactive country: %s", field, id)
		}
	}
	return nil
}

// optionalRef returns the id to validate, or nil when the value is absent or blank.
func optionalRef(v *string) []string {
	if v == nil || *v == "" {
		return nil
	}
	return []string{*v}
}

func (r commerceRefs) lookups(ctx context.Context) (dto.Lookups, error) {
	categories, err := r.categories.List(ctx, false)
	if err != nil {
		return dto.Lookups{}, err
	}
	countries, err := r.countries.List(ctx, false)
	if err != nil {
		return dto.Lookups{}, err
	}
	l := dto.Lookups{
		Categories: make(map[string]*commerce.Category, len(categories)),
		Countries:  make(map[string]*catalog.Country, len(countries)),
	}
	for _, c := range categories {
		l.Categories[c.ID] = c
	}
	for _, c := range countries {
		l.Countries[c.ID] = c
	}
	return l, nil
}
