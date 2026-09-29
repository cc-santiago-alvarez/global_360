//go:build integration

// Integration tests against a real MongoDB. They use MONGO_URI (or the repository
// .env) and a throwaway database that is dropped at the end.
//
//	go test -tags integration ./internal/infrastructure/persistence/mongodb/...
package mongodb_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"

	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
	"global_360/internal/infrastructure/persistence/mongodb"
	"global_360/internal/infrastructure/security"
	"global_360/internal/testutil/memory"
)

func setup(t *testing.T) (*mongodb.Store, context.Context) {
	t.Helper()
	_ = godotenv.Load("../../../../.env")
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	dbName := fmt.Sprintf("global360_test_%d", time.Now().UnixNano())
	store, err := mongodb.Connect(ctx, uri, dbName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.DB.Drop(context.Background())
		_ = store.Close(context.Background())
	})
	t.Logf("database %s (transactions supported: %v)", dbName, store.SupportsTransactions)
	if err := mongodb.EnsureSchema(ctx, store); err != nil {
		t.Fatal(err)
	}
	return store, ctx
}

func TestSchemaAndSeedAreIdempotent(t *testing.T) {
	store, ctx := setup(t)
	if err := mongodb.EnsureSchema(ctx, store); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seeder := mongodb.NewSeeder(store, memory.Hasher{}, security.UUIDGenerator{}, security.SystemClock{}, log)
	cfg := mongodb.SeedConfig{SuperadminEmail: "root@global360.com", SuperadminPassword: "a-very-long-password"}
	for i := 0; i < 2; i++ {
		if err := seeder.Run(ctx, cfg); err != nil {
			t.Fatalf("seed run %d: %v", i+1, err)
		}
	}
	counts := map[string]int64{
		mongodb.CollCountries: 2, mongodb.CollCurrencies: 3, mongodb.CollPermissions: int64(len(access.DefaultPermissions)),
		mongodb.CollRoles: int64(len(access.DefaultRoles)), mongodb.CollUsers: 1, mongodb.CollRoleAssignments: 1,
	}
	for coll, want := range counts {
		got, err := store.DB.Collection(coll).CountDocuments(ctx, bson.M{})
		if err != nil || got != want {
			t.Errorf("%s: got %d documents (err %v), want %d", coll, got, err, want)
		}
	}
	u, err := mongodb.NewUserRepository(store).FindByEmail(ctx, "root@global360.com")
	if err != nil || u.Status != identity.UserActive || u.CompanyID != nil {
		t.Fatalf("superadmin: %v %+v", err, u)
	}
	grants, err := accessGrants(ctx, store, u.ID)
	if err != nil || !grants.CanGlobally(access.PermRoleManage) {
		t.Fatalf("superadmin grants: %v", err)
	}
}

func TestValidatorsRejectInvalidDocuments(t *testing.T) {
	store, ctx := setup(t)
	now := time.Now().UTC()
	bad := bson.M{
		"_id": "u1", "person_id": "p1", "company_id": nil, "email": "x@y.com", "password_hash": "h",
		"status": "deleted", "mfa_enabled": false, "failed_login_attempts": 0, "last_access_at": nil,
		"created_at": now, "updated_at": now,
	}
	if _, err := store.DB.Collection(mongodb.CollUsers).InsertOne(ctx, bad); err == nil {
		t.Fatal("user with invalid status accepted by the database")
	}
	delete(bad, "password_hash")
	bad["status"] = "active"
	if _, err := store.DB.Collection(mongodb.CollUsers).InsertOne(ctx, bad); err == nil {
		t.Fatal("user without password_hash accepted by the database")
	}
}

func TestRepositoriesMapConflictsAndNotFound(t *testing.T) {
	store, ctx := setup(t)
	now := security.SystemClock{}.Now()
	users := mongodb.NewUserRepository(store)

	u1, _ := identity.NewUser("u1", "p1", nil, "dup@acme.com", "hash", now)
	u2, _ := identity.NewUser("u2", "p2", nil, "dup@acme.com", "hash", now)
	if err := users.Create(ctx, u1); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, u2); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("duplicate email: %v", err)
	}
	if _, err := users.FindByID(ctx, "missing"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
	got, err := users.FindByEmail(ctx, "dup@acme.com")
	if err != nil || got.ID != "u1" || !got.CreatedAt.Equal(now) {
		t.Fatalf("round trip: %v %+v", err, got)
	}

	// Only one active assignment per (user, role, company); revoked ones do not count.
	roles := mongodb.NewRoleRepository(store)
	role, _ := access.NewRole("r1", "client_admin", nil, access.ScopeCompany, true, []string{"user.read"}, now)
	if err := roles.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	assignments := mongodb.NewRoleAssignmentRepository(store)
	company := "c1"
	a1, _ := access.NewRoleAssignment("a1", "u1", role, &company, nil, now)
	a2, _ := access.NewRoleAssignment("a2", "u1", role, &company, nil, now)
	if err := assignments.Create(ctx, a1); err != nil {
		t.Fatal(err)
	}
	if err := assignments.Create(ctx, a2); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("duplicate active assignment: %v", err)
	}
	_ = a1.Revoke(nil, now)
	if err := assignments.Update(ctx, a1); err != nil {
		t.Fatal(err)
	}
	if err := assignments.Create(ctx, a2); err != nil {
		t.Fatalf("re-assign after revoke: %v", err)
	}
}

func TestAuditLogRoundTrip(t *testing.T) {
	store, ctx := setup(t)
	repo := mongodb.NewAuditLogRepository(store)
	uid := "u1"
	entry := &audit.Log{
		UserID: &uid, Entity: audit.EntityCompany, EntityID: "c1", Action: audit.ActionUpdate,
		OldValues: map[string]any{"status": "pending_validation", "nested": map[string]any{"a": 1.0}},
		NewValues: map[string]any{"status": "active"}, CreatedAt: security.SystemClock{}.Now(),
	}
	if err := repo.Create(ctx, entry); err != nil {
		t.Fatal(err)
	}
	entity := audit.EntityCompany
	logs, total, err := repo.List(ctx, portFilter(&entity))
	if err != nil || total != 1 || logs[0].ID != entry.ID {
		t.Fatalf("list: %v %d", err, total)
	}
	nested, ok := logs[0].OldValues["nested"].(map[string]any)
	if !ok {
		if m, isM := logs[0].OldValues["nested"].(bson.M); isM {
			nested, ok = m, true
		}
	}
	if !ok || nested["a"] != 1.0 || logs[0].NewValues["status"] != "active" {
		t.Fatalf("snapshots not preserved: %+v", logs[0])
	}
}

func TestTransactionRollback(t *testing.T) {
	store, ctx := setup(t)
	if !store.SupportsTransactions {
		t.Skip("standalone server: transactions not supported")
	}
	tx := mongodb.NewTxManager(store)
	users := mongodb.NewUserRepository(store)
	now := security.SystemClock{}.Now()
	boom := errors.New("boom")
	err := tx.WithinTransaction(ctx, func(ctx context.Context) error {
		u, _ := identity.NewUser("tx1", "p1", nil, "tx@acme.com", "hash", now)
		if err := users.Create(ctx, u); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	if _, err := users.FindByID(ctx, "tx1"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("write was not rolled back: %v", err)
	}
}
