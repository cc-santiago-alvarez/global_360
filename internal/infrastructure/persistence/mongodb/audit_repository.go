package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"global_360/internal/application/port"
	"global_360/internal/domain/audit"
)

type auditLogDoc struct {
	ID        bson.ObjectID `bson:"_id"`
	UserID    *string       `bson:"user_id"`
	Entity    string        `bson:"entity"`
	EntityID  string        `bson:"entity_id"`
	Action    string        `bson:"action"`
	OldValues bson.M        `bson:"old_values"`
	NewValues bson.M        `bson:"new_values"`
	SourceIP  *string       `bson:"source_ip"`
	UserAgent *string       `bson:"user_agent"`
	CreatedAt time.Time     `bson:"created_at"`
}

func (d *auditLogDoc) toDomain() *audit.Log {
	return &audit.Log{
		ID: d.ID.Hex(), UserID: d.UserID, Entity: d.Entity, EntityID: d.EntityID, Action: audit.Action(d.Action),
		OldValues: d.OldValues, NewValues: d.NewValues, SourceIP: d.SourceIP, UserAgent: d.UserAgent, CreatedAt: d.CreatedAt.UTC(),
	}
}

// AuditLogRepository is append-only: there is no update or delete.
type AuditLogRepository struct{ s *Store }

func NewAuditLogRepository(s *Store) *AuditLogRepository { return &AuditLogRepository{s: s} }

func (r *AuditLogRepository) Create(ctx context.Context, l *audit.Log) error {
	id := bson.NewObjectID()
	doc := auditLogDoc{
		ID: id, UserID: l.UserID, Entity: l.Entity, EntityID: l.EntityID, Action: string(l.Action),
		OldValues: l.OldValues, NewValues: l.NewValues, SourceIP: l.SourceIP, UserAgent: l.UserAgent, CreatedAt: l.CreatedAt,
	}
	if err := insertOne(ctx, r.s.coll(CollAuditLogs), doc, "audit log collision"); err != nil {
		return err
	}
	l.ID = id.Hex()
	return nil
}

func (r *AuditLogRepository) List(ctx context.Context, f port.AuditFilter) ([]*audit.Log, int64, error) {
	filter := bson.M{}
	if f.Entity != nil {
		filter["entity"] = *f.Entity
	}
	if f.EntityID != nil {
		filter["entity_id"] = *f.EntityID
	}
	if f.UserID != nil {
		filter["user_id"] = *f.UserID
	}
	if f.Action != nil {
		filter["action"] = string(*f.Action)
	}
	if f.From != nil || f.To != nil {
		rng := bson.M{}
		if f.From != nil {
			rng["$gte"] = *f.From
		}
		if f.To != nil {
			rng["$lte"] = *f.To
		}
		filter["created_at"] = rng
	}
	docs, total, err := findPage[auditLogDoc](ctx, r.s.coll(CollAuditLogs), filter,
		bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}, f.Page)
	if err != nil {
		return nil, 0, err
	}
	return mapAll(docs, (*auditLogDoc).toDomain), total, nil
}

// compile-time interface checks
var (
	_ port.CountryRepository        = (*CountryRepository)(nil)
	_ port.CurrencyRepository       = (*CurrencyRepository)(nil)
	_ port.PersonRepository         = (*PersonRepository)(nil)
	_ port.UserRepository           = (*UserRepository)(nil)
	_ port.RefreshTokenRepository   = (*RefreshTokenRepository)(nil)
	_ port.CompanyRepository        = (*CompanyRepository)(nil)
	_ port.PermissionRepository     = (*PermissionRepository)(nil)
	_ port.RoleRepository           = (*RoleRepository)(nil)
	_ port.RoleAssignmentRepository = (*RoleAssignmentRepository)(nil)
	_ port.AuditLogRepository       = (*AuditLogRepository)(nil)
	_ port.TxManager                = (*TxManager)(nil)
	_ port.Pinger                   = (*Store)(nil)
)
