package identity

import "time"

// RefreshToken is an opaque, rotating session token. Only its hash is stored.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UserAgent string
	IP        string
}

func (t *RefreshToken) IsRevoked() bool              { return t.RevokedAt != nil }
func (t *RefreshToken) IsExpired(now time.Time) bool { return !now.Before(t.ExpiresAt) }

func (t *RefreshToken) Revoke(now time.Time) {
	if t.RevokedAt == nil {
		t.RevokedAt = &now
	}
}
