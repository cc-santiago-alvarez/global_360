package dto

import (
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/shared"
)

type CreateCategory struct {
	Code        string
	Name        string
	Description *string
}

type UpdateCategory struct {
	Name        *string
	Description *string
	Active      *bool
}

// DistributorProfileInput replaces the whole editable content of a profile.
type DistributorProfileInput struct {
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

// DistributorServiceInput creates (Name and CategoryID required) or updates a service.
type DistributorServiceInput struct {
	CategoryID           *string
	Name                 *string
	Description          *string
	OriginCountryID      *string
	DestinationCountryID *string
	Active               *bool
}

type ListDistributors struct {
	Query      *string
	CategoryID *string
	CountryID  *string
	Page       shared.Pagination
}

type ListDistributorProfiles struct {
	Status *commerce.ProfileStatus
	Page   shared.Pagination
}

// CreateContactRequest: contact fields default to the requester's person data.
type CreateContactRequest struct {
	ServiceID    *string
	Message      string
	ContactName  *string
	ContactEmail *string
	ContactPhone *string
}

type HandleContactRequest struct {
	Status *commerce.ContactRequestStatus
	Notes  *string
}

type ListContactRequests struct {
	Status *commerce.ContactRequestStatus
	Page   shared.Pagination
}
