package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"global_360/internal/application/port"
	"global_360/internal/domain/identity"
)

// --- persons ---

type personDoc struct {
	ID                string    `bson:"_id"`
	DocumentType      string    `bson:"document_type"`
	DocumentNumber    string    `bson:"document_number"`
	DocumentCountryID string    `bson:"document_country_id"`
	FirstNames        string    `bson:"first_names"`
	LastNames         string    `bson:"last_names"`
	Email             string    `bson:"email"`
	Phone             *string   `bson:"phone"`
	CreatedAt         time.Time `bson:"created_at"`
	UpdatedAt         time.Time `bson:"updated_at"`
}

func toPersonDoc(p *identity.Person) personDoc {
	return personDoc{
		ID: p.ID, DocumentType: p.DocumentType, DocumentNumber: p.DocumentNumber, DocumentCountryID: p.DocumentCountryID,
		FirstNames: p.FirstNames, LastNames: p.LastNames, Email: p.Email, Phone: p.Phone, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (d *personDoc) toDomain() *identity.Person {
	return &identity.Person{
		ID: d.ID, DocumentType: d.DocumentType, DocumentNumber: d.DocumentNumber, DocumentCountryID: d.DocumentCountryID,
		FirstNames: d.FirstNames, LastNames: d.LastNames, Email: d.Email, Phone: d.Phone,
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type PersonRepository struct{ s *Store }

func NewPersonRepository(s *Store) *PersonRepository { return &PersonRepository{s: s} }

const personConflict = "a person with this document already exists"

func (r *PersonRepository) Create(ctx context.Context, p *identity.Person) error {
	return insertOne(ctx, r.s.coll(CollPersons), toPersonDoc(p), personConflict)
}

func (r *PersonRepository) Update(ctx context.Context, p *identity.Person) error {
	return replaceByID(ctx, r.s.coll(CollPersons), p.ID, toPersonDoc(p), personConflict, "person not found")
}

func (r *PersonRepository) FindByID(ctx context.Context, id string) (*identity.Person, error) {
	d, err := findOne[personDoc](ctx, r.s.coll(CollPersons), bson.M{"_id": id}, "person not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *PersonRepository) FindByDocument(ctx context.Context, documentType, documentNumber, countryID string) (*identity.Person, error) {
	filter := bson.M{"document_type": documentType, "document_number": documentNumber, "document_country_id": countryID}
	d, err := findOne[personDoc](ctx, r.s.coll(CollPersons), filter, "person not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

// --- users ---

type userDoc struct {
	ID                  string     `bson:"_id"`
	PersonID            string     `bson:"person_id"`
	CompanyID           *string    `bson:"company_id"`
	Email               string     `bson:"email"`
	PasswordHash        string     `bson:"password_hash"`
	Status              string     `bson:"status"`
	MFAEnabled          bool       `bson:"mfa_enabled"`
	FailedLoginAttempts int        `bson:"failed_login_attempts"`
	LastAccessAt        *time.Time `bson:"last_access_at"`
	CreatedAt           time.Time  `bson:"created_at"`
	UpdatedAt           time.Time  `bson:"updated_at"`
}

func toUserDoc(u *identity.User) userDoc {
	return userDoc{
		ID: u.ID, PersonID: u.PersonID, CompanyID: u.CompanyID, Email: u.Email, PasswordHash: u.PasswordHash,
		Status: string(u.Status), MFAEnabled: u.MFAEnabled, FailedLoginAttempts: u.FailedLoginAttempts,
		LastAccessAt: u.LastAccessAt, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

func (d *userDoc) toDomain() *identity.User {
	return &identity.User{
		ID: d.ID, PersonID: d.PersonID, CompanyID: d.CompanyID, Email: d.Email, PasswordHash: d.PasswordHash,
		Status: identity.UserStatus(d.Status), MFAEnabled: d.MFAEnabled, FailedLoginAttempts: d.FailedLoginAttempts,
		LastAccessAt: utcPtr(d.LastAccessAt), CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type UserRepository struct{ s *Store }

func NewUserRepository(s *Store) *UserRepository { return &UserRepository{s: s} }

const userConflict = "a user with this email already exists"

func (r *UserRepository) Create(ctx context.Context, u *identity.User) error {
	return insertOne(ctx, r.s.coll(CollUsers), toUserDoc(u), userConflict)
}

func (r *UserRepository) Update(ctx context.Context, u *identity.User) error {
	return replaceByID(ctx, r.s.coll(CollUsers), u.ID, toUserDoc(u), userConflict, "user not found")
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*identity.User, error) {
	d, err := findOne[userDoc](ctx, r.s.coll(CollUsers), bson.M{"_id": id}, "user not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*identity.User, error) {
	d, err := findOne[userDoc](ctx, r.s.coll(CollUsers), bson.M{"email": email}, "user not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *UserRepository) List(ctx context.Context, f port.UserFilter) ([]*identity.User, int64, error) {
	filter := bson.M{}
	if f.RestrictToCompanies {
		filter["company_id"] = bson.M{"$in": nonNil(f.CompanyIDs)}
	}
	if f.Status != nil {
		filter["status"] = string(*f.Status)
	}
	docs, total, err := findPage[userDoc](ctx, r.s.coll(CollUsers), filter, bson.D{{Key: "created_at", Value: -1}}, f.Page)
	if err != nil {
		return nil, 0, err
	}
	return mapAll(docs, (*userDoc).toDomain), total, nil
}

// --- refresh tokens ---

type refreshTokenDoc struct {
	ID        string     `bson:"_id"`
	UserID    string     `bson:"user_id"`
	TokenHash string     `bson:"token_hash"`
	ExpiresAt time.Time  `bson:"expires_at"`
	RevokedAt *time.Time `bson:"revoked_at"`
	CreatedAt time.Time  `bson:"created_at"`
	UserAgent string     `bson:"user_agent"`
	IP        string     `bson:"ip"`
}

func (d *refreshTokenDoc) toDomain() *identity.RefreshToken {
	return &identity.RefreshToken{
		ID: d.ID, UserID: d.UserID, TokenHash: d.TokenHash, ExpiresAt: d.ExpiresAt.UTC(), RevokedAt: utcPtr(d.RevokedAt),
		CreatedAt: d.CreatedAt.UTC(), UserAgent: d.UserAgent, IP: d.IP,
	}
}

type RefreshTokenRepository struct{ s *Store }

func NewRefreshTokenRepository(s *Store) *RefreshTokenRepository {
	return &RefreshTokenRepository{s: s}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *identity.RefreshToken) error {
	doc := refreshTokenDoc{
		ID: t.ID, UserID: t.UserID, TokenHash: t.TokenHash, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
		CreatedAt: t.CreatedAt, UserAgent: t.UserAgent, IP: t.IP,
	}
	return insertOne(ctx, r.s.coll(CollRefreshTokens), doc, "refresh token collision")
}

func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*identity.RefreshToken, error) {
	d, err := findOne[refreshTokenDoc](ctx, r.s.coll(CollRefreshTokens), bson.M{"token_hash": hash}, "refresh token not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *RefreshTokenRepository) MarkRevoked(ctx context.Context, id string, at time.Time) (bool, error) {
	res, err := r.s.coll(CollRefreshTokens).UpdateOne(ctx,
		bson.M{"_id": id, "revoked_at": nil},
		bson.M{"$set": bson.M{"revoked_at": at}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	_, err := r.s.coll(CollRefreshTokens).UpdateMany(ctx,
		bson.M{"user_id": userID, "revoked_at": nil},
		bson.M{"$set": bson.M{"revoked_at": at}})
	return err
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// nonNil guarantees an empty array instead of null in $in filters.
func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
