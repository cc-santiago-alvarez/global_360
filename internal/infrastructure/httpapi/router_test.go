package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"global_360/internal/infrastructure/httpapi"
	"global_360/internal/infrastructure/httpapi/handler"
	"global_360/internal/testutil/fixture"
)

const password = "correct-horse-battery"

type testServer struct {
	t      *testing.T
	app    *fixture.App
	router *gin.Engine
}

func newServer(t *testing.T) *testServer {
	gin.SetMode(gin.TestMode)
	app := fixture.New(t)
	h := &handler.Handlers{
		Auth: app.Auth, Users: app.Users, Companies: app.Companies, Roles: app.Roles, Catalog: app.Catalog, Audit: app.Audit,
		Categories: app.Categories, Marketplace: app.Marketplace, Distributors: app.Distributors, ContactRequests: app.ContactRequests,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &testServer{t: t, app: app, router: httpapi.NewRouter(h, log)}
}

func (s *testServer) do(method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func (s *testServer) login(email string) string {
	s.t.Helper()
	w, body := s.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": password})
	if w.Code != http.StatusOK {
		s.t.Fatalf("login %s: %d %s", email, w.Code, w.Body.String())
	}
	return body["access_token"].(string)
}

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestPublicRoutes(t *testing.T) {
	s := newServer(t)
	tests := []struct {
		path string
		body string
	}{
		{"/health", `{"status":"ok"}`},
		{"/ready", `{"status":"ok"}`},
		{"/api/v1/ping", `{"message":"pong"}`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w, _ := s.do(http.MethodGet, tt.path, "", nil)
			if w.Code != http.StatusOK || w.Body.String() != tt.body {
				t.Fatalf("got %d %s, want 200 %s", w.Code, w.Body.String(), tt.body)
			}
			if w.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing X-Request-ID")
			}
		})
	}
}

func TestErrorFormat(t *testing.T) {
	s := newServer(t)

	w, body := s.do(http.MethodGet, "/api/v1/companies", "", nil)
	if w.Code != http.StatusUnauthorized || errorCode(body) != "unauthorized" {
		t.Fatalf("no token: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodGet, "/api/v1/companies", "not-a-jwt", nil)
	if w.Code != http.StatusUnauthorized || errorCode(body) != "unauthorized" {
		t.Fatalf("bad token: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": "x@y.com"})
	if w.Code != http.StatusBadRequest || errorCode(body) != "validation_error" {
		t.Fatalf("missing password: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": "x@y.com", "password": "whatever-123"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credentials: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodGet, "/api/v1/unknown", "", nil)
	if w.Code != http.StatusNotFound || errorCode(body) != "not_found" || requestID(body) == "" {
		t.Fatalf("unknown route: %d %s", w.Code, w.Body.String())
	}
	w, body = s.do(http.MethodDelete, "/api/v1/ping", "", nil)
	if w.Code != http.StatusMethodNotAllowed || errorCode(body) != "method_not_allowed" || requestID(body) == "" {
		t.Fatalf("wrong method: %d %s", w.Code, w.Body.String())
	}
}

func requestID(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	id, _ := e["request_id"].(string)
	return id
}

func TestEndToEndFoundationFlow(t *testing.T) {
	s := newServer(t)
	s.app.CreateUser(t, "admin@global360.com", password, nil, fixture.Grant{Role: "superadmin"})
	admin := s.login("admin@global360.com")

	w, me := s.do(http.MethodGet, "/api/v1/auth/me", admin, nil)
	if w.Code != http.StatusOK || len(me["global_permissions"].([]any)) == 0 {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}

	// Create a client company and activate it.
	w, comp := s.do(http.MethodPost, "/api/v1/companies", admin, map[string]any{
		"legal_name": "ACME SAS", "document_type": "NIT", "document_number": "900123",
		"country_id": s.app.CountryCO, "billing_currency_id": s.app.CurrencyCOP, "company_type": "client",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create company: %d %s", w.Code, w.Body.String())
	}
	companyID := comp["id"].(string)
	w, _ = s.do(http.MethodPatch, "/api/v1/companies/"+companyID+"/status", admin, map[string]string{"status": "active"})
	if w.Code != http.StatusOK {
		t.Fatalf("activate company: %d %s", w.Code, w.Body.String())
	}

	// Create a user for it, activate it and make it client_admin.
	w, user := s.do(http.MethodPost, "/api/v1/users", admin, map[string]any{
		"email": "boss@acme.com", "password": password, "company_id": companyID,
		"person": map[string]any{
			"document_type": "CC", "document_number": "1010", "document_country_id": s.app.CountryCO,
			"first_names": "Laura", "last_names": "Gómez", "email": "laura@acme.com",
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", w.Code, w.Body.String())
	}
	if _, leaked := user["password_hash"]; leaked {
		t.Fatal("password_hash exposed")
	}
	userID := user["id"].(string)
	w, _ = s.do(http.MethodPatch, "/api/v1/users/"+userID+"/status", admin, map[string]string{"status": "active"})
	if w.Code != http.StatusOK {
		t.Fatalf("activate user: %d %s", w.Code, w.Body.String())
	}
	w, _ = s.do(http.MethodPost, "/api/v1/users/"+userID+"/roles", admin, map[string]any{
		"role_id": s.app.RoleIDs["client_admin"], "company_id": companyID,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("assign role: %d %s", w.Code, w.Body.String())
	}

	// The client admin sees only its company and cannot use global operations.
	boss := s.login("boss@acme.com")
	w, list := s.do(http.MethodGet, "/api/v1/companies", boss, nil)
	if w.Code != http.StatusOK || list["total"].(float64) != 1 {
		t.Fatalf("scoped company list: %d %s", w.Code, w.Body.String())
	}
	w, _ = s.do(http.MethodPost, "/api/v1/companies", boss, map[string]any{
		"legal_name": "Other", "document_type": "NIT", "document_number": "1", "country_id": s.app.CountryCO,
		"billing_currency_id": s.app.CurrencyCOP, "company_type": "client",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("client admin created a company: %d", w.Code)
	}
	w, _ = s.do(http.MethodGet, "/api/v1/audit-logs", boss, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("client admin read audit logs: %d", w.Code)
	}

	// The audit trail shows the company status change with before/after snapshots.
	w, logs := s.do(http.MethodGet, "/api/v1/audit-logs?entity=company&entity_id="+companyID, admin, nil)
	if w.Code != http.StatusOK || logs["total"].(float64) != 2 {
		t.Fatalf("audit logs: %d %s", w.Code, w.Body.String())
	}
	latest := logs["items"].([]any)[0].(map[string]any)
	if latest["old_values"].(map[string]any)["status"] != "pending_validation" || latest["new_values"].(map[string]any)["status"] != "active" {
		t.Fatalf("unexpected audit snapshot: %v", latest)
	}
}
