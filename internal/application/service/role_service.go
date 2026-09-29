package service

import (
	"context"
	"strings"

	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/shared"
)

// RoleService manages roles and exposes the permission catalog.
type RoleService struct {
	roles       port.RoleRepository
	permissions port.PermissionRepository
	d           Deps
}

func NewRoleService(roles port.RoleRepository, permissions port.PermissionRepository, d Deps) *RoleService {
	return &RoleService{roles: roles, permissions: permissions, d: d}
}

func (s *RoleService) ListPermissions(ctx context.Context) ([]dto.Permission, error) {
	if _, err := authorizeAny(ctx, access.PermRoleRead); err != nil {
		return nil, err
	}
	perms, err := s.permissions.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Permission, 0, len(perms))
	for _, p := range perms {
		out = append(out, dto.NewPermission(p))
	}
	return out, nil
}

func (s *RoleService) List(ctx context.Context) ([]dto.Role, error) {
	if _, err := authorizeAny(ctx, access.PermRoleRead); err != nil {
		return nil, err
	}
	roles, err := s.roles.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.Role, 0, len(roles))
	for _, r := range roles {
		out = append(out, dto.NewRole(r))
	}
	return out, nil
}

func (s *RoleService) Get(ctx context.Context, id string) (dto.Role, error) {
	if _, err := authorizeAny(ctx, access.PermRoleRead); err != nil {
		return dto.Role{}, err
	}
	r, err := s.roles.FindByID(ctx, id)
	if err != nil {
		return dto.Role{}, err
	}
	return dto.NewRole(r), nil
}

// Create adds a custom (non system) role. Requires global role.manage.
func (s *RoleService) Create(ctx context.Context, cmd dto.CreateRole) (dto.Role, error) {
	if _, err := authorizeGlobal(ctx, access.PermRoleManage); err != nil {
		return dto.Role{}, err
	}
	r, err := access.NewRole(s.d.IDs.NewID(), cmd.Name, cmd.Description, cmd.Scope, false, cmd.Permissions, s.d.Clock.Now())
	if err != nil {
		return dto.Role{}, err
	}
	if err := s.ensurePermissionsExist(ctx, r.Permissions); err != nil {
		return dto.Role{}, err
	}
	result := dto.NewRole(r)
	err = s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.roles.Create(ctx, r); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityRole, r.ID, audit.ActionCreate, nil, result)
	})
	return result, err
}

// Update renames, describes or (de)activates a role.
func (s *RoleService) Update(ctx context.Context, id string, cmd dto.UpdateRole) (dto.Role, error) {
	if _, err := authorizeGlobal(ctx, access.PermRoleManage); err != nil {
		return dto.Role{}, err
	}
	r, err := s.roles.FindByID(ctx, id)
	if err != nil {
		return dto.Role{}, err
	}
	before := dto.NewRole(r)
	now := s.d.Clock.Now()
	if cmd.Name != nil && strings.ToLower(strings.TrimSpace(*cmd.Name)) != r.Name {
		if err := r.Rename(*cmd.Name, now); err != nil {
			return dto.Role{}, err
		}
	}
	if cmd.Description != nil {
		if err := r.SetDescription(cmd.Description, now); err != nil {
			return dto.Role{}, err
		}
	}
	if cmd.Active != nil {
		if err := r.SetActive(*cmd.Active, now); err != nil {
			return dto.Role{}, err
		}
	}
	return s.save(ctx, r, before)
}

// ReplacePermissions sets the full list of permission codes of a role.
func (s *RoleService) ReplacePermissions(ctx context.Context, id string, codes []string) (dto.Role, error) {
	if _, err := authorizeGlobal(ctx, access.PermRoleManage); err != nil {
		return dto.Role{}, err
	}
	r, err := s.roles.FindByID(ctx, id)
	if err != nil {
		return dto.Role{}, err
	}
	before := dto.NewRole(r)
	if err := r.ReplacePermissions(codes, s.d.Clock.Now()); err != nil {
		return dto.Role{}, err
	}
	if err := s.ensurePermissionsExist(ctx, r.Permissions); err != nil {
		return dto.Role{}, err
	}
	return s.save(ctx, r, before)
}

func (s *RoleService) save(ctx context.Context, r *access.Role, before dto.Role) (dto.Role, error) {
	after := dto.NewRole(r)
	err := s.d.Tx.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.roles.Update(ctx, r); err != nil {
			return err
		}
		return s.d.Audit.Record(ctx, audit.EntityRole, r.ID, audit.ActionUpdate, before, after)
	})
	if err != nil {
		return dto.Role{}, err
	}
	return after, nil
}

// ensurePermissionsExist validates codes against the permission catalog.
func (s *RoleService) ensurePermissionsExist(ctx context.Context, codes []string) error {
	if len(codes) == 0 {
		return nil
	}
	found, err := s.permissions.FindByCodes(ctx, codes)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(found))
	for _, p := range found {
		known[p.Code] = true
	}
	var unknown []string
	for _, c := range codes {
		if !known[c] {
			unknown = append(unknown, c)
		}
	}
	if len(unknown) > 0 {
		return shared.Validation("unknown permissions: %s", strings.Join(unknown, ", "))
	}
	return nil
}
