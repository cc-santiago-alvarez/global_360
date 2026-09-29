//go:build integration

package mongodb_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"global_360/internal/application/port"
	"global_360/internal/domain/access"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/shared"
	"global_360/internal/infrastructure/persistence/mongodb"
	"global_360/internal/infrastructure/security"
	"global_360/internal/testutil/memory"
)

func newProfile(t *testing.T, id, name, summary string, categories []string, publish bool) *commerce.DistributorProfile {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &company.Company{ID: id, CountryID: "co", Type: company.TypeProvider, Status: company.StatusActive}
	email := "ventas@" + id + ".co"
	p, err := commerce.NewProfile(c, commerce.ProfileContent{
		DisplayName: name, Summary: summary, Description: "Operador logístico con experiencia regional.",
		CategoryIDs: categories, CoverageCountryIDs: []string{"co", "cr"}, ContactEmail: &email,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if publish {
		if err := p.Submit(now); err != nil {
			t.Fatal(err)
		}
		if err := p.Approve("reviewer", now); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestCommerceValidatorsRejectInvalidDocuments(t *testing.T) {
	store, ctx := setup(t)
	now := time.Now()
	bad := map[string]bson.M{
		mongodb.CollDistributorProfiles: {
			"_id": "c1", "country_id": "co", "display_name": "X", "summary": "", "description": "", "logo_url": nil,
			"website_url": nil, "category_ids": bson.A{}, "coverage_country_ids": bson.A{}, "contact_email": nil,
			"contact_phone": nil, "whatsapp": nil, "company_eligible": true, "status": "live", "status_reason": nil,
			"submitted_at": nil, "reviewed_at": nil, "reviewed_by": nil, "published_at": nil, "created_at": now, "updated_at": now,
		},
		mongodb.CollCategories:      {"_id": "k1", "code": "Bad Code", "name": "X", "description": nil, "active": true, "created_at": now, "updated_at": now},
		mongodb.CollContactRequests: {"_id": "r1", "distributor_id": "d", "status": "won"},
	}
	for coll, doc := range bad {
		if _, err := store.DB.Collection(coll).InsertOne(ctx, doc); err == nil {
			t.Errorf("%s accepted an invalid document", coll)
		}
	}
}

func TestMarketplaceSearchAndFilters(t *testing.T) {
	store, ctx := setup(t)
	repo := mongodb.NewDistributorProfileRepository(store)
	profiles := []*commerce.DistributorProfile{
		newProfile(t, "andina", "Logística Andina", "Agencia de aduanas en Bogotá", []string{"customs"}, true),
		newProfile(t, "caribe", "Transportes Caribe", "Carga terrestre y trámites de aduanas hacia Costa Rica", []string{"ground"}, true),
		newProfile(t, "draft", "Aduanas Borrador", "Aduanas aduanas aduanas", []string{"customs"}, false),
	}
	hidden := newProfile(t, "hidden", "Aduanas Suspendidas", "Agencia de aduanas", []string{"customs"}, true)
	hidden.SetCompanyEligible(false, time.Now())
	for _, p := range append(profiles, hidden) {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Create(ctx, profiles[0]); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("second profile for the same company: %v", err)
	}

	search := func(f port.MarketplaceFilter) []string {
		t.Helper()
		f.Page = shared.Pagination{Page: 1, PageSize: 10}
		list, total, err := repo.ListListed(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(list))
		for _, p := range list {
			ids = append(ids, p.CompanyID)
		}
		if int(total) != len(ids) {
			t.Fatalf("total %d does not match %d items", total, len(ids))
		}
		return ids
	}
	q := func(s string) *string { return &s }
	cases := []struct {
		name    string
		f       port.MarketplaceFilter
		want    []string
		ordered bool
	}{
		{"only listed profiles", port.MarketplaceFilter{}, []string{"andina", "caribe"}, false},
		{"stemming: singular finds plural", port.MarketplaceFilter{Query: q("aduana")}, []string{"andina", "caribe"}, false},
		{"relevance: name weighs more", port.MarketplaceFilter{Query: q("caribe aduanas")}, []string{"caribe", "andina"}, true},
		{"diacritic and case insensitive", port.MarketplaceFilter{Query: q("LOGISTICA")}, []string{"andina", "caribe"}, true},
		{"no match", port.MarketplaceFilter{Query: q("marítimo")}, []string{}, false},
		{"category", port.MarketplaceFilter{CategoryID: q("ground")}, []string{"caribe"}, false},
		{"coverage country", port.MarketplaceFilter{CountryID: q("cr")}, []string{"andina", "caribe"}, false},
		{"query and category", port.MarketplaceFilter{Query: q("aduanas"), CategoryID: q("customs")}, []string{"andina"}, false},
	}
	for _, c := range cases {
		got := search(c.f)
		if !sameIDs(got, c.want, c.ordered) {
			t.Errorf("%s: got %v, want %v (ordered: %v)", c.name, got, c.want, c.ordered)
		}
	}

	status := commerce.ProfileDraft
	drafts, total, err := repo.ListByStatus(ctx, &status, shared.Pagination{Page: 1, PageSize: 10})
	if err != nil || total != 1 || drafts[0].CompanyID != "draft" {
		t.Fatalf("review queue by status: %v %d %v", drafts, total, err)
	}
}

func TestOnlyOneOpenContactRequestPerUserAndDistributor(t *testing.T) {
	store, ctx := setup(t)
	repo := mongodb.NewContactRequestRepository(store)
	now := time.Now().UTC().Truncate(time.Millisecond)
	newRequest := func(id string) *commerce.ContactRequest {
		r, err := commerce.NewContactRequest(id, commerce.ContactRequestData{
			DistributorID: "andina", DistributorName: "Andina", RequesterCompanyID: "acme", RequesterCompanyName: "Acme",
			RequesterUserID: "u1", ContactName: "Ana", ContactEmail: "ana@acme.com", Message: "Necesito una cotización.",
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := newRequest("r1")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newRequest("r2")); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("second open request: %v", err)
	}
	contacted := commerce.ContactRequestContacted
	if err := first.Handle(&contacted, nil, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, newRequest("r3")); err != nil {
		t.Fatalf("a handled request frees the slot: %v", err)
	}
	distributor := "andina"
	list, total, err := repo.List(ctx, port.ContactRequestFilter{DistributorID: &distributor, Page: shared.Pagination{Page: 1, PageSize: 10}})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("list: %d %v", total, err)
	}
	sent, total, err := repo.List(ctx, port.ContactRequestFilter{RestrictToRequesters: true, Page: shared.Pagination{Page: 1, PageSize: 10}})
	if err != nil || total != 0 || len(sent) != 0 {
		t.Fatalf("an empty requester scope must match nothing: %d %v", total, err)
	}
}

func TestCommerceMigrationRunsOnce(t *testing.T) {
	store, ctx := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	seeder := mongodb.NewSeeder(store, memory.Hasher{}, security.UUIDGenerator{}, security.SystemClock{}, log)
	if err := seeder.Run(ctx, mongodb.SeedConfig{}); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.DB.Collection(mongodb.CollCategories).CountDocuments(ctx, bson.M{}); n != 8 {
		t.Fatalf("expected 8 seeded categories, got %d", n)
	}
	roles := store.DB.Collection(mongodb.CollRoles)
	phase1 := []string{access.PermCatalogRead, access.PermCompanyRead, access.PermRoleRead, access.PermUserManage, access.PermUserRead}
	setPerms := func(perms []string) {
		t.Helper()
		if _, err := roles.UpdateOne(ctx, bson.M{"name": "client_admin"}, bson.M{"$set": bson.M{"permissions": perms}}); err != nil {
			t.Fatal(err)
		}
	}
	hasContact := func() bool {
		t.Helper()
		r, err := mongodb.NewRoleRepository(store).FindByName(ctx, "client_admin")
		if err != nil {
			t.Fatal(err)
		}
		return r.HasPermission(access.PermCommerceContact)
	}

	// Simulate a database created before phase 2.
	setPerms(phase1)
	if _, err := store.DB.Collection(mongodb.CollSchemaMigrations).DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatal(err)
	}
	if err := seeder.Run(ctx, mongodb.SeedConfig{}); err != nil {
		t.Fatal(err)
	}
	if !hasContact() {
		t.Fatal("migration should grant commerce.contact to client_admin")
	}
	audited, _ := store.DB.Collection(mongodb.CollAuditLogs).CountDocuments(ctx, bson.M{"entity": "role", "new_values.source": bson.M{"$regex": "002_"}})
	if audited == 0 {
		t.Fatal("migration changes must be audited")
	}

	// An administrator removes the permission afterwards: it must not come back.
	setPerms(phase1)
	if err := seeder.Run(ctx, mongodb.SeedConfig{}); err != nil {
		t.Fatal(err)
	}
	if hasContact() {
		t.Fatal("an applied migration must not run again")
	}
}

func sameIDs(got, want []string, ordered bool) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for i := range got {
		if ordered && got[i] != want[i] {
			return false
		}
		seen[got[i]]++
		seen[want[i]]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
