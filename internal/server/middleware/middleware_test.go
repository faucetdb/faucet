package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/service"
)

// ---------------------------------------------------------------------------
// RequestID middleware tests
// ---------------------------------------------------------------------------

func TestRequestIDGeneratesUUID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id == "" {
			t.Error("expected non-empty request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	respID := rr.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("expected X-Request-ID in response header")
	}
	// UUID v7 format check: 36 chars with dashes
	if len(respID) != 36 {
		t.Errorf("expected UUID-length request ID, got %q (len=%d)", respID, len(respID))
	}
}

func TestRequestIDPreservesClientID(t *testing.T) {
	clientID := "my-custom-trace-id-123"

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id != clientID {
			t.Errorf("expected context ID %q, got %q", clientID, id)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", clientID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	respID := rr.Header().Get("X-Request-ID")
	if respID != clientID {
		t.Errorf("expected response X-Request-ID %q, got %q", clientID, respID)
	}
}

func TestGetRequestIDEmptyContext(t *testing.T) {
	id := GetRequestID(context.Background())
	if id != "" {
		t.Errorf("expected empty string from bare context, got %q", id)
	}
}

// ---------------------------------------------------------------------------
// RequireAdmin middleware tests
// ---------------------------------------------------------------------------

func TestRequireAdminAllowsAdmins(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin()(inner)

	req := httptest.NewRequest("GET", "/admin", nil)
	ctx := context.WithValue(req.Context(), AuthPrincipalKey, &Principal{
		Type:    "admin",
		AdminID: 1,
		IsAdmin: true,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAdminBlocksNonAdmins(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for non-admin")
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin()(inner)

	req := httptest.NewRequest("GET", "/admin", nil)
	ctx := context.WithValue(req.Context(), AuthPrincipalKey, &Principal{
		Type:    "api_key",
		RoleID:  1,
		IsAdmin: false,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdminBlocksUnauthenticated(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for unauthenticated")
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAdmin()(inner)

	req := httptest.NewRequest("GET", "/admin", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// GetPrincipal tests
// ---------------------------------------------------------------------------

func TestGetPrincipalWithValue(t *testing.T) {
	expected := &Principal{Type: "admin", AdminID: 42, IsAdmin: true}
	ctx := context.WithValue(context.Background(), AuthPrincipalKey, expected)

	got := GetPrincipal(ctx)
	if got == nil {
		t.Fatal("expected non-nil principal")
	}
	if got.AdminID != 42 {
		t.Errorf("expected AdminID 42, got %d", got.AdminID)
	}
	if !got.IsAdmin {
		t.Error("expected IsAdmin true")
	}
}

func TestGetPrincipalWithoutValue(t *testing.T) {
	got := GetPrincipal(context.Background())
	if got != nil {
		t.Error("expected nil principal from bare context")
	}
}

// ---------------------------------------------------------------------------
// RequireAccess middleware tests
// ---------------------------------------------------------------------------

// newAccessRouter builds a chi router mirroring the server's /api/v1/{serviceName}
// group with RequireAccess installed, and returns it with a role that has the
// given rules.
func newAccessRouter(t *testing.T, rules ...model.RoleAccess) (chi.Router, *model.Role) {
	t.Helper()
	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	authSvc := service.NewAuthService(store, "secret")

	role := &model.Role{Name: "r", IsActive: true}
	if err := store.CreateRole(context.Background(), role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if err := store.SetRoleAccess(context.Background(), role.ID, rules); err != nil {
		t.Fatalf("SetRoleAccess: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/api/v1/{serviceName}", func(r chi.Router) {
		r.Use(RequireAccess(authSvc))
		r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return r, role
}

func doWithPrincipal(r http.Handler, method, path string, p *Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if p != nil {
		req = req.WithContext(context.WithValue(req.Context(), AuthPrincipalKey, p))
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestRequireAccessEnforcesVerbMask(t *testing.T) {
	r, role := newAccessRouter(t, model.RoleAccess{ServiceName: "db", Component: "_table/*", VerbMask: model.VerbGet})
	p := &Principal{Type: "api_key", RoleID: role.ID}

	if rr := doWithPrincipal(r, "GET", "/api/v1/db/_table/users", p); rr.Code != http.StatusOK {
		t.Errorf("GET: got %d, want 200", rr.Code)
	}
	if rr := doWithPrincipal(r, "GET", "/api/v1/db/_table", p); rr.Code != http.StatusOK {
		t.Errorf("GET list: got %d, want 200", rr.Code)
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if rr := doWithPrincipal(r, m, "/api/v1/db/_table/users", p); rr.Code != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403", m, rr.Code)
		}
	}
	if rr := doWithPrincipal(r, "GET", "/api/v1/db/_schema", p); rr.Code != http.StatusForbidden {
		t.Errorf("GET _schema: got %d, want 403", rr.Code)
	}
	if rr := doWithPrincipal(r, "GET", "/api/v1/otherdb/_table/users", p); rr.Code != http.StatusForbidden {
		t.Errorf("other service: got %d, want 403", rr.Code)
	}
}

func TestRequireAccessErrorBody(t *testing.T) {
	r, role := newAccessRouter(t, model.RoleAccess{ServiceName: "*", Component: "*", VerbMask: model.VerbGet})
	rr := doWithPrincipal(r, "DELETE", "/api/v1/db/_table/us\"ers", &Principal{Type: "api_key", RoleID: role.ID})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rr.Code)
	}
	var body struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v: %s", err, rr.Body.String())
	}
	if body.Error.Code != 403 || body.Error.Message != `Role does not permit DELETE on db/_table/us"ers` {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestRequireAccessAdminBypass(t *testing.T) {
	r, _ := newAccessRouter(t)
	rr := doWithPrincipal(r, "DELETE", "/api/v1/db/_table/users", &Principal{Type: "admin", AdminID: 1, IsAdmin: true})
	if rr.Code != http.StatusOK {
		t.Errorf("admin: got %d, want 200", rr.Code)
	}
}

func TestRequireAccessMissingPrincipalOrRole(t *testing.T) {
	r, _ := newAccessRouter(t, model.RoleAccess{ServiceName: "*", Component: "*", VerbMask: model.VerbAll})
	if rr := doWithPrincipal(r, "GET", "/api/v1/db/_table/users", nil); rr.Code != http.StatusUnauthorized {
		t.Errorf("no principal: got %d, want 401", rr.Code)
	}
	if rr := doWithPrincipal(r, "GET", "/api/v1/db/_table/users", &Principal{Type: "api_key", RoleID: 9999}); rr.Code != http.StatusForbidden {
		t.Errorf("unknown role: got %d, want 403", rr.Code)
	}
}
