//go:build integration

package mongodb_test

import (
	"context"

	"global_360/internal/application/port"
	"global_360/internal/application/service"
	"global_360/internal/domain/access"
	"global_360/internal/infrastructure/persistence/mongodb"
)

func accessGrants(ctx context.Context, store *mongodb.Store, userID string) (*access.Grants, error) {
	authz := service.NewAuthorizationService(mongodb.NewRoleAssignmentRepository(store), mongodb.NewRoleRepository(store))
	return authz.Grants(ctx, userID)
}

func portFilter(entity *string) port.AuditFilter {
	f := port.AuditFilter{Entity: entity}
	f.Page.Page, f.Page.PageSize = 1, 10
	return f
}
