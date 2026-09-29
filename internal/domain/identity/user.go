package identity

import (
	"time"
	"unicode/utf8"

	"global_360/internal/domain/shared"
)

// UserStatus is the lifecycle state of a user.
type UserStatus string

const (
	UserPendingVerification UserStatus = "pending_verification"
	UserActive              UserStatus = "active"
	UserInactive            UserStatus = "inactive"
	UserBlocked             UserStatus = "blocked"
)

// UserStatuses lists every valid status (also used by the database validator).
var UserStatuses = []UserStatus{UserPendingVerification, UserActive, UserInactive, UserBlocked}

func (s UserStatus) Valid() bool {
	for _, v := range UserStatuses {
		if s == v {
			return true
		}
	}
	return false
}

const (
	MinPasswordLength = 10
	MaxPasswordLength = 72 // bcrypt limit
)

// ValidatePassword enforces the minimum password policy.
func ValidatePassword(plain string) error {
	n := utf8.RuneCountInString(plain)
	if n < MinPasswordLength {
		return shared.Validation("password must have at least %d characters", MinPasswordLength)
	}
	if len(plain) > MaxPasswordLength {
		return shared.Validation("password must have at most %d bytes", MaxPasswordLength)
	}
	return nil
}

// User holds login credentials. CompanyID is nil for Global 360 internal staff.
type User struct {
	ID                  string
	PersonID            string
	CompanyID           *string
	Email               string
	PasswordHash        string
	Status              UserStatus
	MFAEnabled          bool
	FailedLoginAttempts int
	LastAccessAt        *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func NewUser(id, personID string, companyID *string, email, passwordHash string, now time.Time) (*User, error) {
	e, err := shared.NormalizeEmail("email", email)
	if err != nil {
		return nil, err
	}
	if personID == "" {
		return nil, shared.Validation("person_id is required")
	}
	if passwordHash == "" {
		return nil, shared.Validation("password is required")
	}
	return &User{
		ID:           id,
		PersonID:     personID,
		CompanyID:    companyID,
		Email:        e,
		PasswordHash: passwordHash,
		Status:       UserPendingVerification,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// CanLogin reports whether the user is allowed to authenticate.
func (u *User) CanLogin() bool { return u.Status == UserActive }

// RegisterFailedLogin increments the counter and blocks the user when max is reached.
// It returns true when this call blocked the user.
func (u *User) RegisterFailedLogin(max int, now time.Time) bool {
	u.FailedLoginAttempts++
	u.UpdatedAt = now
	if max > 0 && u.FailedLoginAttempts >= max && u.Status == UserActive {
		u.Status = UserBlocked
		return true
	}
	return false
}

// RegisterSuccessfulLogin resets the failure counter and records the access time.
func (u *User) RegisterSuccessfulLogin(now time.Time) {
	u.FailedLoginAttempts = 0
	u.LastAccessAt = &now
	u.UpdatedAt = now
}

// ChangeStatus moves the user to a new status. Activating resets failed attempts.
func (u *User) ChangeStatus(status UserStatus, now time.Time) error {
	if !status.Valid() {
		return shared.Validation("invalid user status %q", status)
	}
	if status == u.Status {
		return shared.Validation("user is already %s", status)
	}
	u.Status = status
	if status == UserActive {
		u.FailedLoginAttempts = 0
	}
	u.UpdatedAt = now
	return nil
}

func (u *User) ChangeEmail(email string, now time.Time) error {
	e, err := shared.NormalizeEmail("email", email)
	if err != nil {
		return err
	}
	u.Email = e
	u.UpdatedAt = now
	return nil
}

func (u *User) ChangePasswordHash(hash string, now time.Time) {
	u.PasswordHash = hash
	u.UpdatedAt = now
}
