package httpapi_test

import (
	"net/http"
	"testing"

	"global_360/internal/domain/company"
	"global_360/internal/testutil/fixture"
)

func TestCommerceHTTPFlow(t *testing.T) {
	s := newServer(t)
	provider := s.app.CreateCompanyOfType(t, "PROV-1", company.TypeProvider)
	client := s.app.CreateCompany(t, "CLIENT-1")
	s.app.CreateUser(t, "ops@g360.com", password, nil, fixture.Grant{Role: "operations"})
	s.app.CreateUser(t, "admin@andina.co", password, &provider, fixture.Grant{Role: "provider_admin", CompanyID: &provider})
	s.app.CreateUser(t, "buyer@acme.com", password, &client, fixture.Grant{Role: "client_admin", CompanyID: &client})
	ops, seller, buyer := s.login("ops@g360.com"), s.login("admin@andina.co"), s.login("buyer@acme.com")
	base := "/api/v1/commerce"
	profile := base + "/distributors/" + provider + "/profile"

	w, body := s.do(http.MethodGet, base+"/categories", "", nil)
	if w.Code != http.StatusOK || len(body["items"].([]any)) != 3 {
		t.Fatalf("public categories: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodGet, base+"/distributors", "not-a-jwt", nil)
	if w.Code != http.StatusUnauthorized || errorCode(body) != "unauthorized" {
		t.Fatalf("an invalid token on a public route must be rejected: %d %s", w.Code, w.Body.String())
	}

	content := map[string]any{
		"display_name": "Logística Andina", "summary": "Agencia de aduanas", "description": "Carga CO-CR desde 2005.",
		"category_ids": []string{s.app.CategoryIDs["customs_brokerage"]}, "coverage_country_ids": []string{s.app.CountryCR},
		"contact_email": "ventas@andina.co",
	}
	steps := []struct {
		name, method, path, token string
		body                      any
		want                      int
	}{
		{"anonymous cannot edit", http.MethodPut, profile, "", content, http.StatusUnauthorized},
		{"client cannot edit", http.MethodPut, profile, buyer, content, http.StatusForbidden},
		{"create profile", http.MethodPut, profile, seller, content, http.StatusCreated},
		{"update profile", http.MethodPut, profile, seller, content, http.StatusOK},
		{"missing display_name", http.MethodPut, profile, seller, map[string]any{}, http.StatusBadRequest},
		{"add service", http.MethodPost, base + "/distributors/" + provider + "/services", seller,
			map[string]any{"name": "Nacionalización", "category_id": s.app.CategoryIDs["customs_brokerage"]}, http.StatusCreated},
		{"submit", http.MethodPost, profile + "/submit", seller, nil, http.StatusOK},
		{"not listed while in review", http.MethodGet, base + "/distributors/" + provider, "", nil, http.StatusNotFound},
		{"seller cannot approve", http.MethodPost, profile + "/approve", seller, nil, http.StatusForbidden},
		{"reject needs a reason", http.MethodPost, profile + "/reject", ops, map[string]any{}, http.StatusBadRequest},
		{"review queue", http.MethodGet, base + "/profiles?status=pending_review", ops, nil, http.StatusOK},
		{"approve", http.MethodPost, profile + "/approve", ops, nil, http.StatusOK},
		{"public listing", http.MethodGet, base + "/distributors?q=aduanas", "", nil, http.StatusOK},
		{"contact without login", http.MethodPost, base + "/distributors/" + provider + "/contact-requests", "",
			map[string]any{"message": "Necesito una cotización."}, http.StatusUnauthorized},
		{"contact", http.MethodPost, base + "/distributors/" + provider + "/contact-requests", buyer,
			map[string]any{"message": "Necesito una cotización."}, http.StatusCreated},
		{"duplicate open contact", http.MethodPost, base + "/distributors/" + provider + "/contact-requests", buyer,
			map[string]any{"message": "Necesito una cotización."}, http.StatusConflict},
		{"sent requests", http.MethodGet, base + "/contact-requests", buyer, nil, http.StatusOK},
		{"seller cannot list sent", http.MethodGet, base + "/contact-requests", seller, nil, http.StatusForbidden},
		{"received requests", http.MethodGet, base + "/distributors/" + provider + "/contact-requests", seller, nil, http.StatusOK},
		{"suspend", http.MethodPost, profile + "/suspend", ops, map[string]any{"reason": "Revisión"}, http.StatusOK},
		{"hidden while suspended", http.MethodGet, base + "/distributors/" + provider, "", nil, http.StatusNotFound},
		{"reinstate", http.MethodPost, profile + "/reinstate", ops, nil, http.StatusOK},
		{"create category", http.MethodPost, base + "/categories", ops, map[string]any{"code": "rail_freight", "name": "Carga férrea"}, http.StatusCreated},
		{"seller cannot create category", http.MethodPost, base + "/categories", seller, map[string]any{"code": "x_y_z", "name": "X"}, http.StatusForbidden},
	}
	for _, st := range steps {
		w, _ := s.do(st.method, st.path, st.token, st.body)
		if w.Code != st.want {
			t.Fatalf("%s: got %d, want %d: %s", st.name, w.Code, st.want, w.Body.String())
		}
	}

	w, body = s.do(http.MethodGet, base+"/distributors/"+provider, "", nil)
	if w.Code != http.StatusOK || body["contact"] != nil || len(body["services"].([]any)) != 1 {
		t.Fatalf("anonymous detail: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodGet, base+"/distributors/"+provider, buyer, nil)
	contact, _ := body["contact"].(map[string]any)
	if w.Code != http.StatusOK || contact["email"] != "ventas@andina.co" {
		t.Fatalf("authenticated detail must include contact: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodGet, base+"/distributors/"+provider+"/contact-requests", seller, nil)
	items := body["items"].([]any)
	requestID := items[0].(map[string]any)["id"].(string)
	w, body = s.do(http.MethodPatch, base+"/distributors/"+provider+"/contact-requests/"+requestID, seller,
		map[string]any{"status": "contacted", "notes": "Llamar el lunes"})
	if w.Code != http.StatusOK || body["status"] != "contacted" {
		t.Fatalf("handle contact request: %d %s", w.Code, w.Body.String())
	}
}
