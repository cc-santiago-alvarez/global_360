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

// ContactRequestService handles the leads clients send to distributors from the marketplace.
type ContactRequestService struct {
	requests  port.ContactRequestRepository
	profiles  port.DistributorProfileRepository
	services  port.DistributorServiceRepository
	companies port.CompanyRepository
	users     port.UserRepository
	persons   port.PersonRepository
	d         Deps
}

func NewContactRequestService(requests port.ContactRequestRepository, profiles port.DistributorProfileRepository,
	services port.DistributorServiceRepository, companies port.CompanyRepository, users port.UserRepository,
	persons port.PersonRepository, d Deps) *ContactRequestService {
	return &ContactRequestService{
		requests: requests, profiles: profiles, services: services, companies: companies,
		users: users, persons: persons, d: d,
	}
}

// Create sends a contact request on behalf of the actor's company. Contact data defaults
// to the actor's person record.
func (s *ContactRequestService) Create(ctx context.Context, distributorID string, cmd dto.CreateContactRequest) (dto.ContactRequest, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return dto.ContactRequest{}, err
	}
	if a.CompanyID == nil {
		return dto.ContactRequest{}, shared.Forbidden("only users of a company can contact distributors")
	}
	if !a.Grants.Can(access.PermCommerceContact, a.CompanyID) {
		return dto.ContactRequest{}, shared.Forbidden("missing permission %s", access.PermCommerceContact)
	}
	requester, err := s.companies.FindByID(ctx, *a.CompanyID)
	if err != nil {
		return dto.ContactRequest{}, err
	}
	if requester.Status != company.StatusActive {
		return dto.ContactRequest{}, shared.Validation("your company must be active to contact distributors")
	}
	p, err := s.profiles.FindByCompanyID(ctx, distributorID)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && !p.IsListed()) {
		return dto.ContactRequest{}, shared.NotFound("distributor not found")
	}
	if err != nil {
		return dto.ContactRequest{}, err
	}
	if cmd.ServiceID != nil && *cmd.ServiceID != "" {
		svc, err := s.services.FindByID(ctx, *cmd.ServiceID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return dto.ContactRequest{}, err
		}
		if svc == nil || svc.CompanyID != distributorID || !svc.Active {
			return dto.ContactRequest{}, shared.Validation("service_id is not an active service of this distributor")
		}
	} else {
		cmd.ServiceID = nil
	}
	user, err := s.users.FindByID(ctx, a.UserID)
	if err != nil {
		return dto.ContactRequest{}, err
	}
	person, err := s.persons.FindByID(ctx, user.PersonID)
	if err != nil {
		return dto.ContactRequest{}, err
	}
	data := commerce.ContactRequestData{
		DistributorID: distributorID, DistributorName: p.DisplayName, ServiceID: cmd.ServiceID,
		RequesterCompanyID: requester.ID, RequesterCompanyName: companyName(requester), RequesterUserID: a.UserID,
		ContactName: person.FirstNames + " " + person.LastNames, ContactEmail: person.Email, ContactPhone: person.Phone,
		Message: cmd.Message,
	}
	if cmd.ContactName != nil {
		data.ContactName = *cmd.ContactName
	}
	if cmd.ContactEmail != nil {
		data.ContactEmail = *cmd.ContactEmail
	}
	if cmd.ContactPhone != nil {
		data.ContactPhone = cmd.ContactPhone
	}
	r, err := commerce.NewContactRequest(s.d.IDs.NewID(), data, s.d.Clock.Now())
	if err != nil {
		return dto.ContactRequest{}, err
	}
	result := dto.NewContactRequest(r, false)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.requests.Create(ctx, r); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityContactRequest, r.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

// ListSent returns the requests sent by the companies where the actor holds commerce.contact.
func (s *ContactRequestService) ListSent(ctx context.Context, q dto.ListContactRequests) (dto.Page[dto.ContactRequest], error) {
	a, err := authorizeAny(ctx, access.PermCommerceContact)
	if err != nil {
		return dto.Page[dto.ContactRequest]{}, err
	}
	all, ids := companyScope(a.Grants, access.PermCommerceContact)
	return s.list(ctx, q, port.ContactRequestFilter{RestrictToRequesters: !all, RequesterCompanyIDs: ids}, false)
}

// ListReceived returns the requests received by a distributor.
func (s *ContactRequestService) ListReceived(ctx context.Context, distributorID string, q dto.ListContactRequests) (dto.Page[dto.ContactRequest], error) {
	if _, err := authorizeDistributor(ctx, access.PermLeadRead, distributorID); err != nil {
		return dto.Page[dto.ContactRequest]{}, err
	}
	return s.list(ctx, q, port.ContactRequestFilter{DistributorID: &distributorID}, true)
}

func (s *ContactRequestService) list(ctx context.Context, q dto.ListContactRequests, f port.ContactRequestFilter,
	withNotes bool) (dto.Page[dto.ContactRequest], error) {
	if q.Status != nil && !q.Status.Valid() {
		return dto.Page[dto.ContactRequest]{}, shared.Validation("invalid contact request status %q", *q.Status)
	}
	f.Status = q.Status
	f.Page = q.Page.Normalize()
	list, total, err := s.requests.List(ctx, f)
	if err != nil {
		return dto.Page[dto.ContactRequest]{}, err
	}
	items := make([]dto.ContactRequest, 0, len(list))
	for _, r := range list {
		items = append(items, dto.NewContactRequest(r, withNotes))
	}
	return dto.Page[dto.ContactRequest]{Items: items, Total: total, Page: f.Page.Page, PageSize: f.Page.PageSize}, nil
}

// Handle lets the distributor move a request forward and keep internal notes.
func (s *ContactRequestService) Handle(ctx context.Context, distributorID, requestID string, cmd dto.HandleContactRequest) (dto.ContactRequest, error) {
	if _, err := authorizeDistributor(ctx, access.PermLeadManage, distributorID); err != nil {
		return dto.ContactRequest{}, err
	}
	r, err := s.requests.FindByID(ctx, requestID)
	if err != nil {
		return dto.ContactRequest{}, err
	}
	if r.DistributorID != distributorID {
		return dto.ContactRequest{}, shared.NotFound("contact request not found")
	}
	before := dto.NewContactRequest(r, true)
	if err := r.Handle(cmd.Status, cmd.Notes, s.d.Clock.Now()); err != nil {
		return dto.ContactRequest{}, err
	}
	after := dto.NewContactRequest(r, true)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.requests.Update(ctx, r); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityContactRequest, r.ID, audit.ActionUpdate, before, after)
	})
	if err != nil {
		return dto.ContactRequest{}, err
	}
	return after, nil
}

func companyName(c *company.Company) string {
	if c.TradeName != nil {
		return *c.TradeName
	}
	return c.LegalName
}
