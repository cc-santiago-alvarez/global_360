package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"global_360/internal/domain/access"
)

// --- permissions ---

type permissionDoc struct {
	ID          string `bson:"_id"`
	Code        string `bson:"code"`
	Module      string `bson:"module"`
	Description string `bson:"description"`
}

func (d *permissionDoc) toDomain() *access.Permission {
	return &access.Permission{ID: d.ID, Code: d.Code, Module: d.Module, Description: d.Description}
}

type PermissionRepository struct{ s *Store }

func NewPermissionRepository(s *Store) *PermissionRepository { return &PermissionRepository{s: s} }

func (r *PermissionRepository) List(ctx context.Context) ([]*access.Permission, error) {
	docs, err := findMany[permissionDoc](ctx, r.s.coll(CollPermissions), bson.M{},
		options.Find().SetSort(bson.D{{Key: "module", Value: 1}, {Key: "code", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*permissionDoc).toDomain), nil
}

func (r *PermissionRepository) FindByCodes(ctx context.Context, codes []string) ([]*access.Permission, error) {
	docs, err := findMany[permissionDoc](ctx, r.s.coll(CollPermissions), bson.M{"code": bson.M{"$in": nonNil(codes)}})
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*permissionDoc).toDomain), nil
}

// --- roles ---

type roleDoc struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	Description *string   `bson:"description"`
	Scope       string    `bson:"scope"`
	IsSystem    bool      `bson:"is_system"`
	Active      bool      `bson:"active"`
	Permissions []string  `bson:"permissions"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

func toRoleDoc(r *access.Role) roleDoc {
	return roleDoc{
		ID: r.ID, Name: r.Name, Description: r.Description, Scope: string(r.Scope), IsSystem: r.IsSystem,
		Active: r.Active, Permissions: nonNil(r.Permissions), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (d *roleDoc) toDomain() *access.Role {
	return &access.Role{
		ID: d.ID, Name: d.Name, Description: d.Description, Scope: access.Scope(d.Scope), IsSystem: d.IsSystem,
		Active: d.Active, Permissions: d.Permissions, CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
}

type RoleRepository struct{ s *Store }

func NewRoleRepository(s *Store) *RoleRepository { return &RoleRepository{s: s} }

const roleConflict = "a role with this name already exists"

func (r *RoleRepository) Create(ctx context.Context, role *access.Role) error {
	return insertOne(ctx, r.s.coll(CollRoles), toRoleDoc(role), roleConflict)
}

func (r *RoleRepository) Update(ctx context.Context, role *access.Role) error {
	return replaceByID(ctx, r.s.coll(CollRoles), role.ID, toRoleDoc(role), roleConflict, "role not found")
}

func (r *RoleRepository) FindByID(ctx context.Context, id string) (*access.Role, error) {
	d, err := findOne[roleDoc](ctx, r.s.coll(CollRoles), bson.M{"_id": id}, "role not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *RoleRepository) FindByName(ctx context.Context, name string) (*access.Role, error) {
	d, err := findOne[roleDoc](ctx, r.s.coll(CollRoles), bson.M{"name": name}, "role not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *RoleRepository) FindByIDs(ctx context.Context, ids []string) ([]*access.Role, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	docs, err := findMany[roleDoc](ctx, r.s.coll(CollRoles), bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*roleDoc).toDomain), nil
}

func (r *RoleRepository) List(ctx context.Context) ([]*access.Role, error) {
	docs, err := findMany[roleDoc](ctx, r.s.coll(CollRoles), bson.M{},
		options.Find().SetSort(bson.D{{Key: "scope", Value: 1}, {Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*roleDoc).toDomain), nil
}

// --- role assignments ---

type roleAssignmentDoc struct {
	ID         string     `bson:"_id"`
	UserID     string     `bson:"user_id"`
	RoleID     string     `bson:"role_id"`
	CompanyID  *string    `bson:"company_id"`
	Active     bool       `bson:"active"`
	AssignedAt time.Time  `bson:"assigned_at"`
	AssignedBy *string    `bson:"assigned_by"`
	RevokedAt  *time.Time `bson:"revoked_at"`
	RevokedBy  *string    `bson:"revoked_by"`
}

func toRoleAssignmentDoc(a *access.RoleAssignment) roleAssignmentDoc {
	return roleAssignmentDoc{
		ID: a.ID, UserID: a.UserID, RoleID: a.RoleID, CompanyID: a.CompanyID, Active: a.Active,
		AssignedAt: a.AssignedAt, AssignedBy: a.AssignedBy, RevokedAt: a.RevokedAt, RevokedBy: a.RevokedBy,
	}
}

func (d *roleAssignmentDoc) toDomain() *access.RoleAssignment {
	return &access.RoleAssignment{
		ID: d.ID, UserID: d.UserID, RoleID: d.RoleID, CompanyID: d.CompanyID, Active: d.Active,
		AssignedAt: d.AssignedAt.UTC(), AssignedBy: d.AssignedBy, RevokedAt: utcPtr(d.RevokedAt), RevokedBy: d.RevokedBy,
	}
}

type RoleAssignmentRepository struct{ s *Store }

func NewRoleAssignmentRepository(s *Store) *RoleAssignmentRepository {
	return &RoleAssignmentRepository{s: s}
}

const assignmentConflict = "the user already has this role in this scope"

func (r *RoleAssignmentRepository) Create(ctx context.Context, a *access.RoleAssignment) error {
	return insertOne(ctx, r.s.coll(CollRoleAssignments), toRoleAssignmentDoc(a), assignmentConflict)
}

func (r *RoleAssignmentRepository) Update(ctx context.Context, a *access.RoleAssignment) error {
	return replaceByID(ctx, r.s.coll(CollRoleAssignments), a.ID, toRoleAssignmentDoc(a), assignmentConflict, "role assignment not found")
}

func (r *RoleAssignmentRepository) FindByID(ctx context.Context, id string) (*access.RoleAssignment, error) {
	d, err := findOne[roleAssignmentDoc](ctx, r.s.coll(CollRoleAssignments), bson.M{"_id": id}, "role assignment not found")
	if err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r *RoleAssignmentRepository) ListActiveByUser(ctx context.Context, userID string) ([]*access.RoleAssignment, error) {
	docs, err := findMany[roleAssignmentDoc](ctx, r.s.coll(CollRoleAssignments), bson.M{"user_id": userID, "active": true},
		options.Find().SetSort(bson.D{{Key: "assigned_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	return mapAll(docs, (*roleAssignmentDoc).toDomain), nil
}
