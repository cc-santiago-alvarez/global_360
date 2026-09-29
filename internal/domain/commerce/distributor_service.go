package commerce

import (
	"strings"
	"time"

	"global_360/internal/domain/shared"
)

// DistributorService is a commercial logistics offering of a distributor
// (for example "Ground transport Bogotá -> San José").
type DistributorService struct {
	ID                   string
	CompanyID            string
	CategoryID           string
	Name                 string
	Description          *string
	OriginCountryID      *string
	DestinationCountryID *string
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ServiceChanges holds optional updates; nil fields are left untouched and blank
// optional values clear the field.
type ServiceChanges struct {
	CategoryID           *string
	Name                 *string
	Description          *string
	OriginCountryID      *string
	DestinationCountryID *string
	Active               *bool
}

func NewDistributorService(id, companyID string, data ServiceChanges, now time.Time) (*DistributorService, error) {
	if data.CategoryID == nil {
		return nil, shared.Validation("category_id is required")
	}
	if data.Name == nil {
		return nil, shared.Validation("name is required")
	}
	s := &DistributorService{ID: id, CompanyID: companyID, Active: true, CreatedAt: now}
	data.Active = nil
	if err := s.Update(data, now); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DistributorService) Update(ch ServiceChanges, now time.Time) error {
	if ch.CategoryID != nil {
		v := strings.TrimSpace(*ch.CategoryID)
		if v == "" {
			return shared.Validation("category_id is required")
		}
		s.CategoryID = v
	}
	if ch.Name != nil {
		v, err := shared.RequireText("name", *ch.Name, 150)
		if err != nil {
			return err
		}
		s.Name = v
	}
	if ch.Description != nil {
		v, err := shared.OptionalText("description", ch.Description, 2000)
		if err != nil {
			return err
		}
		s.Description = v
	}
	if ch.OriginCountryID != nil {
		v, _ := shared.OptionalText("origin_country_id", ch.OriginCountryID, 64)
		s.OriginCountryID = v
	}
	if ch.DestinationCountryID != nil {
		v, _ := shared.OptionalText("destination_country_id", ch.DestinationCountryID, 64)
		s.DestinationCountryID = v
	}
	if ch.Active != nil {
		s.Active = *ch.Active
	}
	s.UpdatedAt = now
	return nil
}
