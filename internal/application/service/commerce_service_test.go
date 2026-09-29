package service_test

import (
	"context"
	"strings"
	"testing"

	"global_360/internal/application/dto"
	"global_360/internal/domain/audit"
	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/shared"
	"global_360/internal/testutil/fixture"
)

// marketplace is a fixture with a provider company, a client company and their users.
type marketplace struct {
	app                             *fixture.App
	provider, client                string
	root, ops, providerAdmin, buyer context.Context
	providerOperator                context.Context
}

func newMarketplace(t *testing.T) *marketplace {
	t.Helper()
	app := fixture.New(t)
	m := &marketplace{app: app}
	m.provider = app.CreateCompanyOfType(t, "PROV-1", company.TypeProvider)
	m.client = app.CreateCompany(t, "CLIENT-1")
	m.root = app.As(t, app.CreateUser(t, "root@g360.com", password, nil, fixture.Grant{Role: "superadmin"}).ID)
	m.ops = app.As(t, app.CreateUser(t, "ops@g360.com", password, nil, fixture.Grant{Role: "operations"}).ID)
	m.providerAdmin = app.As(t, app.CreateUser(t, "admin@andina.co", password, &m.provider,
		fixture.Grant{Role: "provider_admin", CompanyID: &m.provider}).ID)
	m.providerOperator = app.As(t, app.CreateUser(t, "ops@andina.co", password, &m.provider,
		fixture.Grant{Role: "provider_operator", CompanyID: &m.provider}).ID)
	m.buyer = app.As(t, app.CreateUser(t, "buyer@acme.com", password, &m.client,
		fixture.Grant{Role: "client_admin", CompanyID: &m.client}).ID)
	return m
}

func (m *marketplace) profileInput() dto.DistributorProfileInput {
	return dto.DistributorProfileInput{
		DisplayName: "Logística Andina", Summary: "Agencia de aduanas para el corredor Colombia - Costa Rica",
		Description: "Nacionalizamos y exportamos carga desde 2005.",
		CategoryIDs: []string{m.app.CategoryIDs["customs_brokerage"]}, CoverageCountryIDs: []string{m.app.CountryCO, m.app.CountryCR},
		ContactEmail: fixture.Ptr("ventas@andina.co"), WhatsApp: fixture.Ptr("+57 300 123 4567"),
	}
}

// publish creates, submits and approves the provider profile.
func (m *marketplace) publish(t *testing.T) {
	t.Helper()
	if _, _, err := m.app.Distributors.UpsertProfile(m.providerAdmin, m.provider, m.profileInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.app.Distributors.Submit(m.providerAdmin, m.provider); err != nil {
		t.Fatal(err)
	}
	if _, err := m.app.Distributors.Approve(m.ops, m.provider); err != nil {
		t.Fatal(err)
	}
}

func (m *marketplace) listed(t *testing.T, q dto.ListDistributors) int64 {
	t.Helper()
	page, err := m.app.Marketplace.ListDistributors(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	return page.Total
}

func TestDistributorProfileIsListedOnlyAfterApproval(t *testing.T) {
	m := newMarketplace(t)
	out, created, err := m.app.Distributors.UpsertProfile(m.providerAdmin, m.provider, m.profileInput())
	if err != nil || !created || out.Status != commerce.ProfileDraft {
		t.Fatalf("create profile: created=%v %+v %v", created, out, err)
	}
	if _, created, _ = m.app.Distributors.UpsertProfile(m.providerAdmin, m.provider, m.profileInput()); created {
		t.Fatal("second upsert must update, not create")
	}
	if n := m.listed(t, dto.ListDistributors{}); n != 0 {
		t.Fatalf("draft must not be listed, got %d", n)
	}
	if _, err := m.app.Distributors.Submit(m.providerAdmin, m.provider); err != nil {
		t.Fatal(err)
	}
	if n := m.listed(t, dto.ListDistributors{}); n != 0 {
		t.Fatalf("profile pending review must not be listed, got %d", n)
	}
	queue, err := m.app.Distributors.ListProfiles(m.ops, dto.ListDistributorProfiles{Status: fixture.Ptr(commerce.ProfilePendingReview)})
	if err != nil || queue.Total != 1 {
		t.Fatalf("review queue: %+v %v", queue, err)
	}
	_, err = m.app.Distributors.Approve(m.providerAdmin, m.provider)
	expectKind(t, err, shared.ErrForbidden)

	if _, err := m.app.Distributors.Approve(m.ops, m.provider); err != nil {
		t.Fatal(err)
	}
	if n := m.listed(t, dto.ListDistributors{}); n != 1 {
		t.Fatalf("approved profile must be listed, got %d", n)
	}
	cases := []struct {
		q    dto.ListDistributors
		want int64
	}{
		{dto.ListDistributors{Query: fixture.Ptr("aduanas")}, 1},
		{dto.ListDistributors{Query: fixture.Ptr("bodega")}, 0},
		{dto.ListDistributors{CategoryID: fixture.Ptr(m.app.CategoryIDs["customs_brokerage"])}, 1},
		{dto.ListDistributors{CategoryID: fixture.Ptr(m.app.CategoryIDs["warehousing"])}, 0},
		{dto.ListDistributors{CountryID: fixture.Ptr(m.app.CountryCR)}, 1},
	}
	for _, c := range cases {
		if n := m.listed(t, c.q); n != c.want {
			t.Errorf("filter %+v: got %d, want %d", c.q, n, c.want)
		}
	}
	var approved bool
	for _, l := range m.app.Store.AuditEntries() {
		if l.Entity == audit.EntityDistributorProfile && l.Action == audit.ActionApprove {
			approved = l.OldValues["status"] == "pending_review" && l.NewValues["status"] == "published"
		}
	}
	if !approved {
		t.Fatal("approval not audited with before/after")
	}
}

func TestDistributorProfileAuthorization(t *testing.T) {
	m := newMarketplace(t)
	other := m.app.CreateCompanyOfType(t, "PROV-2", company.TypeProvider)

	_, _, err := m.app.Distributors.UpsertProfile(m.providerAdmin, other, m.profileInput())
	expectKind(t, err, shared.ErrForbidden)
	_, _, err = m.app.Distributors.UpsertProfile(m.providerOperator, m.provider, m.profileInput())
	expectKind(t, err, shared.ErrForbidden)
	_, _, err = m.app.Distributors.UpsertProfile(m.buyer, m.provider, m.profileInput())
	expectKind(t, err, shared.ErrForbidden)
	_, _, err = m.app.Distributors.UpsertProfile(m.root, m.client, m.profileInput())
	expectKind(t, err, shared.ErrValidation) // client companies cannot be distributors

	bad := m.profileInput()
	bad.CategoryIDs = []string{"does-not-exist"}
	_, _, err = m.app.Distributors.UpsertProfile(m.providerAdmin, m.provider, bad)
	expectKind(t, err, shared.ErrValidation)

	// Global 360 staff may manage any profile.
	if _, _, err := m.app.Distributors.UpsertProfile(m.ops, other, m.profileInput()); err != nil {
		t.Fatal(err)
	}
}

func TestMarketplaceDisclosesContactOnlyToAuthenticatedUsers(t *testing.T) {
	m := newMarketplace(t)
	_, err := m.app.Marketplace.GetDistributor(context.Background(), m.provider)
	expectKind(t, err, shared.ErrNotFound) // not published yet
	m.publish(t)

	for _, in := range []dto.DistributorServiceInput{
		{Name: fixture.Ptr("Nacionalización"), CategoryID: fixture.Ptr(m.app.CategoryIDs["customs_brokerage"]),
			OriginCountryID: fixture.Ptr(m.app.CountryCO), DestinationCountryID: fixture.Ptr(m.app.CountryCR)},
		{Name: fixture.Ptr("Bodega Cartago"), CategoryID: fixture.Ptr(m.app.CategoryIDs["warehousing"])},
	} {
		if _, err := m.app.Distributors.CreateService(m.providerAdmin, m.provider, in); err != nil {
			t.Fatal(err)
		}
	}
	services, _ := m.app.Distributors.ListServices(m.providerAdmin, m.provider)
	if _, err := m.app.Distributors.UpdateService(m.providerAdmin, m.provider, services[0].ID, dto.DistributorServiceInput{Active: fixture.Ptr(false)}); err != nil {
		t.Fatal(err)
	}

	anon, err := m.app.Marketplace.GetDistributor(context.Background(), m.provider)
	if err != nil {
		t.Fatal(err)
	}
	if anon.Contact != nil {
		t.Fatal("anonymous visitors must not see contact data")
	}
	if len(anon.Services) != 1 || anon.Services[0].Origin == nil || anon.Services[0].Origin.ISOCode != "CO" {
		t.Fatalf("only active services with resolved countries expected: %+v", anon.Services)
	}
	if len(anon.Categories) != 1 || anon.Categories[0].Code != "customs_brokerage" {
		t.Fatalf("categories not resolved: %+v", anon.Categories)
	}
	logged, err := m.app.Marketplace.GetDistributor(m.buyer, m.provider)
	if err != nil || logged.Contact == nil || *logged.Contact.Email != "ventas@andina.co" {
		t.Fatalf("authenticated users must see contact data: %+v %v", logged.Contact, err)
	}
}

func TestDistributorServiceRules(t *testing.T) {
	m := newMarketplace(t)
	in := dto.DistributorServiceInput{Name: fixture.Ptr("Transporte"), CategoryID: fixture.Ptr(m.app.CategoryIDs["ground_transport"])}
	_, err := m.app.Distributors.CreateService(m.providerAdmin, m.provider, in)
	expectKind(t, err, shared.ErrValidation) // profile first

	if _, _, err := m.app.Distributors.UpsertProfile(m.providerAdmin, m.provider, m.profileInput()); err != nil {
		t.Fatal(err)
	}
	svc, err := m.app.Distributors.CreateService(m.providerAdmin, m.provider, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.app.Distributors.CreateService(m.providerAdmin, m.provider, in)
	expectKind(t, err, shared.ErrConflict)
	_, err = m.app.Distributors.CreateService(m.providerAdmin, m.provider, dto.DistributorServiceInput{
		Name: fixture.Ptr("Otro"), CategoryID: fixture.Ptr(m.app.CategoryIDs["ground_transport"]), OriginCountryID: fixture.Ptr("nope"),
	})
	expectKind(t, err, shared.ErrValidation)

	other := m.app.CreateCompanyOfType(t, "PROV-2", company.TypeProvider)
	_, err = m.app.Distributors.UpdateService(m.ops, other, svc.ID, dto.DistributorServiceInput{Name: fixture.Ptr("x")})
	expectKind(t, err, shared.ErrNotFound)
}

func TestCompanyStatusControlsMarketplaceVisibility(t *testing.T) {
	m := newMarketplace(t)
	m.publish(t)

	if _, err := m.app.Companies.ChangeStatus(m.root, m.provider, company.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if n := m.listed(t, dto.ListDistributors{}); n != 0 {
		t.Fatalf("suspended company must leave the marketplace, got %d", n)
	}
	_, err := m.app.Marketplace.GetDistributor(context.Background(), m.provider)
	expectKind(t, err, shared.ErrNotFound)

	if _, err := m.app.Companies.ChangeStatus(m.root, m.provider, company.StatusActive); err != nil {
		t.Fatal(err)
	}
	if n := m.listed(t, dto.ListDistributors{}); n != 1 {
		t.Fatalf("reactivated company must be listed again, got %d", n)
	}
	if _, err := m.app.Companies.Update(m.root, m.provider, dto.UpdateCompany{CompanyType: fixture.Ptr(company.TypeClient)}); err != nil {
		t.Fatal(err)
	}
	if n := m.listed(t, dto.ListDistributors{}); n != 0 {
		t.Fatalf("a company that stops being a provider must leave the marketplace, got %d", n)
	}
}

func TestContactRequestFlow(t *testing.T) {
	m := newMarketplace(t)
	msg := dto.CreateContactRequest{Message: "Necesitamos nacionalizar 2 contenedores en Limón."}
	_, err := m.app.ContactRequests.Create(m.buyer, m.provider, msg)
	expectKind(t, err, shared.ErrNotFound) // not published
	m.publish(t)

	_, err = m.app.ContactRequests.Create(context.Background(), m.provider, msg)
	expectKind(t, err, shared.ErrUnauthorized)
	_, err = m.app.ContactRequests.Create(m.providerOperator, m.provider, msg)
	expectKind(t, err, shared.ErrForbidden) // providers do not hold commerce.contact
	_, err = m.app.ContactRequests.Create(m.buyer, m.provider, dto.CreateContactRequest{Message: msg.Message, ServiceID: fixture.Ptr("nope")})
	expectKind(t, err, shared.ErrValidation)

	req, err := m.app.ContactRequests.Create(m.buyer, m.provider, msg)
	if err != nil {
		t.Fatal(err)
	}
	if req.ContactName != "Test User" || req.ContactEmail != "buyer@acme.com" || req.DistributorName != "Logística Andina" {
		t.Fatalf("contact data should default to the requester: %+v", req)
	}
	_, err = m.app.ContactRequests.Create(m.buyer, m.provider, msg)
	expectKind(t, err, shared.ErrConflict)

	received, err := m.app.ContactRequests.ListReceived(m.providerOperator, m.provider, dto.ListContactRequests{})
	if err != nil || received.Total != 1 {
		t.Fatalf("provider should receive the lead: %+v %v", received, err)
	}
	_, err = m.app.ContactRequests.ListReceived(m.buyer, m.provider, dto.ListContactRequests{})
	expectKind(t, err, shared.ErrForbidden)

	handled, err := m.app.ContactRequests.Handle(m.providerOperator, m.provider, req.ID, dto.HandleContactRequest{
		Status: fixture.Ptr(commerce.ContactRequestContacted), Notes: fixture.Ptr("Llamar el lunes"),
	})
	if err != nil || handled.Status != commerce.ContactRequestContacted {
		t.Fatalf("handle: %+v %v", handled, err)
	}
	sent, err := m.app.ContactRequests.ListSent(m.buyer, dto.ListContactRequests{})
	if err != nil || sent.Total != 1 || sent.Items[0].DistributorNotes != nil {
		t.Fatalf("requester sees its request without internal notes: %+v %v", sent, err)
	}
	// A request no longer "new" frees the slot for a new one.
	if _, err := m.app.ContactRequests.Create(m.buyer, m.provider, msg); err != nil {
		t.Fatalf("new request after the previous one was handled: %v", err)
	}
	other := m.app.CreateCompanyOfType(t, "PROV-2", company.TypeProvider)
	_, err = m.app.ContactRequests.Handle(m.ops, other, req.ID, dto.HandleContactRequest{Notes: fixture.Ptr("x")})
	expectKind(t, err, shared.ErrNotFound)
}

func TestCompanyCannotContactItself(t *testing.T) {
	m := newMarketplace(t)
	m.publish(t)
	self := m.app.As(t, m.app.CreateUser(t, "buyer@andina.co", password, &m.provider,
		fixture.Grant{Role: "client_admin", CompanyID: &m.provider}).ID)
	_, err := m.app.ContactRequests.Create(self, m.provider, dto.CreateContactRequest{Message: "Hola, esto es una prueba."})
	expectKind(t, err, shared.ErrValidation)
	if err != nil && !strings.Contains(err.Error(), "itself") {
		t.Fatalf("unexpected message: %v", err)
	}
}
