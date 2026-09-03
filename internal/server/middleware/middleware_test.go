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
	"github.com/faucetdb/faucet/internal/rbac"
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

// newAccessStore returns a fresh in-memory config store.
func newAccessStore(t *testing.T) *config.Store {
	t.Helper()
	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("config.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// createRole inserts a role with the given access rules and returns its ID.
func createRole(t *testing.T, store *config.Store, name string, active bool, access []model.RoleAccess) int64 {
	t.Helper()
	ctx := context.Background()
	role := &model.Role{Name: name, IsActive: active}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole(%s): %v", name, err)
	}
	if len(access) > 0 {
		if err := store.SetRoleAccess(ctx, role.ID, access); err != nil {
			t.Fatalf("SetRoleAccess(%s): %v", name, err)
		}
	}
	return role.ID
}

// withPrincipal returns a middleware that injects p into the request context,
// bypassing Authenticate so the RBAC middleware can be tested in isolation.
func withPrincipal(p *Principal) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(rbac.WithPrincipal(r.Context(), p)))
		})
	}
}

// newAccessRouter mirrors the nesting used by server.go so that chi's
// RoutePath (from which the component is derived) is exercised for real.
// extra is mounted inside the "/{serviceName}" group before the routes.
func newAccessRouter(enf *rbac.Enforcer, principal *Principal, extra ...func(http.Handler) http.Handler) http.Handler {
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
	r := chi.NewRouter()
	r.Use(withPrincipal(principal))
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/{serviceName}", func(r chi.Router) {
			r.Use(RequireAccess(enf))
			for _, mw := range extra {
				r.Use(mw)
			}
			r.Get("/_schema", ok)
			r.Get("/_schema/{tableName}", ok)
			r.Post("/_schema", ok)
			r.Put("/_schema/{tableName}", ok)
			r.Delete("/_schema/{tableName}", ok)
			r.Get("/_table", ok)
			r.Get("/_table/{tableName}", ok)
			// chi does not route HEAD to GET handlers by default; register
			// HEAD explicitly so the middleware's HEAD handling is exercised.
			r.Head("/_table/{tableName}", ok)
			r.Post("/_table/{tableName}", ok)
			r.Put("/_table/{tableName}", ok)
			r.Patch("/_table/{tableName}", ok)
			r.Delete("/_table/{tableName}", ok)
			r.Get("/_proc", ok)
			r.Post("/_proc/{procName}", ok)
			r.Get("/_doc", ok)
		})
	})
	return r
}

func apiKeyPrincipal(roleID int64) *Principal {
	return &Principal{Type: rbac.PrincipalAPIKey, KeyID: 1, RoleID: roleID}
}

func doRequest(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// decodeErrorResponse decodes the standard error envelope from rr.
func decodeErrorResponse(t *testing.T, rr *httptest.ResponseRecorder) model.ErrorResponse {
	t.Helper()
	var resp model.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v; body = %s", err, rr.Body.String())
	}
	return resp
}

func TestRequireAccess_GetOnlyRole(t *testing.T) {
	store := newAccessStore(t)
	roleID := createRole(t, store, "reader", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbGet},
	})
	h := newAccessRouter(rbac.NewEnforcer(store), apiKeyPrincipal(roleID))

	rr := doRequest(h, http.MethodGet, "/api/v1/mydb/_table/users")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	for _, method := range []string{http.MethodPatch, http.MethodDelete, http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			rr := doRequest(h, method, "/api/v1/mydb/_table/users")
			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			resp := decodeErrorResponse(t, rr)
			if resp.Error.Code != http.StatusForbidden {
				t.Errorf("error.code = %d, want 403", resp.Error.Code)
			}
			if resp.Error.Message == "" {
				t.Error("error.message should not be empty")
			}
			ctx := resp.Error.Context
			if ctx["role"] != "reader" {
				t.Errorf("context.role = %v, want reader", ctx["role"])
			}
			if ctx["service"] != "mydb" {
				t.Errorf("context.service = %v, want mydb", ctx["service"])
			}
			if ctx["component"] != "_table/users" {
				t.Errorf("context.component = %v, want _table/users", ctx["component"])
			}
			if ctx["verb"] != method {
				t.Errorf("context.verb = %v, want %s", ctx["verb"], method)
			}
		})
	}
}

func TestRequireAccess_DeniedRoles(t *testing.T) {
	store := newAccessStore(t)
	enf := rbac.NewEnforcer(store)

	maskZero := createRole(t, store, "mask-zero", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: 0},
	})
	noRules := createRole(t, store, "no-rules", true, nil)
	inactive := createRole(t, store, "inactive", false, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})

	tests := []struct {
		name      string
		principal *Principal
		method    string
		wantCode  int
	}{
		{"verb_mask 0 denies GET", apiKeyPrincipal(maskZero), http.MethodGet, http.StatusForbidden},
		{"no rules denies GET", apiKeyPrincipal(noRules), http.MethodGet, http.StatusForbidden},
		{"inactive role denies GET", apiKeyPrincipal(inactive), http.MethodGet, http.StatusForbidden},
		{"nonexistent role denies GET", apiKeyPrincipal(999999), http.MethodGet, http.StatusForbidden},
		{"no principal is 401", nil, http.MethodGet, http.StatusUnauthorized},
		{"admin bypasses for DELETE", &Principal{Type: rbac.PrincipalAdmin, AdminID: 1, IsAdmin: true}, http.MethodDelete, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAccessRouter(enf, tc.principal)
			rr := doRequest(h, tc.method, "/api/v1/mydb/_table/users")
			if rr.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
			if tc.wantCode != http.StatusOK {
				resp := decodeErrorResponse(t, rr)
				if resp.Error.Code != tc.wantCode {
					t.Errorf("error.code = %d, want %d", resp.Error.Code, tc.wantCode)
				}
			}
		})
	}
}

func TestRequireAccess_ServiceScope(t *testing.T) {
	store := newAccessStore(t)
	enf := rbac.NewEnforcer(store)

	other := createRole(t, store, "other-only", true, []model.RoleAccess{
		{ServiceName: "other", Component: "*", VerbMask: model.VerbAll},
	})
	mine := createRole(t, store, "mydb-only", true, []model.RoleAccess{
		{ServiceName: "mydb", Component: "*", VerbMask: model.VerbAll},
	})
	prefix := createRole(t, store, "prefix", true, []model.RoleAccess{
		{ServiceName: "my*", Component: "*", VerbMask: model.VerbAll},
	})
	upper := createRole(t, store, "MYDB-only", true, []model.RoleAccess{
		{ServiceName: "MYDB", Component: "*", VerbMask: model.VerbAll},
	})

	tests := []struct {
		name     string
		roleID   int64
		path     string
		wantCode int
	}{
		{"rule scoped to other service denies mydb", other, "/api/v1/mydb/_table/users", http.StatusForbidden},
		{"rule scoped to other service allows other", other, "/api/v1/other/_table/users", http.StatusOK},
		{"rule scoped to mydb allows mydb", mine, "/api/v1/mydb/_table/users", http.StatusOK},
		{"rule scoped to mydb is case-sensitive", mine, "/api/v1/MyDB/_table/users", http.StatusForbidden},
		{"upper-case rule does not match lower-case service", upper, "/api/v1/mydb/_table/users", http.StatusForbidden},
		{"upper-case rule matches upper-case service", upper, "/api/v1/MYDB/_table/users", http.StatusOK},
		{"rule scoped to mydb denies other", mine, "/api/v1/other/_table/users", http.StatusForbidden},
		{"prefix rule allows mydb", prefix, "/api/v1/mydb/_table/users", http.StatusOK},
		{"prefix rule denies other", prefix, "/api/v1/other/_table/users", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAccessRouter(enf, apiKeyPrincipal(tc.roleID))
			rr := doRequest(h, http.MethodGet, tc.path)
			if rr.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestRequireAccess_ComponentScope(t *testing.T) {
	store := newAccessStore(t)
	enf := rbac.NewEnforcer(store)

	usersOnly := createRole(t, store, "users-only", true, []model.RoleAccess{
		{ServiceName: "*", Component: "_table/users", VerbMask: model.VerbGet},
	})
	allTables := createRole(t, store, "all-tables", true, []model.RoleAccess{
		{ServiceName: "*", Component: "_table/*", VerbMask: model.VerbGet},
	})
	bareName := createRole(t, store, "bare-name", true, []model.RoleAccess{
		{ServiceName: "*", Component: "users", VerbMask: model.VerbGet},
	})

	tests := []struct {
		name     string
		roleID   int64
		method   string
		path     string
		wantCode int
	}{
		{"exact table allows that table", usersOnly, http.MethodGet, "/api/v1/mydb/_table/users", http.StatusOK},
		{"exact table denies other table", usersOnly, http.MethodGet, "/api/v1/mydb/_table/orders", http.StatusForbidden},
		{"exact table denies listing", usersOnly, http.MethodGet, "/api/v1/mydb/_table", http.StatusForbidden},
		{"exact table denies schema of same table", usersOnly, http.MethodGet, "/api/v1/mydb/_schema/users", http.StatusForbidden},

		{"table wildcard allows listing", allTables, http.MethodGet, "/api/v1/mydb/_table", http.StatusOK},
		{"table wildcard allows any table", allTables, http.MethodGet, "/api/v1/mydb/_table/anything", http.StatusOK},
		{"table wildcard denies schema", allTables, http.MethodGet, "/api/v1/mydb/_schema", http.StatusForbidden},
		{"table wildcard denies schema table", allTables, http.MethodGet, "/api/v1/mydb/_schema/users", http.StatusForbidden},
		{"table wildcard denies proc", allTables, http.MethodGet, "/api/v1/mydb/_proc", http.StatusForbidden},
		{"table wildcard denies doc", allTables, http.MethodGet, "/api/v1/mydb/_doc", http.StatusForbidden},

		{"bare name allows table", bareName, http.MethodGet, "/api/v1/mydb/_table/users", http.StatusOK},
		{"bare name allows schema", bareName, http.MethodGet, "/api/v1/mydb/_schema/users", http.StatusOK},
		{"bare name denies other table", bareName, http.MethodGet, "/api/v1/mydb/_table/users2", http.StatusForbidden},
		{"bare name denies listing", bareName, http.MethodGet, "/api/v1/mydb/_table", http.StatusForbidden},

		{"URL-encoded segment is unescaped before matching", usersOnly, http.MethodGet, "/api/v1/mydb/_table/us%65rs", http.StatusOK},
		{"URL-encoded other table still denied", usersOnly, http.MethodGet, "/api/v1/mydb/_table/ord%65rs", http.StatusForbidden},
		{"component is case-sensitive", usersOnly, http.MethodGet, "/api/v1/mydb/_table/USERS", http.StatusForbidden},
		{"component is case-sensitive (mixed case)", usersOnly, http.MethodGet, "/api/v1/mydb/_table/Users", http.StatusForbidden},

		{"HEAD treated as GET", usersOnly, http.MethodHead, "/api/v1/mydb/_table/users", http.StatusOK},
		{"HEAD denied where GET is denied", usersOnly, http.MethodHead, "/api/v1/mydb/_table/orders", http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAccessRouter(enf, apiKeyPrincipal(tc.roleID))
			rr := doRequest(h, tc.method, tc.path)
			if rr.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestRequireAccess_ComponentInDenialContext(t *testing.T) {
	store := newAccessStore(t)
	enf := rbac.NewEnforcer(store)
	roleID := createRole(t, store, "users-only", true, []model.RoleAccess{
		{ServiceName: "*", Component: "_table/users", VerbMask: model.VerbGet},
	})
	h := newAccessRouter(enf, apiKeyPrincipal(roleID))

	tests := []struct {
		method        string
		path          string
		wantComponent string
	}{
		{http.MethodGet, "/api/v1/mydb/_table", "_table"},
		{http.MethodGet, "/api/v1/mydb/_schema", "_schema"},
		{http.MethodGet, "/api/v1/mydb/_schema/orders", "_schema/orders"},
		{http.MethodPost, "/api/v1/mydb/_proc/do_it", "_proc/do_it"},
		{http.MethodGet, "/api/v1/mydb/_doc", "_doc"},
		{http.MethodDelete, "/api/v1/mydb/_table/users", "_table/users"},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := doRequest(h, tc.method, tc.path)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
			}
			resp := decodeErrorResponse(t, rr)
			if resp.Error.Context["component"] != tc.wantComponent {
				t.Errorf("context.component = %v, want %s", resp.Error.Context["component"], tc.wantComponent)
			}
		})
	}
}

func TestRequireAccess_UnsupportedMethod(t *testing.T) {
	store := newAccessStore(t)
	roleID := createRole(t, store, "full", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})
	// Mount the middleware directly so the request reaches it regardless of
	// chi's per-method routing.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called for unsupported methods")
	})
	h := withPrincipal(apiKeyPrincipal(roleID))(RequireAccess(rbac.NewEnforcer(store))(inner))

	rr := doRequest(h, http.MethodOptions, "/api/v1/mydb/_table/users")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for OPTIONS, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ReadOnlyGuard middleware tests
// ---------------------------------------------------------------------------

func TestReadOnlyGuard(t *testing.T) {
	store := newAccessStore(t)
	ctx := context.Background()

	for _, svc := range []*model.ServiceConfig{
		{Name: "rodb", Driver: "sqlite", DSN: ":memory:", ReadOnly: true, IsActive: true, Pool: model.DefaultPoolConfig()},
		{Name: "rwdb", Driver: "sqlite", DSN: ":memory:", ReadOnly: false, IsActive: true, Pool: model.DefaultPoolConfig()},
	} {
		if err := store.CreateService(ctx, svc); err != nil {
			t.Fatalf("CreateService(%s): %v", svc.Name, err)
		}
	}

	// Full-access API key so RequireAccess never interferes; ReadOnlyGuard
	// must refuse writes regardless of the role.
	roleID := createRole(t, store, "full", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})
	enf := rbac.NewEnforcer(store)

	tests := []struct {
		name      string
		principal *Principal
		method    string
		path      string
		wantCode  int
	}{
		{"read-only POST", apiKeyPrincipal(roleID), http.MethodPost, "/api/v1/rodb/_table/users", http.StatusForbidden},
		{"read-only PUT", apiKeyPrincipal(roleID), http.MethodPut, "/api/v1/rodb/_table/users", http.StatusForbidden},
		{"read-only PATCH", apiKeyPrincipal(roleID), http.MethodPatch, "/api/v1/rodb/_table/users", http.StatusForbidden},
		{"read-only DELETE", apiKeyPrincipal(roleID), http.MethodDelete, "/api/v1/rodb/_table/users", http.StatusForbidden},
		{"read-only schema POST", apiKeyPrincipal(roleID), http.MethodPost, "/api/v1/rodb/_schema", http.StatusForbidden},
		{"read-only GET", apiKeyPrincipal(roleID), http.MethodGet, "/api/v1/rodb/_table/users", http.StatusOK},
		{"read-only HEAD", apiKeyPrincipal(roleID), http.MethodHead, "/api/v1/rodb/_table/users", http.StatusOK},
		{"read-only blocks admin too", &Principal{Type: rbac.PrincipalAdmin, IsAdmin: true}, http.MethodDelete, "/api/v1/rodb/_table/users", http.StatusForbidden},
		{"writable POST", apiKeyPrincipal(roleID), http.MethodPost, "/api/v1/rwdb/_table/users", http.StatusOK},
		{"writable DELETE", apiKeyPrincipal(roleID), http.MethodDelete, "/api/v1/rwdb/_table/users", http.StatusOK},
		{"unknown service passes through", apiKeyPrincipal(roleID), http.MethodPost, "/api/v1/nosuch/_table/users", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAccessRouter(enf, tc.principal, ReadOnlyGuard(store))
			rr := doRequest(h, tc.method, tc.path)
			if rr.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rr.Code, rr.Body.String())
			}
			if tc.wantCode == http.StatusForbidden {
				resp := decodeErrorResponse(t, rr)
				if resp.Error.Code != http.StatusForbidden {
					t.Errorf("error.code = %d, want 403", resp.Error.Code)
				}
				if resp.Error.Context["service"] != "rodb" {
					t.Errorf("context.service = %v, want rodb", resp.Error.Context["service"])
				}
				if resp.Error.Context["verb"] != tc.method {
					t.Errorf("context.verb = %v, want %s", resp.Error.Context["verb"], tc.method)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fail-closed on config store failure
// ---------------------------------------------------------------------------

// assertAuthorizationFailed checks that rr is a 500 in the standard error
// envelope produced when the config store cannot be consulted.
func assertAuthorizationFailed(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	resp := decodeErrorResponse(t, rr)
	if resp.Error.Code != http.StatusInternalServerError {
		t.Errorf("error.code = %d, want 500", resp.Error.Code)
	}
	if resp.Error.Message != "Authorization check failed" {
		t.Errorf("error.message = %q, want %q", resp.Error.Message, "Authorization check failed")
	}
}

func TestRequireAccess_StoreFailureFailsClosed(t *testing.T) {
	store := newAccessStore(t)
	enf := rbac.NewEnforcer(store)
	roleID := createRole(t, store, "full", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})
	h := newAccessRouter(enf, apiKeyPrincipal(roleID))

	rr := doRequest(h, http.MethodGet, "/api/v1/mydb/_table/users")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 before store failure, got %d: %s", rr.Code, rr.Body.String())
	}

	// Simulate a config store failure: every role lookup now errors with
	// something other than ErrNotFound, and the middleware must refuse the
	// request rather than let it through.
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}

	rr = doRequest(h, http.MethodGet, "/api/v1/mydb/_table/users")
	assertAuthorizationFailed(t, rr)

	// Admin principals never consult the store, so they are unaffected.
	admin := newAccessRouter(enf, &Principal{Type: rbac.PrincipalAdmin, IsAdmin: true})
	rr = doRequest(admin, http.MethodGet, "/api/v1/mydb/_table/users")
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestReadOnlyGuard_StoreFailureFailsClosed(t *testing.T) {
	store := newAccessStore(t)
	ctx := context.Background()
	svc := &model.ServiceConfig{Name: "rwdb", Driver: "sqlite", DSN: ":memory:", IsActive: true, Pool: model.DefaultPoolConfig()}
	if err := store.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	enf := rbac.NewEnforcer(store)

	// Use an admin principal so RequireAccess is bypassed and the 500 can
	// only come from ReadOnlyGuard.
	h := newAccessRouter(enf, &Principal{Type: rbac.PrincipalAdmin, IsAdmin: true}, ReadOnlyGuard(store))

	rr := doRequest(h, http.MethodPost, "/api/v1/rwdb/_table/users")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 before store failure, got %d: %s", rr.Code, rr.Body.String())
	}

	if err := store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}

	// Writes can no longer be proven safe: refuse them.
	rr = doRequest(h, http.MethodPost, "/api/v1/rwdb/_table/users")
	assertAuthorizationFailed(t, rr)

	// Reads never consult the store and still pass through the guard.
	rr = doRequest(h, http.MethodGet, "/api/v1/rwdb/_table/users")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET after store failure: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
