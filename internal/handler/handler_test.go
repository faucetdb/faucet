package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/service"
)

const (
	testJWTSecret = "test-secret-for-handler-tests"
	testPassword  = "supersecretpassword"
)

// testEnv holds shared state for handler integration tests.
type testEnv struct {
	store   *config.Store
	authSvc *service.AuthService
	handler *SystemHandler
	router  chi.Router
}

// newTestEnv creates a fresh test environment with an in-memory config store,
// a system handler, and a Chi router with routes mounted (no auth middleware).
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	store, err := config.NewStore("") // in-memory SQLite
	if err != nil {
		t.Fatalf("config.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	authSvc := service.NewAuthService(store, testJWTSecret)
	sysHandler := NewSystemHandler(store, authSvc, connector.NewRegistry())

	// Mount routes without auth middleware for direct handler testing.
	r := chi.NewRouter()
	r.Route("/api/v1/system", func(r chi.Router) {
		r.Post("/admin/session", sysHandler.Login)
		r.Delete("/admin/session", sysHandler.Logout)

		r.Get("/service", sysHandler.ListServices)
		r.Post("/service", sysHandler.CreateService)
		r.Get("/service/{serviceName}", sysHandler.GetService)
		r.Put("/service/{serviceName}", sysHandler.UpdateService)
		r.Delete("/service/{serviceName}", sysHandler.DeleteService)

		r.Get("/role", sysHandler.ListRoles)
		r.Post("/role", sysHandler.CreateRole)
		r.Get("/role/{roleId}", sysHandler.GetRole)
		r.Put("/role/{roleId}", sysHandler.UpdateRole)
		r.Delete("/role/{roleId}", sysHandler.DeleteRole)

		r.Get("/admin", sysHandler.ListAdmins)
		r.Post("/admin", sysHandler.CreateAdmin)

		r.Get("/api-key", sysHandler.ListAPIKeys)
		r.Post("/api-key", sysHandler.CreateAPIKey)
		r.Delete("/api-key/{keyId}", sysHandler.RevokeAPIKey)

		r.Get("/mcp", sysHandler.MCPInfo)
	})

	return &testEnv{
		store:   store,
		authSvc: authSvc,
		handler: sysHandler,
		router:  r,
	}
}

// seedAdmin creates a default admin account and returns it.
func (e *testEnv) seedAdmin(t *testing.T) *model.Admin {
	t.Helper()
	admin := &model.Admin{
		Email:        "admin@example.com",
		PasswordHash: config.HashAPIKey(testPassword),
		Name:         "Test Admin",
		IsActive:     true,
		IsSuperAdmin: true,
	}
	if err := e.store.CreateAdmin(context.Background(), admin); err != nil {
		t.Fatalf("seedAdmin: %v", err)
	}
	return admin
}

// seedRole creates a role and returns it.
func (e *testEnv) seedRole(t *testing.T, name string) *model.Role {
	t.Helper()
	role := &model.Role{
		Name:        name,
		Description: "Test role: " + name,
		IsActive:    true,
	}
	if err := e.store.CreateRole(context.Background(), role); err != nil {
		t.Fatalf("seedRole: %v", err)
	}
	return role
}

// do executes an HTTP request against the test router and returns the recorder.
func (e *testEnv) do(t *testing.T, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)
	return rr
}

func toJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		t.Fatalf("toJSON: %v", err)
	}
	return buf
}

func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Errorf("status = %d, want %d; body = %s", rr.Code, want, rr.Body.String())
	}
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decodeJSON: %v; body = %s", err, rr.Body.String())
	}
}

// suppress unused import for slog and time in this file
var _ = slog.Default
var _ = time.Now
