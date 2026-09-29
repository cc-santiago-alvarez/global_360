// Package audit models the generic audit trail: who did what, when, on which entity.
package audit

import "time"

// Action is the kind of audited operation.
type Action string

const (
	ActionCreate  Action = "create"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionApprove Action = "approve"
	ActionReject  Action = "reject"
	ActionLogin   Action = "login"
	ActionLogout  Action = "logout"
)

var Actions = []Action{ActionCreate, ActionUpdate, ActionDelete, ActionApprove, ActionReject, ActionLogin, ActionLogout}

func (a Action) Valid() bool {
	for _, v := range Actions {
		if a == v {
			return true
		}
	}
	return false
}

// Entity names used in audit records. New phases add their own names without schema changes.
const (
	EntityCountry        = "country"
	EntityCurrency       = "currency"
	EntityPerson         = "person"
	EntityCompany        = "company"
	EntityUser           = "user"
	EntityRole           = "role"
	EntityRoleAssignment = "role_assignment"

	EntityCategory           = "category"
	EntityDistributorProfile = "distributor_profile"
	EntityDistributorService = "distributor_service"
	EntityContactRequest     = "contact_request"
)

// Log is one audit record. UserID is nil when the action was performed by the system.
type Log struct {
	ID        string
	UserID    *string
	Entity    string
	EntityID  string
	Action    Action
	OldValues map[string]any
	NewValues map[string]any
	SourceIP  *string
	UserAgent *string
	CreatedAt time.Time
}
