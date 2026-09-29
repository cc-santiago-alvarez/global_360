package service

import (
	"context"
	"errors"
	"strings"

	"global_360/internal/application/actor"
	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/shared"
)

// MarketplaceService is the public ("commerce") read side: it only exposes published
// profiles of eligible companies. Contact data is disclosed to authenticated users only.
type MarketplaceService struct {
	profiles port.DistributorProfileRepository
	services port.DistributorServiceRepository
	refs     commerceRefs
}

func NewMarketplaceService(profiles port.DistributorProfileRepository, services port.DistributorServiceRepository,
	categories port.CategoryRepository, countries port.CountryRepository) *MarketplaceService {
	return &MarketplaceService{profiles: profiles, services: services, refs: commerceRefs{categories, countries}}
}

func (s *MarketplaceService) ListDistributors(ctx context.Context, q dto.ListDistributors) (dto.Page[dto.DistributorCard], error) {
	page := q.Page.Normalize()
	if q.Query != nil {
		if v := strings.TrimSpace(*q.Query); v == "" {
			q.Query = nil
		} else if len([]rune(v)) > 100 {
			return dto.Page[dto.DistributorCard]{}, shared.Validation("q must have at most 100 characters")
		}
	}
	profiles, total, err := s.profiles.ListListed(ctx, port.MarketplaceFilter{
		Query: q.Query, CategoryID: q.CategoryID, CountryID: q.CountryID, Page: page,
	})
	if err != nil {
		return dto.Page[dto.DistributorCard]{}, err
	}
	l, err := s.refs.lookups(ctx)
	if err != nil {
		return dto.Page[dto.DistributorCard]{}, err
	}
	items := make([]dto.DistributorCard, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, dto.NewDistributorCard(p, l))
	}
	return dto.Page[dto.DistributorCard]{Items: items, Total: total, Page: page.Page, PageSize: page.PageSize}, nil
}

// GetDistributor returns a listed distributor with its active services.
func (s *MarketplaceService) GetDistributor(ctx context.Context, companyID string) (dto.DistributorDetail, error) {
	p, err := s.profiles.FindByCompanyID(ctx, companyID)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && !p.IsListed()) {
		return dto.DistributorDetail{}, shared.NotFound("distributor not found")
	}
	if err != nil {
		return dto.DistributorDetail{}, err
	}
	services, err := s.services.ListByCompany(ctx, companyID, true)
	if err != nil {
		return dto.DistributorDetail{}, err
	}
	l, err := s.refs.lookups(ctx)
	if err != nil {
		return dto.DistributorDetail{}, err
	}
	withContact := actor.FromContext(ctx).Authenticated()
	return dto.NewDistributorDetail(p, services, l, withContact), nil
}
