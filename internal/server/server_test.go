package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/service"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const (
	testJWTSecret = "test-secret-for-jwt-integration-tests"
	testPassword  = "supersecretpassword"
	testAdminName = "Test Admin"
)

// testEnv holds all the shared state for integration tests.
type testEnv struct {
	server   *Server
	store    *config.Store
	authSvc  *service.AuthService
	registry *connector.Registry
}

// newTestEnv creates a fresh test environment with an in-memory config store,
// a default admin account, and a fully wired Server.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	store, err := config.NewStore("") // in-memory SQLite
	if err != nil {
		t.Fatalf("config.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	authSvc := service.NewAuthService(store, testJWTSecret)
	registry := connector.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := DefaultConfig()
	srv := New(cfg, registry, store, authSvc, logger)

	return &testEnv{
		server:   srv,
		store:    store,
		authSvc:  authSvc,
		registry: registry,
	}
}

// seedAdmin creates a default admin account and returns it.
func (e *testEnv) seedAdmin(t *testing.T) *model.Admin {
	t.Helper()
	admin := &model.Admin{
		Email:        "admin@example.com",
		PasswordHash: config.HashAPIKey(testPassword),
		Name:         testAdminName,
		IsActive:     true,
		IsSuperAdmin: true,
	}
	if err := e.store.CreateAdmin(context.Background(), admin); err != nil {
		t.Fatalf("seedAdmin: %v", err)
	}
	return admin
}

// adminToken logs in as the default admin and returns the JWT token string.
func (e *testEnv) adminToken(t *testing.T) string {
	t.Helper()
	body := jsonBody(t, map[string]string{
		"email":    "admin@example.com",
		"password": testPassword,
	})
	rr := e.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Token string `json:"session_token"`
	}
	decodeJSON(t, rr, &resp)
	if resp.Token == "" {
		t.Fatal("adminToken: got empty token from login")
	}
	return resp.Token
}

// do executes an HTTP request against the test server and returns the recorder.
// headers is an optional map of header key-value pairs.
func (e *testEnv) do(t *testing.T, method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	e.server.ServeHTTP(rr, req)
	return rr
}

// doAuth executes an authenticated HTTP request using the admin JWT.
func (e *testEnv) doAuth(t *testing.T, method, path string, body io.Reader, token string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(t, method, path, body, map[string]string{
		"Authorization": "Bearer " + token,
	})
}

// doAPIKey executes an HTTP request authenticated with an API key.
func (e *testEnv) doAPIKey(t *testing.T, method, path string, body io.Reader, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	return e.do(t, method, path, body, map[string]string{
		"X-API-Key": apiKey,
	})
}

func jsonBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return buf
}

func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Errorf("status = %d, want %d; body = %s", rr.Code, want, rr.Body.String())
	}
}

func assertContentType(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	got := rr.Header().Get("Content-Type")
	if got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decodeJSON: %v; body = %s", err, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Health check tests
// ---------------------------------------------------------------------------

func TestHealthz(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/healthz", nil, nil)
	assertStatus(t, rr, http.StatusOK)
	assertContentType(t, rr, "application/json")

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestReadyz(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/readyz", nil, nil)
	assertStatus(t, rr, http.StatusOK)
	assertContentType(t, rr, "application/json")

	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

// ---------------------------------------------------------------------------
// Admin login/logout tests
// ---------------------------------------------------------------------------

func TestAdminLogin_Success(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := jsonBody(t, map[string]string{
		"email":    "admin@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Token     string `json:"session_token"`
		TokenType string `json:"token_type"`
		ExpiresIn int    `json:"expires_in"`
		AdminID   int64  `json:"admin_id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
	}
	decodeJSON(t, rr, &resp)

	if resp.Token == "" {
		t.Error("expected non-empty session_token")
	}
	if resp.TokenType != "bearer" {
		t.Errorf("token_type = %q, want %q", resp.TokenType, "bearer")
	}
	if resp.ExpiresIn <= 0 {
		t.Errorf("expires_in = %d, want > 0", resp.ExpiresIn)
	}
	if resp.Email != "admin@example.com" {
		t.Errorf("email = %q, want %q", resp.Email, "admin@example.com")
	}
	if resp.Name != testAdminName {
		t.Errorf("name = %q, want %q", resp.Name, testAdminName)
	}
}

func TestAdminLogin_WrongPassword(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := jsonBody(t, map[string]string{
		"email":    "admin@example.com",
		"password": "wrongpassword",
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestAdminLogin_UnknownEmail(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := jsonBody(t, map[string]string{
		"email":    "nobody@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestAdminLogin_MissingFields(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	// Missing password
	body := jsonBody(t, map[string]string{"email": "admin@example.com"})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusBadRequest)

	// Missing email
	body = jsonBody(t, map[string]string{"password": testPassword})
	rr = env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestAdminLogin_InactiveAccount(t *testing.T) {
	env := newTestEnv(t)
	admin := &model.Admin{
		Email:        "inactive@example.com",
		PasswordHash: config.HashAPIKey(testPassword),
		Name:         "Inactive Admin",
		IsActive:     false,
	}
	if err := env.store.CreateAdmin(context.Background(), admin); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	body := jsonBody(t, map[string]string{
		"email":    "inactive@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestAdminLogout(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/admin/session", nil, nil)
	assertStatus(t, rr, http.StatusOK)

	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
}

// ---------------------------------------------------------------------------
// Authentication / authorization tests
// ---------------------------------------------------------------------------

func TestSystemEndpoints_Unauthenticated(t *testing.T) {
	env := newTestEnv(t)

	// All system admin endpoints (other than login/logout) should reject
	// unauthenticated requests with 401.
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/system/service"},
		{"POST", "/api/v1/system/service"},
		{"GET", "/api/v1/system/role"},
		{"POST", "/api/v1/system/role"},
		{"GET", "/api/v1/system/admin"},
		{"POST", "/api/v1/system/admin"},
		{"GET", "/api/v1/system/api-key"},
		{"POST", "/api/v1/system/api-key"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var body io.Reader
			if ep.method == "POST" {
				body = jsonBody(t, map[string]string{})
			}
			rr := env.do(t, ep.method, ep.path, body, nil)
			assertStatus(t, rr, http.StatusUnauthorized)
		})
	}
}

func TestSystemEndpoints_InvalidJWT(t *testing.T) {
	env := newTestEnv(t)

	rr := env.doAuth(t, "GET", "/api/v1/system/service", nil, "invalid.jwt.token")
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestSystemEndpoints_ExpiredJWT(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	// Issue a token that already expired.
	token, err := env.authSvc.IssueJWT(context.Background(), 1, "admin@example.com", -1*time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	rr := env.doAuth(t, "GET", "/api/v1/system/service", nil, token)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestSystemEndpoints_APIKeyNotAdmin(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	// Create a role and API key.
	role := &model.Role{Name: "reader", IsActive: true}
	if err := env.store.CreateRole(context.Background(), role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	rawKey := "faucet_testapikey1234567890abcdef"
	keyHash := config.HashAPIKey(rawKey)
	apiKey := &model.APIKey{
		KeyHash:   keyHash,
		KeyPrefix: rawKey[:15],
		Label:     "test",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := env.store.CreateAPIKey(context.Background(), apiKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// API keys are not admin, so system endpoints should return 403.
	rr := env.doAPIKey(t, "GET", "/api/v1/system/service", nil, rawKey)
	assertStatus(t, rr, http.StatusForbidden)
}

// ---------------------------------------------------------------------------
// Service management tests
// ---------------------------------------------------------------------------

func TestServiceCRUD(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	// --- Create ---
	createBody := jsonBody(t, map[string]interface{}{
		"name":   "testdb",
		"label":  "Test Database",
		"driver": "postgres",
		"dsn":    "postgres://localhost:5432/test",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/service", createBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["name"] != "testdb" {
		t.Errorf("created name = %v, want testdb", created["name"])
	}
	if created["driver"] != "postgres" {
		t.Errorf("created driver = %v, want postgres", created["driver"])
	}
	if created["is_active"] != true {
		t.Errorf("created is_active = %v, want true", created["is_active"])
	}

	// --- List ---
	rr = env.doAuth(t, "GET", "/api/v1/system/service", nil, token)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
		Meta     map[string]interface{}   `json:"meta"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}
	if listResp.Resource[0]["name"] != "testdb" {
		t.Errorf("list[0].name = %v, want testdb", listResp.Resource[0]["name"])
	}

	// --- Get by name ---
	rr = env.doAuth(t, "GET", "/api/v1/system/service/testdb", nil, token)
	assertStatus(t, rr, http.StatusOK)

	var getResp map[string]interface{}
	decodeJSON(t, rr, &getResp)
	if getResp["name"] != "testdb" {
		t.Errorf("get name = %v, want testdb", getResp["name"])
	}

	// --- Update ---
	updateBody := jsonBody(t, map[string]interface{}{
		"label":     "Updated Database",
		"is_active": true,
	})
	rr = env.doAuth(t, "PUT", "/api/v1/system/service/testdb", updateBody, token)
	assertStatus(t, rr, http.StatusOK)

	var updateResp map[string]interface{}
	decodeJSON(t, rr, &updateResp)
	if updateResp["label"] != "Updated Database" {
		t.Errorf("update label = %v, want Updated Database", updateResp["label"])
	}

	// --- Delete ---
	rr = env.doAuth(t, "DELETE", "/api/v1/system/service/testdb", nil, token)
	assertStatus(t, rr, http.StatusOK)

	var delResp map[string]interface{}
	decodeJSON(t, rr, &delResp)
	if delResp["success"] != true {
		t.Errorf("delete success = %v, want true", delResp["success"])
	}

	// Verify deleted.
	rr = env.doAuth(t, "GET", "/api/v1/system/service/testdb", nil, token)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestCreateService_Validation(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing name", map[string]interface{}{"driver": "postgres", "dsn": "postgres://localhost/test"}},
		{"missing driver", map[string]interface{}{"name": "test", "dsn": "postgres://localhost/test"}},
		{"missing dsn", map[string]interface{}{"name": "test", "driver": "postgres"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := env.doAuth(t, "POST", "/api/v1/system/service", jsonBody(t, tt.body), token)
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestCreateService_DuplicateName(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	body := jsonBody(t, map[string]interface{}{
		"name":   "dupdb",
		"label":  "First",
		"driver": "postgres",
		"dsn":    "postgres://localhost/test",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/service", body, token)
	assertStatus(t, rr, http.StatusCreated)

	body = jsonBody(t, map[string]interface{}{
		"name":   "dupdb",
		"label":  "Second",
		"driver": "mysql",
		"dsn":    "root@tcp(localhost)/test",
	})
	rr = env.doAuth(t, "POST", "/api/v1/system/service", body, token)
	assertStatus(t, rr, http.StatusConflict)
}

func TestGetService_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	rr := env.doAuth(t, "GET", "/api/v1/system/service/nonexistent", nil, token)
	assertStatus(t, rr, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Role management tests
// ---------------------------------------------------------------------------

func TestRoleCRUD(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	// --- Create ---
	createBody := jsonBody(t, map[string]interface{}{
		"name":        "readonly",
		"description": "Read-only access to all services",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/role", createBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["name"] != "readonly" {
		t.Errorf("created name = %v, want readonly", created["name"])
	}
	if created["is_active"] != true {
		t.Errorf("created is_active = %v, want true", created["is_active"])
	}
	roleID := created["id"]

	// --- List ---
	rr = env.doAuth(t, "GET", "/api/v1/system/role", nil, token)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}

	// --- Get ---
	roleIDStr := fmt.Sprintf("%v", roleID)
	rr = env.doAuth(t, "GET", "/api/v1/system/role/"+roleIDStr, nil, token)
	assertStatus(t, rr, http.StatusOK)

	var getResp map[string]interface{}
	decodeJSON(t, rr, &getResp)
	if getResp["name"] != "readonly" {
		t.Errorf("get name = %v, want readonly", getResp["name"])
	}

	// --- Update ---
	updateBody := jsonBody(t, map[string]interface{}{
		"name":        "read-write",
		"description": "Updated description",
		"is_active":   true,
	})
	rr = env.doAuth(t, "PUT", "/api/v1/system/role/"+roleIDStr, updateBody, token)
	assertStatus(t, rr, http.StatusOK)

	var updateResp map[string]interface{}
	decodeJSON(t, rr, &updateResp)
	if updateResp["name"] != "read-write" {
		t.Errorf("update name = %v, want read-write", updateResp["name"])
	}

	// --- Delete ---
	rr = env.doAuth(t, "DELETE", "/api/v1/system/role/"+roleIDStr, nil, token)
	assertStatus(t, rr, http.StatusOK)

	var delResp map[string]interface{}
	decodeJSON(t, rr, &delResp)
	if delResp["success"] != true {
		t.Errorf("delete success = %v, want true", delResp["success"])
	}

	// Verify deleted.
	rr = env.doAuth(t, "GET", "/api/v1/system/role/"+roleIDStr, nil, token)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestCreateRole_MissingName(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	body := jsonBody(t, map[string]interface{}{
		"description": "no name provided",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/role", body, token)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCreateRole_WithAccessRules(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	body := jsonBody(t, map[string]interface{}{
		"name":        "custom",
		"description": "Custom role with access",
		"access": []map[string]interface{}{
			{
				"service_name":   "*",
				"component":      "_table/*",
				"verb_mask":      model.VerbGet | model.VerbPost,
				"requestor_mask": model.RequestorAPI,
				"filters":        []interface{}{},
				"filter_op":      "AND",
			},
		},
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/role", body, token)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["name"] != "custom" {
		t.Errorf("name = %v, want custom", created["name"])
	}
}

// ---------------------------------------------------------------------------
// Admin management tests
// ---------------------------------------------------------------------------

func TestAdminCRUD(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	// --- List (should include the seed admin) ---
	rr := env.doAuth(t, "GET", "/api/v1/system/admin", nil, token)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}
	if listResp.Resource[0]["email"] != "admin@example.com" {
		t.Errorf("email = %v, want admin@example.com", listResp.Resource[0]["email"])
	}

	// --- Create a second admin ---
	createBody := jsonBody(t, map[string]interface{}{
		"email":    "admin2@example.com",
		"password": "anothersecretpassword",
		"name":     "Second Admin",
	})
	rr = env.doAuth(t, "POST", "/api/v1/system/admin", createBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["email"] != "admin2@example.com" {
		t.Errorf("created email = %v, want admin2@example.com", created["email"])
	}
	if created["is_active"] != true {
		t.Errorf("created is_active = %v, want true", created["is_active"])
	}

	// --- List should now have 2 ---
	rr = env.doAuth(t, "GET", "/api/v1/system/admin", nil, token)
	assertStatus(t, rr, http.StatusOK)
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 2 {
		t.Errorf("list count = %d, want 2", len(listResp.Resource))
	}

	// --- New admin can log in ---
	loginBody := jsonBody(t, map[string]string{
		"email":    "admin2@example.com",
		"password": "anothersecretpassword",
	})
	rr = env.do(t, "POST", "/api/v1/system/admin/session", loginBody, nil)
	assertStatus(t, rr, http.StatusOK)
}

func TestCreateAdmin_Validation(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"missing email", map[string]interface{}{"password": "longpassword123"}},
		{"missing password", map[string]interface{}{"email": "test@test.com"}},
		{"short password", map[string]interface{}{"email": "test@test.com", "password": "short"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := env.doAuth(t, "POST", "/api/v1/system/admin", jsonBody(t, tt.body), token)
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestCreateAdmin_DuplicateEmail(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	body := jsonBody(t, map[string]interface{}{
		"email":    "admin@example.com",
		"password": "duplicatepassword",
		"name":     "Duplicate",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/admin", body, token)
	assertStatus(t, rr, http.StatusConflict)
}

// ---------------------------------------------------------------------------
// API key management tests
// ---------------------------------------------------------------------------

func TestAPIKeyCRUD(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	// Create a role first (required for API keys).
	roleBody := jsonBody(t, map[string]interface{}{
		"name": "apitest",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/role", roleBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var roleResp map[string]interface{}
	decodeJSON(t, rr, &roleResp)
	roleID := roleResp["id"]

	// --- Create API key ---
	createBody := jsonBody(t, map[string]interface{}{
		"label":   "Test Key",
		"role_id": roleID,
	})
	rr = env.doAuth(t, "POST", "/api/v1/system/api-key", createBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var keyResp struct {
		ID        int64  `json:"id"`
		Key       string `json:"api_key"`
		KeyPrefix string `json:"key_prefix"`
		Label     string `json:"label"`
		RoleID    int64  `json:"role_id"`
		IsActive  bool   `json:"is_active"`
	}
	decodeJSON(t, rr, &keyResp)

	if keyResp.Key == "" {
		t.Fatal("expected non-empty api_key")
	}
	if keyResp.Label != "Test Key" {
		t.Errorf("label = %q, want %q", keyResp.Label, "Test Key")
	}
	if !keyResp.IsActive {
		t.Error("expected is_active = true")
	}
	if keyResp.KeyPrefix == "" {
		t.Error("expected non-empty key_prefix")
	}

	// --- List ---
	rr = env.doAuth(t, "GET", "/api/v1/system/api-key", nil, token)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}

	// --- Revoke ---
	revokeURL := fmt.Sprintf("/api/v1/system/api-key/%d", keyResp.ID)
	rr = env.doAuth(t, "DELETE", revokeURL, nil, token)
	assertStatus(t, rr, http.StatusOK)

	var revokeResp map[string]interface{}
	decodeJSON(t, rr, &revokeResp)
	if revokeResp["success"] != true {
		t.Errorf("revoke success = %v, want true", revokeResp["success"])
	}
}

func TestCreateAPIKey_MissingRoleID(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	body := jsonBody(t, map[string]interface{}{
		"label": "No Role",
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/api-key", body, token)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCreateAPIKey_NonexistentRole(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	body := jsonBody(t, map[string]interface{}{
		"label":   "Bad Role",
		"role_id": 99999,
	})
	rr := env.doAuth(t, "POST", "/api/v1/system/api-key", body, token)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestRevokeAPIKey_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	rr := env.doAuth(t, "DELETE", "/api/v1/system/api-key/99999", nil, token)
	assertStatus(t, rr, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// API key authentication for service endpoints
// ---------------------------------------------------------------------------

func TestServiceEndpoint_APIKeyAuth(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	// Create a role and API key.
	ctx := context.Background()
	role := &model.Role{Name: "reader", IsActive: true}
	if err := env.store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	rawKey := "faucet_integrationtestapikey12345"
	keyHash := config.HashAPIKey(rawKey)
	apiKey := &model.APIKey{
		KeyHash:   keyHash,
		KeyPrefix: rawKey[:15],
		Label:     "integration-test",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := env.store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// With a valid API key, the request should reach the handler
	// (will get 404 because no service is registered in the registry).
	rr := env.doAPIKey(t, "GET", "/api/v1/myservice/_table", nil, rawKey)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestServiceEndpoint_InvalidAPIKey(t *testing.T) {
	env := newTestEnv(t)

	rr := env.doAPIKey(t, "GET", "/api/v1/myservice/_table", nil, "faucet_invalid_key_here")
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestServiceEndpoint_Unauthenticated(t *testing.T) {
	env := newTestEnv(t)

	// No auth headers at all.
	rr := env.do(t, "GET", "/api/v1/myservice/_table", nil, nil)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestServiceEndpoint_RevokedAPIKey(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	role := &model.Role{Name: "revoketest", IsActive: true}
	if err := env.store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	rawKey := "faucet_revokedkeytest0987654321"
	keyHash := config.HashAPIKey(rawKey)
	apiKey := &model.APIKey{
		KeyHash:   keyHash,
		KeyPrefix: rawKey[:15],
		Label:     "revoke-test",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := env.store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Revoke the key.
	if err := env.store.RevokeAPIKey(ctx, apiKey.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	rr := env.doAPIKey(t, "GET", "/api/v1/myservice/_table", nil, rawKey)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestServiceEndpoint_JWTAuth(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)
	token := env.adminToken(t)

	// Admin JWT should also work for service endpoints (returns 404 since
	// no service is registered, but should not be 401).
	rr := env.doAuth(t, "GET", "/api/v1/myservice/_table", nil, token)
	assertStatus(t, rr, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// OpenAPI spec endpoint
// ---------------------------------------------------------------------------

func TestOpenAPISpec(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/openapi.json", nil, nil)
	assertStatus(t, rr, http.StatusOK)
	assertContentType(t, rr, "application/json")

	var spec map[string]interface{}
	decodeJSON(t, rr, &spec)

	if spec["openapi"] != "3.1.0" {
		t.Errorf("openapi version = %v, want 3.1.0", spec["openapi"])
	}
	info, ok := spec["info"].(map[string]interface{})
	if !ok {
		t.Fatal("expected info to be an object")
	}
	if info["title"] != "Faucet API" {
		t.Errorf("info.title = %v, want Faucet API", info["title"])
	}
}

// ---------------------------------------------------------------------------
// Admin UI placeholder endpoints
// ---------------------------------------------------------------------------

func TestAdminUIEndpoint(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/admin", nil, nil)
	assertStatus(t, rr, http.StatusOK)
	assertContentType(t, rr, "text/html; charset=utf-8")
}

func TestSetupEndpoint(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/setup", nil, nil)
	assertStatus(t, rr, http.StatusOK)
	assertContentType(t, rr, "text/html; charset=utf-8")
}

// ---------------------------------------------------------------------------
// CORS headers test
// ---------------------------------------------------------------------------

func TestCORSHeaders(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "OPTIONS", "/healthz", nil, map[string]string{
		"Origin":                        "http://localhost:3000",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "Authorization,Content-Type,X-API-Key",
	})

	// Chi's CORS handler should return a 2xx for preflight.
	if rr.Code < 200 || rr.Code >= 300 {
		t.Errorf("CORS preflight status = %d, want 2xx", rr.Code)
	}

	acaoHeader := rr.Header().Get("Access-Control-Allow-Origin")
	if acaoHeader == "" {
		t.Error("expected Access-Control-Allow-Origin header")
	}
}

// ---------------------------------------------------------------------------
// Full workflow: login -> create role -> create API key -> use API key
// ---------------------------------------------------------------------------

func TestFullWorkflow(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	// Step 1: Login
	loginBody := jsonBody(t, map[string]string{
		"email":    "admin@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", loginBody, nil)
	assertStatus(t, rr, http.StatusOK)

	var loginResp struct {
		Token string `json:"session_token"`
	}
	decodeJSON(t, rr, &loginResp)
	token := loginResp.Token

	// Step 2: Create a service
	svcBody := jsonBody(t, map[string]interface{}{
		"name":   "demo",
		"label":  "Demo DB",
		"driver": "postgres",
		"dsn":    "postgres://localhost/demo",
	})
	rr = env.doAuth(t, "POST", "/api/v1/system/service", svcBody, token)
	assertStatus(t, rr, http.StatusCreated)

	// Step 3: Create a role
	roleBody := jsonBody(t, map[string]interface{}{
		"name":        "demo-reader",
		"description": "Read access to demo",
	})
	rr = env.doAuth(t, "POST", "/api/v1/system/role", roleBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var roleResp map[string]interface{}
	decodeJSON(t, rr, &roleResp)
	roleID := roleResp["id"]

	// Step 4: Create an API key bound to the role
	keyBody := jsonBody(t, map[string]interface{}{
		"label":   "demo-key",
		"role_id": roleID,
	})
	rr = env.doAuth(t, "POST", "/api/v1/system/api-key", keyBody, token)
	assertStatus(t, rr, http.StatusCreated)

	var keyResp struct {
		Key string `json:"api_key"`
	}
	decodeJSON(t, rr, &keyResp)

	if keyResp.Key == "" {
		t.Fatal("expected API key in response")
	}

	// Step 5: Use the API key to access a service endpoint.
	// The request will be authenticated but the service won't be found in the
	// connector registry, so we expect 404 (not 401).
	rr = env.doAPIKey(t, "GET", "/api/v1/demo/_table", nil, keyResp.Key)
	assertStatus(t, rr, http.StatusNotFound)

	// Step 6: Verify the API key cannot access system admin endpoints (403).
	rr = env.doAPIKey(t, "GET", "/api/v1/system/service", nil, keyResp.Key)
	assertStatus(t, rr, http.StatusForbidden)

	// Step 7: Verify the admin JWT can access service endpoints too.
	rr = env.doAuth(t, "GET", "/api/v1/demo/_table", nil, token)
	assertStatus(t, rr, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Error response format test
// ---------------------------------------------------------------------------

func TestErrorResponseFormat(t *testing.T) {
	env := newTestEnv(t)

	// Hit a route that will return an error (unauthenticated).
	rr := env.do(t, "GET", "/api/v1/system/service", nil, nil)
	assertStatus(t, rr, http.StatusUnauthorized)

	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSON(t, rr, &errResp)

	if errResp.Error.Code != 401 {
		t.Errorf("error.code = %d, want 401", errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error.message")
	}
}

// ---------------------------------------------------------------------------
// Method not allowed
// ---------------------------------------------------------------------------

func TestMethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)

	// PATCH /healthz is not defined.
	rr := env.do(t, "PATCH", "/healthz", nil, nil)
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 405 or 404", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Request with invalid JSON body
// ---------------------------------------------------------------------------

func TestInvalidJSONBody(t *testing.T) {
	env := newTestEnv(t)

	body := bytes.NewBufferString("{invalid json")
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body, nil)
	assertStatus(t, rr, http.StatusBadRequest)
}
