package service

import (
	"context"

	"global_360/internal/application/port"
	"global_360/internal/domain/access"
)

// AuthorizationService resolves the effective permissions of a user from the
// data model (role assignments + roles). Nothing is hardcoded in the interface.
type AuthorizationService struct {
	assignments port.RoleAssignmentRepository
	roles       port.RoleRepository
}

func NewAuthorizationService(assignments port.RoleAssignmentRepository, roles port.RoleRepository) *AuthorizationService {
	return &AuthorizationService{assignments: assignments, roles: roles}
}

// Grants loads the active assignments of userID and the roles they reference.
func (s *AuthorizationService) Grants(ctx context.Context, userID string) (*access.Grants, error) {
	assignments, roles, err := s.activeAssignments(ctx, userID)
	if err != nil {
		return nil, err
	}
	return access.BuildGrants(assignments, roles), nil
}

func (s *AuthorizationService) activeAssignments(ctx context.Context, userID string) ([]*access.RoleAssignment, []*access.Role, error) {
	assignments, err := s.assignments.ListActiveByUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if len(assignments) == 0 {
		return assignments, nil, nil
	}
	ids := make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.RoleID)
	}
	roles, err := s.roles.FindByIDs(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	return assignments, roles, nil
}
