package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"global_360/internal/application/actor"
	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

type AuthConfig struct {
	MaxFailedLoginAttempts int
	RefreshTokenTTL        time.Duration
}

// AuthService handles login, token rotation, logout and request authentication.
type AuthService struct {
	users   port.UserRepository
	persons port.PersonRepository
	tokens  port.RefreshTokenRepository
	roles   port.RoleRepository
	hasher  port.PasswordHasher
	issuer  port.TokenIssuer
	secrets port.SecretGenerator
	authz   *AuthorizationService
	d       Deps
	cfg     AuthConfig
	// dummyHash is compared when the email does not exist, so response time
	// does not reveal which emails are registered.
	dummyHash string
}

func NewAuthService(
	users port.UserRepository, persons port.PersonRepository, tokens port.RefreshTokenRepository,
	roles port.RoleRepository, hasher port.PasswordHasher, issuer port.TokenIssuer, secrets port.SecretGenerator,
	authz *AuthorizationService, d Deps, cfg AuthConfig,
) (*AuthService, error) {
	dummy, err := hasher.Hash("global360-timing-equalizer")
	if err != nil {
		return nil, err
	}
	return &AuthService{
		users: users, persons: persons, tokens: tokens, roles: roles, hasher: hasher, issuer: issuer,
		secrets: secrets, authz: authz, d: d, cfg: cfg, dummyHash: dummy,
	}, nil
}

var errInvalidCredentials = shared.Unauthorized("invalid email or password")
var errInvalidRefreshToken = shared.Unauthorized("invalid or expired refresh token")

// Login verifies credentials and returns a new token pair.
func (s *AuthService) Login(ctx context.Context, email, password string) (dto.TokenPair, error) {
	u, err := s.users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, shared.ErrNotFound) {
		s.hasher.Compare(s.dummyHash, password)
		return dto.TokenPair{}, errInvalidCredentials
	}
	if err != nil {
		return dto.TokenPair{}, err
	}

	now := s.d.Clock.Now()
	if !s.hasher.Compare(u.PasswordHash, password) {
		if u.Status == identity.UserActive {
			before := dto.NewUser(u, nil)
			blocked := u.RegisterFailedLogin(s.cfg.MaxFailedLoginAttempts, now)
			err := s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
				if err := s.users.Update(ctx, u); err != nil {
					return err
				}
				if blocked {
					if err := s.tokens.RevokeAllForUser(ctx, u.ID, now); err != nil {
						return err
					}
					return s.d.Audit.Record(ctx, audit.EntityUser, u.ID, audit.ActionUpdate, before, dto.NewUser(u, nil))
				}
				return nil
			})
			if err != nil {
				return dto.TokenPair{}, err
			}
		}
		return dto.TokenPair{}, errInvalidCredentials
	}
	if !u.CanLogin() {
		return dto.TokenPair{}, shared.Forbidden("user account is %s", u.Status)
	}

	u.RegisterSuccessfulLogin(now)
	ctx = asUser(ctx, u)
	pair, token, err := s.issueTokens(ctx, u, now)
	if err != nil {
		return dto.TokenPair{}, err
	}
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.users.Update(ctx, u); err != nil {
			return err
		}
		if err := s.tokens.Create(ctx, token); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityUser, u.ID, audit.ActionLogin, nil, map[string]any{"email": u.Email})
	})
	if err != nil {
		return dto.TokenPair{}, err
	}
	return pair, nil
}

// Refresh rotates a refresh token. Reusing a revoked token revokes every session of the user.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (dto.TokenPair, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return dto.TokenPair{}, errInvalidRefreshToken
	}
	now := s.d.Clock.Now()
	t, err := s.tokens.FindByHash(ctx, s.secrets.Hash(refreshToken))
	if errors.Is(err, shared.ErrNotFound) {
		return dto.TokenPair{}, errInvalidRefreshToken
	}
	if err != nil {
		return dto.TokenPair{}, err
	}
	if t.IsRevoked() {
		if err := s.tokens.RevokeAllForUser(ctx, t.UserID, now); err != nil {
			return dto.TokenPair{}, err
		}
		return dto.TokenPair{}, errInvalidRefreshToken
	}
	if t.IsExpired(now) {
		return dto.TokenPair{}, errInvalidRefreshToken
	}
	u, err := s.users.FindByID(ctx, t.UserID)
	if err != nil {
		return dto.TokenPair{}, err
	}
	if !u.CanLogin() {
		_, _ = s.tokens.MarkRevoked(ctx, t.ID, now)
		return dto.TokenPair{}, errInvalidRefreshToken
	}

	pair, next, err := s.issueTokens(ctx, u, now)
	if err != nil {
		return dto.TokenPair{}, err
	}
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		revoked, err := s.tokens.MarkRevoked(ctx, t.ID, now)
		if err != nil {
			return err
		}
		if !revoked { // a concurrent request already rotated it
			return errInvalidRefreshToken
		}
		return s.tokens.Create(ctx, next)
	})
	if err != nil {
		return dto.TokenPair{}, err
	}
	return pair, nil
}

// Logout revokes the refresh token of the authenticated user.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	a, err := currentActor(ctx)
	if err != nil {
		return err
	}
	t, err := s.tokens.FindByHash(ctx, s.secrets.Hash(refreshToken))
	if errors.Is(err, shared.ErrNotFound) {
		return errInvalidRefreshToken
	}
	if err != nil {
		return err
	}
	if t.UserID != a.UserID {
		return errInvalidRefreshToken
	}
	now := s.d.Clock.Now()
	return s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if _, err := s.tokens.MarkRevoked(ctx, t.ID, now); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityUser, a.UserID, audit.ActionLogout, nil, nil)
	})
}

// Authenticate validates an access token and resolves the caller with its current
// permissions. Users that are no longer active are rejected immediately.
func (s *AuthService) Authenticate(ctx context.Context, accessToken string) (actor.Actor, error) {
	claims, err := s.issuer.ParseAccessToken(accessToken)
	if err != nil {
		return actor.Actor{}, shared.Unauthorized("invalid or expired access token")
	}
	u, err := s.users.FindByID(ctx, claims.UserID)
	if errors.Is(err, shared.ErrNotFound) {
		return actor.Actor{}, shared.Unauthorized("invalid or expired access token")
	}
	if err != nil {
		return actor.Actor{}, err
	}
	if !u.CanLogin() {
		return actor.Actor{}, shared.Unauthorized("user account is %s", u.Status)
	}
	grants, err := s.authz.Grants(ctx, u.ID)
	if err != nil {
		return actor.Actor{}, err
	}
	a := actor.FromContext(ctx)
	a.UserID, a.CompanyID, a.Grants = u.ID, u.CompanyID, grants
	return a, nil
}

// Me returns the authenticated user with its role assignments and effective permissions.
func (s *AuthService) Me(ctx context.Context) (dto.Me, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return dto.Me{}, err
	}
	u, err := s.users.FindByID(ctx, a.UserID)
	if err != nil {
		return dto.Me{}, err
	}
	p, err := s.persons.FindByID(ctx, u.PersonID)
	if err != nil {
		return dto.Me{}, err
	}
	assignments, roles, err := s.authz.activeAssignments(ctx, u.ID)
	if err != nil {
		return dto.Me{}, err
	}
	return dto.Me{
		User:               dto.NewUser(u, p),
		RoleAssignments:    assignmentResults(assignments, roles),
		GlobalPermissions:  a.Grants.GlobalPermissions(),
		CompanyPermissions: a.Grants.CompanyPermissions(),
	}, nil
}

func (s *AuthService) issueTokens(ctx context.Context, u *identity.User, now time.Time) (dto.TokenPair, *identity.RefreshToken, error) {
	access, claims, err := s.issuer.IssueAccessToken(u.ID, u.CompanyID)
	if err != nil {
		return dto.TokenPair{}, nil, err
	}
	secret, err := s.secrets.NewSecret()
	if err != nil {
		return dto.TokenPair{}, nil, err
	}
	a := actor.FromContext(ctx)
	token := &identity.RefreshToken{
		ID:        s.d.IDs.NewID(),
		UserID:    u.ID,
		TokenHash: s.secrets.Hash(secret),
		ExpiresAt: now.Add(s.cfg.RefreshTokenTTL),
		CreatedAt: now,
		UserAgent: truncate(a.UserAgent, 255),
		IP:        a.IP,
	}
	return dto.TokenPair{
		TokenType:             "Bearer",
		AccessToken:           access,
		AccessTokenExpiresAt:  claims.ExpiresAt,
		RefreshToken:          secret,
		RefreshTokenExpiresAt: token.ExpiresAt,
	}, token, nil
}

// asUser returns a context whose actor is u (used to audit login as the user).
func asUser(ctx context.Context, u *identity.User) context.Context {
	a := actor.FromContext(ctx)
	a.UserID, a.CompanyID = u.ID, u.CompanyID
	return actor.WithActor(ctx, a)
}
