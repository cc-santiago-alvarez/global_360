// Package service implements the phase 1 use cases (driving ports).
package service

import (
	"context"
	"encoding/json"
	"errors"

	"global_360/internal/application/actor"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/shared"
)

// Deps groups the collaborators shared by every service.
type Deps struct {
	Tx    port.TxManager
	Clock port.Clock
	IDs   port.IDGenerator
	Audit *AuditService
}

// currentActor returns the authenticated actor or an unauthorized error.
func currentActor(ctx context.Context) (actor.Actor, error) {
	a := actor.FromContext(ctx)
	if !a.Authenticated() {
		return a, shared.Unauthorized("authentication required")
	}
	return a, nil
}

// authorize checks code globally or within companyID.
func authorize(ctx context.Context, code string, companyID *string) (actor.Actor, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return a, err
	}
	if !a.Grants.Can(code, companyID) {
		return a, shared.Forbidden("missing permission %s", code)
	}
	return a, nil
}

// authorizeGlobal checks code without company restriction.
func authorizeGlobal(ctx context.Context, code string) (actor.Actor, error) {
	return authorize(ctx, code, nil)
}

// authorizeAny checks code is granted globally or in at least one company.
func authorizeAny(ctx context.Context, code string) (actor.Actor, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return a, err
	}
	if !a.Grants.HasAny(code) {
		return a, shared.Forbidden("missing permission %s", code)
	}
	return a, nil
}

// companyScope resolves which companies the actor may see for code.
// all=true means no restriction.
func companyScope(g *access.Grants, code string) (all bool, ids []string) {
	if g.CanGlobally(code) {
		return true, nil
	}
	return false, g.CompaniesWith(code)
}

// refErr converts a not-found error on a referenced entity into a validation error.
func refErr(err error, field string) error {
	if errors.Is(err, shared.ErrNotFound) {
		return shared.Validation("%s does not exist", field)
	}
	return err
}

// snapshot converts a result DTO into a generic document for the audit trail.
func snapshot(v any) map[string]any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}
