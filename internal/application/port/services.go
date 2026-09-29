package port

import (
	"context"
	"time"
)

// TxManager runs fn atomically when the database supports transactions.
// Repositories must use the ctx received inside fn.
type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// PasswordHasher hashes and verifies passwords (bcrypt/argon2, never plain text).
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Compare(hash, plain string) bool
}

// AccessClaims is the identity carried by an access token.
type AccessClaims struct {
	UserID    string
	CompanyID *string
	TokenID   string
	ExpiresAt time.Time
}

// TokenIssuer issues and verifies short lived access tokens.
type TokenIssuer interface {
	IssueAccessToken(userID string, companyID *string) (token string, claims AccessClaims, err error)
	ParseAccessToken(token string) (AccessClaims, error)
}

// SecretGenerator creates opaque refresh tokens and their storable hash.
type SecretGenerator interface {
	NewSecret() (string, error)
	Hash(secret string) string
}

type Clock interface{ Now() time.Time }

type IDGenerator interface{ NewID() string }

// Pinger reports database health.
type Pinger interface {
	Ping(ctx context.Context) error
}
