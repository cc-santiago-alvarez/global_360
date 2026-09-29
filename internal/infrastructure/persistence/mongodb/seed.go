package mongodb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/identity"
	"global_360/internal/domain/shared"
)

// SeedConfig configures the optional bootstrap superadmin.
type SeedConfig struct {
	SuperadminEmail    string
	SuperadminPassword string
}

type countrySeed struct{ iso, name string }
type categorySeed struct{ code, name, description string }
type currencySeed struct {
	iso, name, symbol string
	decimals          int
}

var (
	defaultCountries  = []countrySeed{{"CO", "Colombia"}, {"CR", "Costa Rica"}}
	defaultCurrencies = []currencySeed{
		{"COP", "Peso colombiano", "$", 2},
		{"CRC", "Colón costarricense", "₡", 2},
		{"USD", "Dólar estadounidense", "US$", 2},
	}
	defaultCategories = []categorySeed{
		{"freight_forwarding", "Agente de carga", "Coordinación integral del envío internacional"},
		{"customs_brokerage", "Agencia de aduanas", "Nacionalización, exportación y trámites aduaneros"},
		{"ground_transport", "Transporte terrestre", "Carga por carretera nacional e internacional"},
		{"air_freight", "Carga aérea", "Transporte de mercancía por vía aérea"},
		{"ocean_freight", "Carga marítima", "Transporte de contenedores y carga suelta por vía marítima"},
		{"warehousing", "Bodegaje", "Almacenamiento, inventario y preparación de pedidos"},
		{"last_mile", "Última milla", "Distribución y entrega al destino final"},
		{"cargo_insurance", "Seguro de carga", "Pólizas para proteger la mercancía en tránsito"},
	}
)

// Seeder loads the minimum reference data. Every step is idempotent (upsert by natural key)
// and never overwrites data edited by administrators, except the superadmin permissions,
// which always contain every permission.
type Seeder struct {
	store  *Store
	hasher port.PasswordHasher
	ids    port.IDGenerator
	clock  port.Clock
	log    *slog.Logger
}

func NewSeeder(store *Store, hasher port.PasswordHasher, ids port.IDGenerator, clock port.Clock, log *slog.Logger) *Seeder {
	return &Seeder{store: store, hasher: hasher, ids: ids, clock: clock, log: log}
}

func (s *Seeder) Run(ctx context.Context, cfg SeedConfig) error {
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"countries", s.seedCountries},
		{"currencies", s.seedCurrencies},
		{"permissions", s.seedPermissions},
		{"roles", s.seedRoles},
		{"categories", s.seedCategories},
		{"migrations", s.runMigrations},
	}
	for _, step := range steps {
		if err := step.fn(ctx); err != nil {
			return fmt.Errorf("seed %s: %w", step.name, err)
		}
	}
	if err := s.seedSuperadmin(ctx, cfg); err != nil {
		return fmt.Errorf("seed superadmin: %w", err)
	}
	return nil
}

func (s *Seeder) upsert(ctx context.Context, coll string, filter bson.M, update bson.M) error {
	_, err := s.store.coll(coll).UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	return err
}

func (s *Seeder) seedCountries(ctx context.Context) error {
	now := s.clock.Now()
	for _, c := range defaultCountries {
		err := s.upsert(ctx, CollCountries, bson.M{"iso_code": c.iso}, bson.M{"$setOnInsert": bson.M{
			"_id": s.ids.NewID(), "name": c.name, "active": true, "created_at": now,
		}})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) seedCurrencies(ctx context.Context) error {
	now := s.clock.Now()
	for _, c := range defaultCurrencies {
		err := s.upsert(ctx, CollCurrencies, bson.M{"iso_code": c.iso}, bson.M{"$setOnInsert": bson.M{
			"_id": s.ids.NewID(), "name": c.name, "symbol": c.symbol, "decimals": c.decimals, "active": true, "created_at": now,
		}})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) seedPermissions(ctx context.Context) error {
	for _, p := range access.DefaultPermissions {
		err := s.upsert(ctx, CollPermissions, bson.M{"code": p.Code}, bson.M{
			"$setOnInsert": bson.M{"_id": s.ids.NewID()},
			"$set":         bson.M{"module": p.Module, "description": p.Description},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Seeder) seedRoles(ctx context.Context) error {
	now := s.clock.Now()
	for _, r := range access.DefaultRoles {
		perms, err := access.NormalizePermissionCodes(r.Permissions)
		if err != nil {
			return err
		}
		err = s.upsert(ctx, CollRoles, bson.M{"name": r.Name}, bson.M{"$setOnInsert": bson.M{
			"_id": s.ids.NewID(), "description": r.Description, "scope": string(r.Scope), "is_system": true,
			"active": true, "permissions": perms, "created_at": now, "updated_at": now,
		}})
		if err != nil {
			return err
		}
	}
	// Superadmin always holds the full, current permission catalog.
	all, err := access.NormalizePermissionCodes(access.AllPermissionCodes())
	if err != nil {
		return err
	}
	_, err = s.store.coll(CollRoles).UpdateOne(ctx, bson.M{"name": access.SuperadminRoleName},
		bson.M{"$set": bson.M{"permissions": all, "is_system": true, "active": true}})
	return err
}

func (s *Seeder) seedCategories(ctx context.Context) error {
	now := s.clock.Now()
	for _, c := range defaultCategories {
		err := s.upsert(ctx, CollCategories, bson.M{"code": c.code}, bson.M{"$setOnInsert": bson.M{
			"_id": s.ids.NewID(), "name": c.name, "description": c.description, "active": true,
			"created_at": now, "updated_at": now,
		}})
		if err != nil {
			return err
		}
	}
	return nil
}

// migration is a one-off data change for databases created by earlier versions.
// Applied migrations are recorded in schema_migrations and never run again.
type migration struct {
	id string
	fn func(context.Context) error
}

func (s *Seeder) migrations() []migration {
	return []migration{
		{"002_commerce_role_permissions", s.migrateCommerceRolePermissions},
	}
}

func (s *Seeder) runMigrations(ctx context.Context) error {
	coll := s.store.coll(CollSchemaMigrations)
	for _, m := range s.migrations() {
		err := coll.FindOne(ctx, bson.M{"_id": m.id}).Err()
		if err == nil {
			continue
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if err := m.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", m.id, err)
		}
		if _, err := coll.InsertOne(ctx, bson.M{"_id": m.id, "applied_at": s.clock.Now()}); err != nil {
			return err
		}
		s.log.Info("migration applied", "id", m.id)
	}
	return nil
}

// migrateCommerceRolePermissions grants the phase 2 permissions to the system roles
// created before phase 2 (roles are seeded with $setOnInsert, so they would not get them).
func (s *Seeder) migrateCommerceRolePermissions(ctx context.Context) error {
	roles := NewRoleRepository(s.store)
	auditRepo := NewAuditLogRepository(s.store)
	now := s.clock.Now()
	for name, perms := range access.Phase2RolePermissions {
		role, err := roles.FindByName(ctx, name)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		merged, err := access.NormalizePermissionCodes(append(append([]string{}, role.Permissions...), perms...))
		if err != nil {
			return err
		}
		if len(merged) == len(role.Permissions) {
			continue
		}
		_, err = s.store.coll(CollRoles).UpdateOne(ctx, bson.M{"_id": role.ID},
			bson.M{"$set": bson.M{"permissions": merged, "updated_at": now}})
		if err != nil {
			return err
		}
		err = auditRepo.Create(ctx, &audit.Log{
			Entity: audit.EntityRole, EntityID: role.ID, Action: audit.ActionUpdate, CreatedAt: now,
			OldValues: map[string]any{"permissions": role.Permissions},
			NewValues: map[string]any{"permissions": merged, "source": "migration 002_commerce_role_permissions"},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// seedSuperadmin creates the first administrator when it does not exist yet.
func (s *Seeder) seedSuperadmin(ctx context.Context, cfg SeedConfig) error {
	if cfg.SuperadminEmail == "" || cfg.SuperadminPassword == "" {
		s.log.Warn("SEED_SUPERADMIN_EMAIL/SEED_SUPERADMIN_PASSWORD not set; skipping superadmin bootstrap")
		return nil
	}
	users := NewUserRepository(s.store)
	email, err := shared.NormalizeEmail("SEED_SUPERADMIN_EMAIL", cfg.SuperadminEmail)
	if err != nil {
		return err
	}
	if _, err := users.FindByEmail(ctx, email); err == nil {
		return nil // already bootstrapped
	} else if !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	if err := identity.ValidatePassword(cfg.SuperadminPassword); err != nil {
		return fmt.Errorf("SEED_SUPERADMIN_PASSWORD: %w", err)
	}

	country, err := findOne[countryDoc](ctx, s.store.coll(CollCountries), bson.M{"iso_code": "CO"}, "country CO not found")
	if err != nil {
		return err
	}
	role, err := NewRoleRepository(s.store).FindByName(ctx, access.SuperadminRoleName)
	if err != nil {
		return err
	}
	hash, err := s.hasher.Hash(cfg.SuperadminPassword)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	persons := NewPersonRepository(s.store)
	person, err := persons.FindByDocument(ctx, "INTERNAL", email, country.ID)
	if errors.Is(err, shared.ErrNotFound) {
		person, err = identity.NewPerson(s.ids.NewID(), identity.PersonData{
			DocumentType: "INTERNAL", DocumentNumber: email, DocumentCountryID: country.ID,
			FirstNames: "Super", LastNames: "Admin", Email: email,
		}, now)
		if err != nil {
			return err
		}
		err = persons.Create(ctx, person)
	}
	if err != nil {
		return err
	}
	user, err := identity.NewUser(s.ids.NewID(), person.ID, nil, email, hash, now)
	if err != nil {
		return err
	}
	user.Status = identity.UserActive
	if err := users.Create(ctx, user); err != nil {
		return err
	}
	assignment, err := access.NewRoleAssignment(s.ids.NewID(), user.ID, role, nil, nil, now)
	if err != nil {
		return err
	}
	if err := NewRoleAssignmentRepository(s.store).Create(ctx, assignment); err != nil {
		return err
	}
	auditRepo := NewAuditLogRepository(s.store)
	for _, l := range []*audit.Log{
		{Entity: audit.EntityUser, EntityID: user.ID, Action: audit.ActionCreate, NewValues: map[string]any{"email": email, "source": "seed"}, CreatedAt: now},
		{Entity: audit.EntityRoleAssignment, EntityID: assignment.ID, Action: audit.ActionCreate, NewValues: map[string]any{"role": role.Name, "user_id": user.ID, "source": "seed"}, CreatedAt: now},
	} {
		if err := auditRepo.Create(ctx, l); err != nil {
			return err
		}
	}
	s.log.Info("superadmin bootstrapped", "email", email)
	return nil
}
