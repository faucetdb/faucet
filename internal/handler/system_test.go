package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
)

// ---------------------------------------------------------------------------
// Login / Logout
// ---------------------------------------------------------------------------

func TestLogin_ValidCredentials(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := toJSON(t, map[string]string{
		"email":    "admin@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body)
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
	if resp.Name != "Test Admin" {
		t.Errorf("name = %q, want %q", resp.Name, "Test Admin")
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := toJSON(t, map[string]string{
		"email":    "admin@example.com",
		"password": "wrongpassword",
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestLogin_UnknownEmail(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := toJSON(t, map[string]string{
		"email":    "nobody@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestLogin_MissingFields(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing password", map[string]string{"email": "admin@example.com"}},
		{"missing email", map[string]string{"password": testPassword}},
		{"both empty", map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := env.do(t, "POST", "/api/v1/system/admin/session", toJSON(t, tt.body))
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestLogin_InactiveAccount(t *testing.T) {
	env := newTestEnv(t)

	admin := &model.Admin{
		Email:        "inactive@example.com",
		PasswordHash: config.HashAPIKey(testPassword),
		Name:         "Inactive",
		IsActive:     false,
	}
	if err := env.store.CreateAdmin(t.Context(), admin); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	body := toJSON(t, map[string]string{
		"email":    "inactive@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", body)
	assertStatus(t, rr, http.StatusUnauthorized)
}

func TestLogin_InvalidJSON(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "POST", "/api/v1/system/admin/session",
		toJSON(t, "not an object"))
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestLogout(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/admin/session", nil)
	assertStatus(t, rr, http.StatusOK)

	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
}

// ---------------------------------------------------------------------------
// Service CRUD
// ---------------------------------------------------------------------------

func TestServiceCRUD(t *testing.T) {
	env := newTestEnv(t)

	// --- Create ---
	body := toJSON(t, map[string]interface{}{
		"name":   "mydb",
		"label":  "My Database",
		"driver": "postgres",
		"dsn":    "postgres://localhost:5432/mydb",
	})
	rr := env.do(t, "POST", "/api/v1/system/service", body)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["name"] != "mydb" {
		t.Errorf("created name = %v, want mydb", created["name"])
	}
	if created["driver"] != "postgres" {
		t.Errorf("created driver = %v, want postgres", created["driver"])
	}
	if created["is_active"] != true {
		t.Errorf("created is_active = %v, want true", created["is_active"])
	}

	// --- List ---
	rr = env.do(t, "GET", "/api/v1/system/service", nil)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
		Meta     map[string]interface{}   `json:"meta"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}
	if listResp.Resource[0]["name"] != "mydb" {
		t.Errorf("list[0].name = %v, want mydb", listResp.Resource[0]["name"])
	}

	// --- Get by name ---
	rr = env.do(t, "GET", "/api/v1/system/service/mydb", nil)
	assertStatus(t, rr, http.StatusOK)

	var getResp map[string]interface{}
	decodeJSON(t, rr, &getResp)
	if getResp["name"] != "mydb" {
		t.Errorf("get name = %v, want mydb", getResp["name"])
	}

	// --- Update ---
	updateBody := toJSON(t, map[string]interface{}{
		"label":     "Updated DB",
		"is_active": true,
	})
	rr = env.do(t, "PUT", "/api/v1/system/service/mydb", updateBody)
	assertStatus(t, rr, http.StatusOK)

	var updateResp map[string]interface{}
	decodeJSON(t, rr, &updateResp)
	if updateResp["label"] != "Updated DB" {
		t.Errorf("update label = %v, want Updated DB", updateResp["label"])
	}

	// --- Delete ---
	rr = env.do(t, "DELETE", "/api/v1/system/service/mydb", nil)
	assertStatus(t, rr, http.StatusOK)

	var delResp map[string]interface{}
	decodeJSON(t, rr, &delResp)
	if delResp["success"] != true {
		t.Errorf("delete success = %v, want true", delResp["success"])
	}

	// Verify deleted.
	rr = env.do(t, "GET", "/api/v1/system/service/mydb", nil)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestCreateService_Validation(t *testing.T) {
	env := newTestEnv(t)

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
			rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, tt.body))
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestCreateService_DuplicateName(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"name":   "dupdb",
		"driver": "postgres",
		"dsn":    "postgres://localhost/dup",
	})
	rr := env.do(t, "POST", "/api/v1/system/service", body)
	assertStatus(t, rr, http.StatusCreated)

	body = toJSON(t, map[string]interface{}{
		"name":   "dupdb",
		"driver": "mysql",
		"dsn":    "root@tcp(localhost)/dup",
	})
	rr = env.do(t, "POST", "/api/v1/system/service", body)
	assertStatus(t, rr, http.StatusConflict)
}

func TestGetService_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/api/v1/system/service/nonexistent", nil)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestUpdateService_NotFound(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"label": "nope",
	})
	rr := env.do(t, "PUT", "/api/v1/system/service/nonexistent", body)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestDeleteService_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/service/nonexistent", nil)
	assertStatus(t, rr, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Role CRUD
// ---------------------------------------------------------------------------

func TestRoleCRUD(t *testing.T) {
	env := newTestEnv(t)

	// --- Create ---
	body := toJSON(t, map[string]interface{}{
		"name":        "readonly",
		"description": "Read-only access",
	})
	rr := env.do(t, "POST", "/api/v1/system/role", body)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["name"] != "readonly" {
		t.Errorf("name = %v, want readonly", created["name"])
	}
	if created["is_active"] != true {
		t.Errorf("is_active = %v, want true", created["is_active"])
	}
	roleID := created["id"]

	// --- List ---
	rr = env.do(t, "GET", "/api/v1/system/role", nil)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}

	// --- Get ---
	roleIDStr := fmt.Sprintf("%.0f", roleID)
	rr = env.do(t, "GET", "/api/v1/system/role/"+roleIDStr, nil)
	assertStatus(t, rr, http.StatusOK)

	var getResp map[string]interface{}
	decodeJSON(t, rr, &getResp)
	if getResp["name"] != "readonly" {
		t.Errorf("get name = %v, want readonly", getResp["name"])
	}

	// --- Update ---
	updateBody := toJSON(t, map[string]interface{}{
		"name":        "readwrite",
		"description": "Read-write access",
		"is_active":   true,
	})
	rr = env.do(t, "PUT", "/api/v1/system/role/"+roleIDStr, updateBody)
	assertStatus(t, rr, http.StatusOK)

	var updateResp map[string]interface{}
	decodeJSON(t, rr, &updateResp)
	if updateResp["name"] != "readwrite" {
		t.Errorf("update name = %v, want readwrite", updateResp["name"])
	}

	// --- Delete ---
	rr = env.do(t, "DELETE", "/api/v1/system/role/"+roleIDStr, nil)
	assertStatus(t, rr, http.StatusOK)

	var delResp map[string]interface{}
	decodeJSON(t, rr, &delResp)
	if delResp["success"] != true {
		t.Errorf("delete success = %v, want true", delResp["success"])
	}

	// Verify deleted.
	rr = env.do(t, "GET", "/api/v1/system/role/"+roleIDStr, nil)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestCreateRole_MissingName(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"description": "no name",
	})
	rr := env.do(t, "POST", "/api/v1/system/role", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCreateRole_WithAccessRules(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"name":        "custom",
		"description": "Custom role with access rules",
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
	rr := env.do(t, "POST", "/api/v1/system/role", body)
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	if created["name"] != "custom" {
		t.Errorf("name = %v, want custom", created["name"])
	}

	// Verify access rules were persisted by fetching the role.
	roleIDStr := fmt.Sprintf("%.0f", created["id"])
	rr = env.do(t, "GET", "/api/v1/system/role/"+roleIDStr, nil)
	assertStatus(t, rr, http.StatusOK)

	var getResp map[string]interface{}
	decodeJSON(t, rr, &getResp)
	access, ok := getResp["access"].([]interface{})
	if !ok {
		t.Fatal("expected access to be an array")
	}
	if len(access) != 1 {
		t.Errorf("access count = %d, want 1", len(access))
	}
}

func TestUpdateRole_WithAccessRules(t *testing.T) {
	env := newTestEnv(t)
	role := env.seedRole(t, "updatable")

	// Update the role with new access rules.
	body := toJSON(t, map[string]interface{}{
		"name":      "updatable",
		"is_active": true,
		"access": []map[string]interface{}{
			{
				"service_name":   "mydb",
				"component":      "_table/users",
				"verb_mask":      model.VerbGet,
				"requestor_mask": model.RequestorAPI,
				"filters":        []interface{}{},
				"filter_op":      "AND",
			},
			{
				"service_name":   "mydb",
				"component":      "_table/orders",
				"verb_mask":      model.VerbAll,
				"requestor_mask": model.RequestorAPI,
				"filters":        []interface{}{},
				"filter_op":      "AND",
			},
		},
	})
	rr := env.do(t, "PUT", fmt.Sprintf("/api/v1/system/role/%d", role.ID), body)
	assertStatus(t, rr, http.StatusOK)

	var resp map[string]interface{}
	decodeJSON(t, rr, &resp)
	access, ok := resp["access"].([]interface{})
	if !ok {
		t.Fatal("expected access to be an array")
	}
	if len(access) != 2 {
		t.Errorf("access count = %d, want 2", len(access))
	}
}

func TestGetRole_InvalidID(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/api/v1/system/role/notanumber", nil)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestGetRole_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/api/v1/system/role/99999", nil)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestUpdateRole_NotFound(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"name":      "ghost",
		"is_active": true,
	})
	rr := env.do(t, "PUT", "/api/v1/system/role/99999", body)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestDeleteRole_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/role/99999", nil)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestDeleteRole_InvalidID(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/role/abc", nil)
	assertStatus(t, rr, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Admin CRUD
// ---------------------------------------------------------------------------

func TestAdminCRUD(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	// --- List ---
	rr := env.do(t, "GET", "/api/v1/system/admin", nil)
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
	// Password hash must NOT be exposed.
	if _, exists := listResp.Resource[0]["password_hash"]; exists {
		t.Error("password_hash should not be exposed in admin list response")
	}

	// --- Create second admin ---
	createBody := toJSON(t, map[string]interface{}{
		"email":    "admin2@example.com",
		"password": "anothersecretpassword",
		"name":     "Second Admin",
	})
	rr = env.do(t, "POST", "/api/v1/system/admin", createBody)
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
	rr = env.do(t, "GET", "/api/v1/system/admin", nil)
	assertStatus(t, rr, http.StatusOK)
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 2 {
		t.Errorf("list count = %d, want 2", len(listResp.Resource))
	}
}

func TestCreateAdmin_Validation(t *testing.T) {
	env := newTestEnv(t)

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
			rr := env.do(t, "POST", "/api/v1/system/admin", toJSON(t, tt.body))
			assertStatus(t, rr, http.StatusBadRequest)
		})
	}
}

func TestCreateAdmin_DuplicateEmail(t *testing.T) {
	env := newTestEnv(t)
	env.seedAdmin(t)

	body := toJSON(t, map[string]interface{}{
		"email":    "admin@example.com",
		"password": "duplicatepassword",
		"name":     "Duplicate",
	})
	rr := env.do(t, "POST", "/api/v1/system/admin", body)
	assertStatus(t, rr, http.StatusConflict)
}

func TestCreateAdmin_NewAdminCanLogin(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"email":    "new@example.com",
		"password": "newadminpassword",
		"name":     "New Admin",
	})
	rr := env.do(t, "POST", "/api/v1/system/admin", body)
	assertStatus(t, rr, http.StatusCreated)

	// Newly created admin should be able to log in.
	loginBody := toJSON(t, map[string]string{
		"email":    "new@example.com",
		"password": "newadminpassword",
	})
	rr = env.do(t, "POST", "/api/v1/system/admin/session", loginBody)
	assertStatus(t, rr, http.StatusOK)
}

// ---------------------------------------------------------------------------
// API Key CRUD
// ---------------------------------------------------------------------------

func TestAPIKeyCRUD(t *testing.T) {
	env := newTestEnv(t)
	role := env.seedRole(t, "apitest")

	// --- Create API key ---
	createBody := toJSON(t, map[string]interface{}{
		"label":   "Test Key",
		"role_id": role.ID,
	})
	rr := env.do(t, "POST", "/api/v1/system/api-key", createBody)
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
	if keyResp.RoleID != role.ID {
		t.Errorf("role_id = %d, want %d", keyResp.RoleID, role.ID)
	}

	// --- List ---
	rr = env.do(t, "GET", "/api/v1/system/api-key", nil)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 1 {
		t.Fatalf("list count = %d, want 1", len(listResp.Resource))
	}
	// The raw key should NOT appear in list responses.
	if _, exists := listResp.Resource[0]["api_key"]; exists {
		t.Error("raw api_key should not appear in list response")
	}

	// --- Revoke ---
	revokeURL := fmt.Sprintf("/api/v1/system/api-key/%d", keyResp.ID)
	rr = env.do(t, "DELETE", revokeURL, nil)
	assertStatus(t, rr, http.StatusOK)

	var revokeResp map[string]interface{}
	decodeJSON(t, rr, &revokeResp)
	if revokeResp["success"] != true {
		t.Errorf("revoke success = %v, want true", revokeResp["success"])
	}
}

func TestCreateAPIKey_MissingRoleID(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"label": "No Role",
	})
	rr := env.do(t, "POST", "/api/v1/system/api-key", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCreateAPIKey_NonexistentRole(t *testing.T) {
	env := newTestEnv(t)

	body := toJSON(t, map[string]interface{}{
		"label":   "Bad Role",
		"role_id": 99999,
	})
	rr := env.do(t, "POST", "/api/v1/system/api-key", body)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestRevokeAPIKey_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/api-key/99999", nil)
	assertStatus(t, rr, http.StatusNotFound)
}

func TestRevokeAPIKey_InvalidID(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "DELETE", "/api/v1/system/api-key/notanumber", nil)
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestCreateAPIKey_MultipleKeys(t *testing.T) {
	env := newTestEnv(t)
	role := env.seedRole(t, "multikey")

	// Create two API keys for the same role.
	for i := 0; i < 2; i++ {
		body := toJSON(t, map[string]interface{}{
			"label":   fmt.Sprintf("Key %d", i+1),
			"role_id": role.ID,
		})
		rr := env.do(t, "POST", "/api/v1/system/api-key", body)
		assertStatus(t, rr, http.StatusCreated)
	}

	// List should have 2 keys.
	rr := env.do(t, "GET", "/api/v1/system/api-key", nil)
	assertStatus(t, rr, http.StatusOK)

	var listResp struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &listResp)
	if len(listResp.Resource) != 2 {
		t.Errorf("list count = %d, want 2", len(listResp.Resource))
	}
}

// ---------------------------------------------------------------------------
// Error response format
// ---------------------------------------------------------------------------

func TestErrorResponseFormat(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "GET", "/api/v1/system/service/nonexistent", nil)
	assertStatus(t, rr, http.StatusNotFound)

	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeJSON(t, rr, &errResp)

	if errResp.Error.Code != 404 {
		t.Errorf("error.code = %d, want 404", errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("expected non-empty error.message")
	}
}

// ---------------------------------------------------------------------------
// Full workflow: create admin -> login -> create service -> create role ->
//               create API key -> list everything -> revoke key -> delete all
// ---------------------------------------------------------------------------

func TestFullWorkflow(t *testing.T) {
	env := newTestEnv(t)

	// Step 1: Create admin and login
	env.seedAdmin(t)
	loginBody := toJSON(t, map[string]string{
		"email":    "admin@example.com",
		"password": testPassword,
	})
	rr := env.do(t, "POST", "/api/v1/system/admin/session", loginBody)
	assertStatus(t, rr, http.StatusOK)

	// Step 2: Create a service
	svcBody := toJSON(t, map[string]interface{}{
		"name":   "demo",
		"label":  "Demo DB",
		"driver": "postgres",
		"dsn":    "postgres://localhost/demo",
	})
	rr = env.do(t, "POST", "/api/v1/system/service", svcBody)
	assertStatus(t, rr, http.StatusCreated)

	// Step 3: Create a role
	roleBody := toJSON(t, map[string]interface{}{
		"name":        "demo-reader",
		"description": "Read access to demo",
	})
	rr = env.do(t, "POST", "/api/v1/system/role", roleBody)
	assertStatus(t, rr, http.StatusCreated)

	var roleResp map[string]interface{}
	decodeJSON(t, rr, &roleResp)
	roleID := roleResp["id"]

	// Step 4: Create an API key
	keyBody := toJSON(t, map[string]interface{}{
		"label":   "demo-key",
		"role_id": roleID,
	})
	rr = env.do(t, "POST", "/api/v1/system/api-key", keyBody)
	assertStatus(t, rr, http.StatusCreated)

	var keyResp struct {
		ID  int64  `json:"id"`
		Key string `json:"api_key"`
	}
	decodeJSON(t, rr, &keyResp)
	if keyResp.Key == "" {
		t.Fatal("expected API key in response")
	}

	// Step 5: Revoke the API key
	rr = env.do(t, "DELETE", fmt.Sprintf("/api/v1/system/api-key/%d", keyResp.ID), nil)
	assertStatus(t, rr, http.StatusOK)

	// Step 6: Delete the service
	rr = env.do(t, "DELETE", "/api/v1/system/service/demo", nil)
	assertStatus(t, rr, http.StatusOK)

	// Verify the service is deleted.
	rr = env.do(t, "GET", "/api/v1/system/service", nil)
	assertStatus(t, rr, http.StatusOK)
	var svcList struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	decodeJSON(t, rr, &svcList)
	if len(svcList.Resource) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcList.Resource))
	}

	// Note: The role cannot be deleted while the API key row still references
	// it (FK constraint). The RevokeAPIKey endpoint only deactivates; it does
	// not delete the row. This is expected behavior.
}
