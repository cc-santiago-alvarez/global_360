package commerce

import (
	"time"

	"global_360/internal/domain/shared"
)

// ContactRequestStatus tracks how the distributor handled a lead.
type ContactRequestStatus string

const (
	ContactRequestNew       ContactRequestStatus = "new"
	ContactRequestContacted ContactRequestStatus = "contacted"
	ContactRequestClosed    ContactRequestStatus = "closed"
)

var ContactRequestStatuses = []ContactRequestStatus{ContactRequestNew, ContactRequestContacted, ContactRequestClosed}

func (s ContactRequestStatus) Valid() bool {
	for _, v := range ContactRequestStatuses {
		if s == v {
			return true
		}
	}
	return false
}

var contactRequestTransitions = map[ContactRequestStatus][]ContactRequestStatus{
	ContactRequestNew:       {ContactRequestContacted, ContactRequestClosed},
	ContactRequestContacted: {ContactRequestClosed},
}

const (
	MinContactMessage = 10
	MaxContactMessage = 2000
)

// ContactRequest is a business opportunity (lead) sent by a client company to a distributor.
// Names and contact data are snapshots taken when the request is created.
type ContactRequest struct {
	ID                   string
	DistributorID        string
	DistributorName      string
	ServiceID            *string
	RequesterCompanyID   string
	RequesterCompanyName string
	RequesterUserID      string
	ContactName          string
	ContactEmail         string
	ContactPhone         *string
	Message              string
	Status               ContactRequestStatus
	DistributorNotes     *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ContactRequestData holds the attributes of a new contact request.
type ContactRequestData struct {
	DistributorID        string
	DistributorName      string
	ServiceID            *string
	RequesterCompanyID   string
	RequesterCompanyName string
	RequesterUserID      string
	ContactName          string
	ContactEmail         string
	ContactPhone         *string
	Message              string
}

func NewContactRequest(id string, d ContactRequestData, now time.Time) (*ContactRequest, error) {
	if d.DistributorID == d.RequesterCompanyID {
		return nil, shared.Validation("a company cannot contact itself")
	}
	msg, err := shared.RequireText("message", d.Message, MaxContactMessage)
	if err != nil {
		return nil, err
	}
	if len([]rune(msg)) < MinContactMessage {
		return nil, shared.Validation("message must have at least %d characters", MinContactMessage)
	}
	name, err := shared.RequireText("contact_name", d.ContactName, 150)
	if err != nil {
		return nil, err
	}
	email, err := shared.NormalizeEmail("contact_email", d.ContactEmail)
	if err != nil {
		return nil, err
	}
	phone, err := optionalPhone("contact_phone", d.ContactPhone)
	if err != nil {
		return nil, err
	}
	return &ContactRequest{
		ID: id, DistributorID: d.DistributorID, DistributorName: d.DistributorName, ServiceID: d.ServiceID,
		RequesterCompanyID: d.RequesterCompanyID, RequesterCompanyName: d.RequesterCompanyName,
		RequesterUserID: d.RequesterUserID, ContactName: name, ContactEmail: email, ContactPhone: phone,
		Message: msg, Status: ContactRequestNew, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Handle moves the request through its states and/or updates the distributor notes.
func (r *ContactRequest) Handle(to *ContactRequestStatus, notes *string, now time.Time) error {
	if to == nil && notes == nil {
		return shared.Validation("status or notes is required")
	}
	if to != nil {
		if !to.Valid() {
			return shared.Validation("invalid contact request status %q", *to)
		}
		if !r.canTransition(*to) {
			return shared.Validation("cannot change contact request status from %s to %s", r.Status, *to)
		}
		r.Status = *to
	}
	if notes != nil {
		v, err := shared.OptionalText("notes", notes, 2000)
		if err != nil {
			return err
		}
		r.DistributorNotes = v
	}
	r.UpdatedAt = now
	return nil
}

func (r *ContactRequest) canTransition(to ContactRequestStatus) bool {
	for _, s := range contactRequestTransitions[r.Status] {
		if s == to {
			return true
		}
	}
	return false
}
