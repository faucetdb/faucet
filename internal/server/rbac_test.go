package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/sqlite"
	"github.com/faucetdb/faucet/internal/model"
)

// ---------------------------------------------------------------------------
// RBAC end-to-end tests: role access rules (service / component / verb_mask)
// must be enforced for API-key requests on the data API. Before the fix,
// a read-only role could still write because role_access was never consulted.
// ---------------------------------------------------------------------------

const (
	rbacTable       = "items"
	rbacItemsPath   = "/api/v1/mydb/_table/items"
	rbacNoMatchFltr = "?filter=id%20%3D%20-99999"
)

// newRBACEnv creates a test server with a sqlite service "mydb" containing an
// "items" table with two rows. The service is registered both in the
// connector registry (for the handlers) and in the config store (for
// ReadOnlyGuard lookups).
func newRBACEnv(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	env.seedAdmin(t)
	env.registry.RegisterDriver("sqlite", func() connector.Connector { return sqlite.New() })
	env.addSQLiteService(t, "mydb", false)
	return env
}

// addSQLiteService connects an in-memory sqlite database under name, seeds
// the items table, and records the service in the config store.
func (e *testEnv) addSQLiteService(t *testing.T, name string, readOnly bool) {
	t.Helper()
	if err := e.registry.Connect(name, connector.ConnectionConfig{
		Driver: "sqlite",
		DSN:    ":memory:",
	}); err != nil {
		t.Fatalf("connect sqlite %s: %v", name, err)
	}
	t.Cleanup(func() { e.registry.Disconnect(name) })

	conn, err := e.registry.Get(name)
	if err != nil {
		t.Fatalf("registry.Get(%s): %v", name, err)
	}
	if _, err := conn.DB().Exec(`
		CREATE TABLE items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		);
		INSERT INTO items (name) VALUES ('alpha'), ('beta');
	`); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}

	svc := &model.ServiceConfig{
		Name:     name,
		Driver:   "sqlite",
		DSN:      ":memory:",
		ReadOnly: readOnly,
		IsActive: true,
		Pool:     model.DefaultPoolConfig(),
	}
	if err := e.store.CreateService(context.Background(), svc); err != nil {
		t.Fatalf("CreateService(%s): %v", name, err)
	}
}

// rbacKeyCounter makes every generated API key unique across tests.
var rbacKeyCounter int

// createRoleWithKey creates a role with the given rules and an API key bound
// to it. It returns the role and the raw key.
func (e *testEnv) createRoleWithKey(t *testing.T, name string, active bool, access []model.RoleAccess) (*model.Role, string) {
	t.Helper()
	ctx := context.Background()

	role := &model.Role{Name: name, IsActive: active}
	if err := e.store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole(%s): %v", name, err)
	}
	if len(access) > 0 {
		if err := e.store.SetRoleAccess(ctx, role.ID, access); err != nil {
			t.Fatalf("SetRoleAccess(%s): %v", name, err)
		}
	}

	rbacKeyCounter++
	rawKey := fmt.Sprintf("faucet_rbactest_%s_%04d_0123456789abcdef", name, rbacKeyCounter)
	apiKey := &model.APIKey{
		KeyHash:   config.HashAPIKey(rawKey),
		KeyPrefix: rawKey[:15],
		Label:     "rbac-" + name,
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := e.store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("CreateAPIKey(%s): %v", name, err)
	}
	return role, rawKey
}

func accessRule(service, component string, mask int) model.RoleAccess {
	return model.RoleAccess{ServiceName: service, Component: component, VerbMask: mask}
}

// assertForbidden checks that rr is a 403 in the standard error envelope and
// returns the decoded context map for further assertions.
func assertForbidden(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	assertStatus(t, rr, http.StatusForbidden)
	assertContentType(t, rr, "application/json")
	var resp model.ErrorResponse
	decodeJSON(t, rr, &resp)
	if resp.Error.Code != http.StatusForbidden {
		t.Errorf("error.code = %d, want 403", resp.Error.Code)
	}
	if resp.Error.Message == "" {
		t.Error("error.message should not be empty")
	}
	return resp.Error.Context
}

// countItems returns the number of rows in mydb.items via a full-access
// admin GET, so the count is independent of the key under test.
func (e *testEnv) countItems(t *testing.T, service string) int {
	t.Helper()
	token := e.adminToken(t)
	rr := e.doAuth(t, "GET", "/api/v1/"+service+"/_table/items", nil, token)
	assertStatus(t, rr, http.StatusOK)
	var resp model.ListResponse
	decodeJSON(t, rr, &resp)
	return len(resp.Resource)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRBAC_ReadOnlyRole_BugReport(t *testing.T) {
	env := newRBACEnv(t)
	_, key := env.createRoleWithKey(t, "readonly", true, []model.RoleAccess{
		accessRule("*", "*", model.VerbGet),
	})

	// GET is allowed and sees both seeded rows.
	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	assertStatus(t, rr, http.StatusOK)
	var list model.ListResponse
	decodeJSON(t, rr, &list)
	if len(list.Resource) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(list.Resource))
	}

	// Every write verb must be refused before reaching the handler.
	rr = env.doAPIKey(t, "PATCH", rbacItemsPath+rbacNoMatchFltr, jsonBody(t, map[string]interface{}{"name": "x"}), key)
	ctx := assertForbidden(t, rr)
	if ctx["verb"] != "PATCH" {
		t.Errorf("context.verb = %v, want PATCH", ctx["verb"])
	}
	if ctx["role"] != "readonly" || ctx["service"] != "mydb" || ctx["component"] != "_table/items" {
		t.Errorf("unexpected denial context: %v", ctx)
	}

	rr = env.doAPIKey(t, "DELETE", rbacItemsPath+rbacNoMatchFltr, nil, key)
	ctx = assertForbidden(t, rr)
	if ctx["verb"] != "DELETE" {
		t.Errorf("context.verb = %v, want DELETE", ctx["verb"])
	}

	rr = env.doAPIKey(t, "POST", rbacItemsPath, jsonBody(t, map[string]interface{}{"name": "gamma"}), key)
	ctx = assertForbidden(t, rr)
	if ctx["verb"] != "POST" {
		t.Errorf("context.verb = %v, want POST", ctx["verb"])
	}

	rr = env.doAPIKey(t, "PUT", rbacItemsPath+rbacNoMatchFltr, jsonBody(t, map[string]interface{}{"name": "x"}), key)
	ctx = assertForbidden(t, rr)
	if ctx["verb"] != "PUT" {
		t.Errorf("context.verb = %v, want PUT", ctx["verb"])
	}

	// Nothing was written.
	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	assertStatus(t, rr, http.StatusOK)
	decodeJSON(t, rr, &list)
	if len(list.Resource) != 2 {
		t.Errorf("expected 2 rows after denied writes, got %d", len(list.Resource))
	}
	for _, row := range list.Resource {
		if name, _ := row["name"].(string); name == "x" || name == "gamma" {
			t.Errorf("denied write was applied: %v", row)
		}
	}
}

func TestRBAC_VerbMaskZero_DeniesGET(t *testing.T) {
	env := newRBACEnv(t)
	_, key := env.createRoleWithKey(t, "maskzero", true, []model.RoleAccess{
		accessRule("*", "*", 0),
	})
	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	ctx := assertForbidden(t, rr)
	if ctx["verb"] != "GET" {
		t.Errorf("context.verb = %v, want GET", ctx["verb"])
	}
}

func TestRBAC_NoRules_Denied(t *testing.T) {
	env := newRBACEnv(t)
	_, key := env.createRoleWithKey(t, "norules", true, nil)

	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		rr := env.doAPIKey(t, method, rbacItemsPath, nil, key)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403; body = %s", method, rr.Code, rr.Body.String())
		}
	}
	rr := env.doAPIKey(t, "GET", "/api/v1/mydb/_table", nil, key)
	assertForbidden(t, rr)
}

func TestRBAC_InactiveRole_Denied(t *testing.T) {
	env := newRBACEnv(t)
	_, key := env.createRoleWithKey(t, "sleeper", false, []model.RoleAccess{
		accessRule("*", "*", model.VerbAll),
	})
	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	ctx := assertForbidden(t, rr)
	if ctx["role"] != "sleeper" {
		t.Errorf("context.role = %v, want sleeper", ctx["role"])
	}
}

// TestRBAC_RoleDeactivated_Denied covers a key whose role is deactivated
// after the key was issued. api_keys.role_id references roles(id) without
// ON DELETE CASCADE, so an in-use role cannot actually be deleted; the
// deleted-role path is covered by TestRBAC_RoleDeleted_Denied below when
// the FK allows it.
func TestRBAC_RoleDeactivated_Denied(t *testing.T) {
	env := newRBACEnv(t)
	role, key := env.createRoleWithKey(t, "later-inactive", true, []model.RoleAccess{
		accessRule("*", "*", model.VerbAll),
	})

	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	assertStatus(t, rr, http.StatusOK)

	role.IsActive = false
	if err := env.store.UpdateRole(context.Background(), role); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}

	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	assertForbidden(t, rr)
}

func TestRBAC_AdminJWT_Bypasses(t *testing.T) {
	env := newRBACEnv(t)
	token := env.adminToken(t)

	rr := env.doAuth(t, "PATCH", rbacItemsPath+"?filter=id%20%3D%201",
		jsonBody(t, map[string]interface{}{"name": "renamed"}), token)
	assertStatus(t, rr, http.StatusOK)

	rr = env.doAuth(t, "GET", rbacItemsPath+"?filter=id%20%3D%201", nil, token)
	assertStatus(t, rr, http.StatusOK)
	var list model.ListResponse
	decodeJSON(t, rr, &list)
	if len(list.Resource) != 1 || list.Resource[0]["name"] != "renamed" {
		t.Errorf("admin PATCH was not applied: %v", list.Resource)
	}

	rr = env.doAuth(t, "DELETE", rbacItemsPath+"?filter=id%20%3D%202", nil, token)
	assertStatus(t, rr, http.StatusOK)
	if n := env.countItems(t, "mydb"); n != 1 {
		t.Errorf("expected 1 row after admin DELETE, got %d", n)
	}
}

func TestRBAC_ServiceScoped(t *testing.T) {
	env := newRBACEnv(t)
	_, otherKey := env.createRoleWithKey(t, "other-only", true, []model.RoleAccess{
		accessRule("otherdb", "*", model.VerbAll),
	})
	_, mydbKey := env.createRoleWithKey(t, "mydb-only", true, []model.RoleAccess{
		accessRule("mydb", "*", model.VerbAll),
	})

	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, otherKey)
	ctx := assertForbidden(t, rr)
	if ctx["service"] != "mydb" {
		t.Errorf("context.service = %v, want mydb", ctx["service"])
	}

	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, mydbKey)
	assertStatus(t, rr, http.StatusOK)

	// The mydb-only key is refused on a service it has no rule for, even
	// one that does not exist.
	rr = env.doAPIKey(t, "GET", "/api/v1/otherdb/_table/items", nil, mydbKey)
	assertForbidden(t, rr)
}

// TestRBAC_CaseSensitive verifies that service and component patterns must
// match the URL exactly: a rule for "MYDB" grants nothing on "mydb", and a
// rule for "_table/Items" grants nothing on "_table/items".
func TestRBAC_CaseSensitive(t *testing.T) {
	env := newRBACEnv(t)
	_, upperSvcKey := env.createRoleWithKey(t, "upper-service", true, []model.RoleAccess{
		accessRule("MYDB", "*", model.VerbAll),
	})
	_, upperTableKey := env.createRoleWithKey(t, "upper-table", true, []model.RoleAccess{
		accessRule("mydb", "_table/Items", model.VerbAll),
	})
	_, prefixKey := env.createRoleWithKey(t, "upper-prefix", true, []model.RoleAccess{
		accessRule("MY*", "_TABLE/*", model.VerbAll),
	})

	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, upperSvcKey)
	ctx := assertForbidden(t, rr)
	if ctx["service"] != "mydb" {
		t.Errorf("context.service = %v, want mydb", ctx["service"])
	}

	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, upperTableKey)
	ctx = assertForbidden(t, rr)
	if ctx["component"] != "_table/items" {
		t.Errorf("context.component = %v, want _table/items", ctx["component"])
	}

	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, prefixKey)
	assertForbidden(t, rr)

	// The exact spelling is still granted.
	_, exactKey := env.createRoleWithKey(t, "exact", true, []model.RoleAccess{
		accessRule("mydb", "_table/items", model.VerbGet),
	})
	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, exactKey)
	assertStatus(t, rr, http.StatusOK)
	// ... but not a differently-cased URL for the same rule.
	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_table/Items", nil, exactKey)
	assertForbidden(t, rr)
}

func TestRBAC_TableScoped(t *testing.T) {
	env := newRBACEnv(t)
	_, itemsKey := env.createRoleWithKey(t, "items-only", true, []model.RoleAccess{
		accessRule("mydb", "_table/items", model.VerbGet),
	})
	_, allTablesKey := env.createRoleWithKey(t, "all-tables", true, []model.RoleAccess{
		accessRule("mydb", "_table/*", model.VerbGet),
	})

	rr := env.doAPIKey(t, "GET", rbacItemsPath, nil, itemsKey)
	assertStatus(t, rr, http.StatusOK)

	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_table/nope", nil, itemsKey)
	ctx := assertForbidden(t, rr)
	if ctx["component"] != "_table/nope" {
		t.Errorf("context.component = %v, want _table/nope", ctx["component"])
	}

	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_table", nil, itemsKey)
	ctx = assertForbidden(t, rr)
	if ctx["component"] != "_table" {
		t.Errorf("context.component = %v, want _table", ctx["component"])
	}

	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_table", nil, allTablesKey)
	assertStatus(t, rr, http.StatusOK)
	var list model.ListResponse
	decodeJSON(t, rr, &list)
	if len(list.Resource) == 0 {
		t.Error("expected table listing to include items")
	}

	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, allTablesKey)
	assertStatus(t, rr, http.StatusOK)

	// "_table/*" does not extend to schema introspection.
	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_schema", nil, allTablesKey)
	assertForbidden(t, rr)
}

func TestRBAC_WriteRole_CanWrite(t *testing.T) {
	env := newRBACEnv(t)
	_, key := env.createRoleWithKey(t, "writer", true, []model.RoleAccess{
		accessRule("mydb", "_table/*", model.VerbGet|model.VerbPost|model.VerbPatch|model.VerbDelete),
	})

	// POST
	rr := env.doAPIKey(t, "POST", rbacItemsPath, jsonBody(t, map[string]interface{}{
		"resource": []map[string]interface{}{{"name": "gamma"}},
	}), key)
	assertStatus(t, rr, http.StatusCreated)
	if n := env.countItems(t, "mydb"); n != 3 {
		t.Errorf("expected 3 rows after POST, got %d", n)
	}

	// PATCH by filter
	rr = env.doAPIKey(t, "PATCH", rbacItemsPath+"?filter=name%20%3D%20'gamma'",
		jsonBody(t, map[string]interface{}{"name": "delta"}), key)
	assertStatus(t, rr, http.StatusOK)
	var resp model.ListResponse
	decodeJSON(t, rr, &resp)
	if resp.Meta == nil || resp.Meta.Count != 1 {
		t.Errorf("expected 1 updated row, got %+v", resp.Meta)
	}

	// DELETE by filter
	rr = env.doAPIKey(t, "DELETE", rbacItemsPath+"?filter=name%20%3D%20'delta'", nil, key)
	assertStatus(t, rr, http.StatusOK)
	if n := env.countItems(t, "mydb"); n != 2 {
		t.Errorf("expected 2 rows after DELETE, got %d", n)
	}

	// PUT is not in the mask and must still be refused.
	rr = env.doAPIKey(t, "PUT", rbacItemsPath+rbacNoMatchFltr, jsonBody(t, map[string]interface{}{"name": "x"}), key)
	ctx := assertForbidden(t, rr)
	if ctx["verb"] != "PUT" {
		t.Errorf("context.verb = %v, want PUT", ctx["verb"])
	}
}

func TestRBAC_RoleUpdate_TakesEffectImmediately(t *testing.T) {
	env := newRBACEnv(t)
	token := env.adminToken(t)
	role, key := env.createRoleWithKey(t, "promotable", true, []model.RoleAccess{
		accessRule("*", "*", model.VerbGet),
	})
	rolePath := fmt.Sprintf("/api/v1/system/role/%d", role.ID)
	patchBody := func() map[string]interface{} { return map[string]interface{}{"name": "x"} }

	rr := env.doAPIKey(t, "PATCH", rbacItemsPath+rbacNoMatchFltr, jsonBody(t, patchBody()), key)
	assertForbidden(t, rr)

	// Grant PATCH via the admin API.
	rr = env.doAuth(t, "PUT", rolePath, jsonBody(t, map[string]interface{}{
		"is_active": true,
		"access": []map[string]interface{}{
			{"service_name": "*", "component": "*", "verb_mask": model.VerbGet | model.VerbPatch},
		},
	}), token)
	assertStatus(t, rr, http.StatusOK)

	rr = env.doAPIKey(t, "PATCH", rbacItemsPath+rbacNoMatchFltr, jsonBody(t, patchBody()), key)
	assertStatus(t, rr, http.StatusOK)

	// Revoke PATCH again.
	rr = env.doAuth(t, "PUT", rolePath, jsonBody(t, map[string]interface{}{
		"is_active": true,
		"access": []map[string]interface{}{
			{"service_name": "*", "component": "*", "verb_mask": model.VerbGet},
		},
	}), token)
	assertStatus(t, rr, http.StatusOK)

	rr = env.doAPIKey(t, "PATCH", rbacItemsPath+rbacNoMatchFltr, jsonBody(t, patchBody()), key)
	assertForbidden(t, rr)

	// GET was never affected.
	rr = env.doAPIKey(t, "GET", rbacItemsPath, nil, key)
	assertStatus(t, rr, http.StatusOK)
}

func TestRBAC_ReadOnlyService_BlocksWrites(t *testing.T) {
	env := newRBACEnv(t)
	env.addSQLiteService(t, "rodb", true)
	token := env.adminToken(t)
	_, key := env.createRoleWithKey(t, "full", true, []model.RoleAccess{
		accessRule("*", "*", model.VerbAll),
	})
	const roPath = "/api/v1/rodb/_table/items"
	record := func() map[string]interface{} { return map[string]interface{}{"name": "gamma"} }

	// Full-access API key: writes refused by the read-only guard.
	rr := env.doAPIKey(t, "POST", roPath, jsonBody(t, record()), key)
	ctx := assertForbidden(t, rr)
	if ctx["service"] != "rodb" {
		t.Errorf("context.service = %v, want rodb", ctx["service"])
	}
	rr = env.doAPIKey(t, "PATCH", roPath+"?filter=id%20%3D%201", jsonBody(t, record()), key)
	assertForbidden(t, rr)
	rr = env.doAPIKey(t, "DELETE", roPath+"?filter=id%20%3D%201", nil, key)
	assertForbidden(t, rr)

	// Admin JWT bypasses RBAC but not the read-only flag.
	rr = env.doAuth(t, "POST", roPath, jsonBody(t, record()), token)
	ctx = assertForbidden(t, rr)
	if ctx["service"] != "rodb" {
		t.Errorf("context.service = %v, want rodb", ctx["service"])
	}

	// Reads still work for both.
	rr = env.doAPIKey(t, "GET", roPath, nil, key)
	assertStatus(t, rr, http.StatusOK)
	rr = env.doAuth(t, "GET", roPath, nil, token)
	assertStatus(t, rr, http.StatusOK)

	if n := env.countItems(t, "rodb"); n != 2 {
		t.Errorf("expected 2 rows in read-only service, got %d", n)
	}

	// The same key can still write to the writable service.
	rr = env.doAPIKey(t, "POST", rbacItemsPath, jsonBody(t, record()), key)
	assertStatus(t, rr, http.StatusCreated)
}

func TestRBAC_SchemaAndProcComponents(t *testing.T) {
	env := newRBACEnv(t)
	_, key := env.createRoleWithKey(t, "schema-reader", true, []model.RoleAccess{
		accessRule("*", "*", model.VerbGet),
	})

	rr := env.doAPIKey(t, "GET", "/api/v1/mydb/_schema", nil, key)
	if rr.Code == http.StatusForbidden || rr.Code == http.StatusUnauthorized {
		t.Fatalf("GET /_schema: status = %d, want not 401/403; body = %s", rr.Code, rr.Body.String())
	}
	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_schema/items", nil, key)
	if rr.Code == http.StatusForbidden || rr.Code == http.StatusUnauthorized {
		t.Fatalf("GET /_schema/items: status = %d, want not 401/403; body = %s", rr.Code, rr.Body.String())
	}
	rr = env.doAPIKey(t, "GET", "/api/v1/mydb/_proc", nil, key)
	if rr.Code == http.StatusForbidden || rr.Code == http.StatusUnauthorized {
		t.Fatalf("GET /_proc: status = %d, want not 401/403; body = %s", rr.Code, rr.Body.String())
	}

	rr = env.doAPIKey(t, "POST", "/api/v1/mydb/_schema", jsonBody(t, map[string]interface{}{
		"name": "evil", "columns": []map[string]interface{}{{"name": "id", "type": "integer"}},
	}), key)
	ctx := assertForbidden(t, rr)
	if ctx["component"] != "_schema" || ctx["verb"] != "POST" {
		t.Errorf("unexpected denial context: %v", ctx)
	}

	rr = env.doAPIKey(t, "DELETE", "/api/v1/mydb/_schema/items", nil, key)
	ctx = assertForbidden(t, rr)
	if ctx["component"] != "_schema/items" || ctx["verb"] != "DELETE" {
		t.Errorf("unexpected denial context: %v", ctx)
	}

	rr = env.doAPIKey(t, "POST", "/api/v1/mydb/_proc/anything", jsonBody(t, map[string]interface{}{}), key)
	ctx = assertForbidden(t, rr)
	if ctx["component"] != "_proc/anything" || ctx["verb"] != "POST" {
		t.Errorf("unexpected denial context: %v", ctx)
	}

	// The table is still there.
	if n := env.countItems(t, "mydb"); n != 2 {
		t.Errorf("expected 2 rows after denied DDL, got %d", n)
	}
}
