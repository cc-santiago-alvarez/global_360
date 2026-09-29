package service

import (
	"context"

	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/commerce"
)

// CategoryService manages the marketplace categories.
type CategoryService struct {
	categories port.CategoryRepository
	d          Deps
}

func NewCategoryService(categories port.CategoryRepository, d Deps) *CategoryService {
	return &CategoryService{categories: categories, d: d}
}

// List is public for active categories; including inactive ones requires global commerce.manage.
func (s *CategoryService) List(ctx context.Context, includeInactive bool) ([]dto.Category, error) {
	if includeInactive {
		if _, err := authorizeGlobal(ctx, access.PermCommerceManage); err != nil {
			return nil, err
		}
	}
	list, err := s.categories.List(ctx, !includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Category, 0, len(list))
	for _, c := range list {
		out = append(out, dto.NewCategory(c))
	}
	return out, nil
}

func (s *CategoryService) Create(ctx context.Context, cmd dto.CreateCategory) (dto.Category, error) {
	if _, err := authorizeGlobal(ctx, access.PermCommerceManage); err != nil {
		return dto.Category{}, err
	}
	c, err := commerce.NewCategory(s.d.IDs.NewID(), cmd.Code, cmd.Name, cmd.Description, s.d.Clock.Now())
	if err != nil {
		return dto.Category{}, err
	}
	result := dto.NewCategory(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.categories.Create(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCategory, c.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

func (s *CategoryService) Update(ctx context.Context, id string, cmd dto.UpdateCategory) (dto.Category, error) {
	if _, err := authorizeGlobal(ctx, access.PermCommerceManage); err != nil {
		return dto.Category{}, err
	}
	c, err := s.categories.FindByID(ctx, id)
	if err != nil {
		return dto.Category{}, err
	}
	before := dto.NewCategory(c)
	if err := c.Update(cmd.Name, cmd.Description, cmd.Active, s.d.Clock.Now()); err != nil {
		return dto.Category{}, err
	}
	after := dto.NewCategory(c)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.categories.Update(ctx, c); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityCategory, c.ID, audit.ActionUpdate, before, after)
	})
	if err != nil {
		return dto.Category{}, err
	}
	return after, nil
}
