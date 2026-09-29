package service

import (
	"context"

	"global_360/internal/application/actor"
	"global_360/internal/application/dto"
	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/shared"
)

// AuditService records and queries the audit trail.
type AuditService struct {
	repo  port.AuditLogRepository
	clock port.Clock
}

func NewAuditService(repo port.AuditLogRepository, clock port.Clock) *AuditService {
	return &AuditService{repo: repo, clock: clock}
}

// Record stores an audit entry for the actor found in ctx. before/after are result
// DTOs (or nil) and are stored as generic snapshots.
func (s *AuditService) Record(ctx context.Context, entity, entityID string, action audit.Action, before, after any) error {
	a := actor.FromContext(ctx)
	l := &audit.Log{
		UserID:    a.UserIDPtr(),
		Entity:    entity,
		EntityID:  entityID,
		Action:    action,
		OldValues: snapshot(before),
		NewValues: snapshot(after),
		SourceIP:  nonEmpty(a.IP),
		UserAgent: nonEmpty(truncate(a.UserAgent, 255)),
		CreatedAt: s.clock.Now(),
	}
	return s.repo.Create(ctx, l)
}

// List returns audit entries. Requires the global audit.read permission.
func (s *AuditService) List(ctx context.Context, q dto.ListAuditLogs) (dto.Page[dto.AuditLog], error) {
	if _, err := authorizeGlobal(ctx, access.PermAuditRead); err != nil {
		return dto.Page[dto.AuditLog]{}, err
	}
	if q.Action != nil && !q.Action.Valid() {
		return dto.Page[dto.AuditLog]{}, shared.Validation("invalid action %q", *q.Action)
	}
	page := q.Page.Normalize()
	logs, total, err := s.repo.List(ctx, port.AuditFilter{
		Entity: q.Entity, EntityID: q.EntityID, UserID: q.UserID, Action: q.Action, From: q.From, To: q.To, Page: page,
	})
	if err != nil {
		return dto.Page[dto.AuditLog]{}, err
	}
	items := make([]dto.AuditLog, 0, len(logs))
	for _, l := range logs {
		items = append(items, dto.NewAuditLog(l))
	}
	return dto.Page[dto.AuditLog]{Items: items, Total: total, Page: page.Page, PageSize: page.PageSize}, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
