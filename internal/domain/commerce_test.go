package domain_test

import (
	"errors"
	"strings"
	"testing"

	"global_360/internal/domain/commerce"
	"global_360/internal/domain/company"
	"global_360/internal/domain/shared"
)

func providerCompany(t *testing.T, status company.Status) *company.Company {
	t.Helper()
	c, err := company.New("c1", company.Data{
		LegalName: "Logística Andina SAS", DocumentType: "NIT", DocumentNumber: "900", CountryID: "co",
		BillingCurrencyID: "cop", Type: company.TypeProvider,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	c.Status = status
	return c
}

func completeContent() commerce.ProfileContent {
	email := "ventas@andina.co"
	return commerce.ProfileContent{
		DisplayName: "Logística Andina", Summary: "Agencia de aduanas CO-CR", Description: "Más de 20 años moviendo carga.",
		CategoryIDs: []string{"cat1", "cat1", " cat2 "}, CoverageCountryIDs: []string{"co", "cr"}, ContactEmail: &email,
	}
}

func expectValidation(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestDistributorProfileLifecycle(t *testing.T) {
	c := providerCompany(t, company.StatusActive)
	p, err := commerce.NewProfile(c, commerce.ProfileContent{DisplayName: "Logística Andina"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != commerce.ProfileDraft || !p.CompanyEligible || p.IsListed() || p.CountryID != "co" {
		t.Fatalf("unexpected new profile: %+v", p)
	}

	err = p.Submit(now)
	expectValidation(t, err)
	if !strings.Contains(err.Error(), "summary") || !strings.Contains(err.Error(), "contact channel") {
		t.Fatalf("incomplete profile message should list what is missing: %v", err)
	}

	if err := p.Update(completeContent(), now); err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.CategoryIDs, ",") != "cat1,cat2" {
		t.Fatalf("category ids not normalized: %v", p.CategoryIDs)
	}
	if err := p.Submit(now); err != nil {
		t.Fatal(err)
	}
	expectValidation(t, p.Update(completeContent(), now)) // locked while under review
	expectValidation(t, p.Reject("reviewer", " ", now))   // reason required
	if err := p.Reject("reviewer", "Falta el NIT en la descripción", now); err != nil {
		t.Fatal(err)
	}
	if p.Status != commerce.ProfileRejected || p.StatusReason == nil {
		t.Fatalf("rejection not recorded: %+v", p)
	}
	if err := p.Submit(now); err != nil {
		t.Fatal(err)
	}
	if p.StatusReason != nil {
		t.Fatal("resubmitting must clear the rejection reason")
	}
	if err := p.Approve("reviewer", now); err != nil {
		t.Fatal(err)
	}
	if !p.IsListed() || p.PublishedAt == nil || *p.ReviewedBy != "reviewer" {
		t.Fatalf("approved profile should be listed: %+v", p)
	}
	if err := p.Update(completeContent(), now); err != nil || p.Status != commerce.ProfilePublished {
		t.Fatalf("published profiles stay published when edited: %v %s", err, p.Status)
	}

	p.SetCompanyEligible(false, now)
	if p.IsListed() {
		t.Fatal("a profile of an ineligible company must not be listed")
	}
	p.SetCompanyEligible(true, now)

	expectValidation(t, p.Approve("reviewer", now))
	if err := p.Suspend("reviewer", "Reclamos de clientes", now); err != nil {
		t.Fatal(err)
	}
	if p.IsListed() {
		t.Fatal("suspended profile must not be listed")
	}
	expectValidation(t, p.Update(completeContent(), now))
	if err := p.Reinstate("reviewer", now); err != nil || !p.IsListed() {
		t.Fatalf("reinstate: %v", err)
	}
}

func TestDistributorProfileRules(t *testing.T) {
	client := providerCompany(t, company.StatusActive)
	client.Type = company.TypeClient
	_, err := commerce.NewProfile(client, completeContent(), now)
	expectValidation(t, err)

	pending := providerCompany(t, company.StatusPendingValidation)
	p, err := commerce.NewProfile(pending, completeContent(), now)
	if err != nil {
		t.Fatal(err)
	}
	expectValidation(t, p.Submit(now)) // company not active yet

	bad := []func(*commerce.ProfileContent){
		func(c *commerce.ProfileContent) { c.DisplayName = "  " },
		func(c *commerce.ProfileContent) { c.WebsiteURL = ptr("javascript:alert(1)") },
		func(c *commerce.ProfileContent) { c.LogoURL = ptr("ftp://logo.png") },
		func(c *commerce.ProfileContent) { c.WhatsApp = ptr("call me") },
		func(c *commerce.ProfileContent) { c.ContactEmail = ptr("not-an-email") },
		func(c *commerce.ProfileContent) { c.CategoryIDs = []string{""} },
		func(c *commerce.ProfileContent) { c.Summary = strings.Repeat("a", 281) },
	}
	for i, mutate := range bad {
		content := completeContent()
		mutate(&content)
		if _, err := commerce.NewProfile(providerCompany(t, company.StatusActive), content, now); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("case %d: expected validation error, got %v", i, err)
		}
	}
	ok := completeContent()
	ok.WebsiteURL, ok.WhatsApp = ptr("https://andina.co"), ptr("+57 300 123 4567")
	if _, err := commerce.NewProfile(providerCompany(t, company.StatusActive), ok, now); err != nil {
		t.Fatalf("valid content rejected: %v", err)
	}
}

func TestContactRequestTransitions(t *testing.T) {
	data := commerce.ContactRequestData{
		DistributorID: "d1", DistributorName: "Andina", RequesterCompanyID: "c2", RequesterCompanyName: "Acme",
		RequesterUserID: "u1", ContactName: "Ana", ContactEmail: "ANA@acme.com", Message: "Necesito cotizar 2 contenedores.",
	}
	short := data
	short.Message = "hola"
	_, err := commerce.NewContactRequest("r0", short, now)
	expectValidation(t, err)
	self := data
	self.RequesterCompanyID = "d1"
	_, err = commerce.NewContactRequest("r0", self, now)
	expectValidation(t, err)

	r, err := commerce.NewContactRequest("r1", data, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != commerce.ContactRequestNew || r.ContactEmail != "ana@acme.com" {
		t.Fatalf("unexpected request: %+v", r)
	}
	expectValidation(t, r.Handle(nil, nil, now))
	expectValidation(t, r.Handle(ptr(commerce.ContactRequestStatus("won")), nil, now))
	if err := r.Handle(ptr(commerce.ContactRequestContacted), ptr("Llamada agendada"), now); err != nil {
		t.Fatal(err)
	}
	expectValidation(t, r.Handle(ptr(commerce.ContactRequestNew), nil, now))
	if err := r.Handle(ptr(commerce.ContactRequestClosed), nil, now); err != nil {
		t.Fatal(err)
	}
	expectValidation(t, r.Handle(ptr(commerce.ContactRequestContacted), nil, now))
	if err := r.Handle(nil, ptr("Cliente firmó"), now); err != nil || *r.DistributorNotes != "Cliente firmó" {
		t.Fatalf("notes can be updated at any time: %v", err)
	}
}

func TestCategoryCode(t *testing.T) {
	c, err := commerce.NewCategory("id", " Customs_Brokerage ", "Agencia de aduanas", nil, now)
	if err != nil || c.Code != "customs_brokerage" || !c.Active {
		t.Fatalf("unexpected category: %+v %v", c, err)
	}
	_, err = commerce.NewCategory("id", "x", "Bad", nil, now)
	expectValidation(t, err)
}

func ptr[T any](v T) *T { return &v }
