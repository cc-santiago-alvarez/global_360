// Package security provides the password hashing, token and id adapters.
package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"global_360/internal/application/port"
)

// BcryptHasher implements port.PasswordHasher.
type BcryptHasher struct{ cost int }

func NewBcryptHasher(cost int) *BcryptHasher { return &BcryptHasher{cost: cost} }

func (h *BcryptHasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

func (h *BcryptHasher) Compare(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// JWTIssuer implements port.TokenIssuer with HS256 access tokens.
type JWTIssuer struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

func NewJWTIssuer(secret, issuer string, ttl time.Duration) *JWTIssuer {
	return &JWTIssuer{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}
}

type accessClaims struct {
	CompanyID *string `json:"company_id"`
	jwt.RegisteredClaims
}

func (j *JWTIssuer) IssueAccessToken(userID string, companyID *string) (string, port.AccessClaims, error) {
	now := j.now().UTC()
	exp := now.Add(j.ttl)
	jti := uuid.NewString()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{
		CompanyID: companyID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", port.AccessClaims{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, port.AccessClaims{UserID: userID, CompanyID: companyID, TokenID: jti, ExpiresAt: exp}, nil
}

func (j *JWTIssuer) ParseAccessToken(raw string) (port.AccessClaims, error) {
	var c accessClaims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(j.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(j.now),
	)
	if err != nil {
		return port.AccessClaims{}, err
	}
	if c.Subject == "" {
		return port.AccessClaims{}, errors.New("token without subject")
	}
	return port.AccessClaims{UserID: c.Subject, CompanyID: c.CompanyID, TokenID: c.ID, ExpiresAt: c.ExpiresAt.Time}, nil
}

// RandomSecrets implements port.SecretGenerator: 32 random bytes, stored as SHA-256.
type RandomSecrets struct{}

func (RandomSecrets) NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (RandomSecrets) Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// UUIDGenerator implements port.IDGenerator with UUID v4 strings.
type UUIDGenerator struct{}

func (UUIDGenerator) NewID() string { return uuid.NewString() }

// SystemClock implements port.Clock in UTC truncated to milliseconds (MongoDB precision).
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
