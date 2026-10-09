package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	mcpClient "github.com/mark3labs/mcp-go/client"
	mcpTransport "github.com/mark3labs/mcp-go/client/transport"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/sqlite"
	"github.com/faucetdb/faucet/internal/model"
)

// ---------------------------------------------------------------------------
// MCP RBAC end-to-end tests: API-key clients calling tools through the real
// router must be subject to the same role access rules as REST clients.
// ---------------------------------------------------------------------------

const mcpRBACService = "demo"

// setupMCPRBACEnv wires an in-memory SQLite service named "demo" (with a
// "users" table and matching service row) into a fresh test environment.
func setupMCPRBACEnv(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	env.seedAdmin(t)
	ctx := context.Background()

	env.registry.RegisterDriver("sqlite", func() connector.Connector { return sqlite.New() })
	if err := env.registry.Connect(mcpRBACService, connector.ConnectionConfig{
		Driver: "sqlite",
		DSN:    ":memory:",
	}); err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	t.Cleanup(func() { env.registry.Disconnect(mcpRBACService) })

	conn, _ := env.registry.Get(mcpRBACService)
	if _, err := conn.DB().ExecContext(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		);
		INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com');
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := env.store.CreateService(ctx, &model.ServiceConfig{
		Name:     mcpRBACService,
		Driver:   "sqlite",
		DSN:      ":memory:",
		IsActive: true,
	}); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	return env
}

// createRoleWithKey creates a role with the given access rules and an
// active API key bound to it, returning the raw key.
func createRoleWithKey(t *testing.T, env *testEnv, roleName, rawKey string, access []model.RoleAccess) string {
	t.Helper()
	ctx := context.Background()

	role := &model.Role{Name: roleName, IsActive: true}
	if err := env.store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole(%s): %v", roleName, err)
	}
	if err := env.store.SetRoleAccess(ctx, role.ID, access); err != nil {
		t.Fatalf("SetRoleAccess(%s): %v", roleName, err)
	}
	apiKey := &model.APIKey{
		KeyHash:   config.HashAPIKey(rawKey),
		KeyPrefix: rawKey[:15],
		Label:     roleName,
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := env.store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("CreateAPIKey(%s): %v", roleName, err)
	}
	return rawKey
}

// newMCPAPIKeyClient connects an initialized Streamable HTTP MCP client to
// ts using X-API-Key authentication.
func newMCPAPIKeyClient(t *testing.T, ctx context.Context, url, rawKey string) *mcpClient.Client {
	t.Helper()
	c, err := mcpClient.NewStreamableHttpClient(
		url+"/mcp",
		mcpTransport.WithHTTPHeaders(map[string]string{"X-API-Key": rawKey}),
	)
	if err != nil {
		t.Fatalf("NewStreamableHttpClient: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "rbac-e2e", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

func callTool(t *testing.T, ctx context.Context, c *mcpClient.Client, name string, args map[string]interface{}) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	text := ""
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(mcp.TextContent); ok {
			text = tc.Text
		}
	}
	return res, text
}

func TestMCPEndpoint_E2E_RBACEnforced(t *testing.T) {
	env := setupMCPRBACEnv(t)

	readKey := createRoleWithKey(t, env, "mcp-reader", "faucet_mcp_rbac_reader_key_00001", []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbGet},
	})
	fullKey := createRoleWithKey(t, env, "mcp-writer", "faucet_mcp_rbac_writer_key_00001", []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})

	// Registered as a cleanup (not a defer) so the MCP clients created
	// below, whose cleanups run first, close before the server goes away.
	ts := httptest.NewServer(env.server.Router())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	queryArgs := map[string]interface{}{"service": mcpRBACService, "table": "users"}
	insertArgs := map[string]interface{}{
		"service": mcpRBACService,
		"table":   "users",
		"records": []interface{}{map[string]interface{}{"name": "Bob", "email": "bob@example.com"}},
	}
	deleteArgs := map[string]interface{}{"service": mcpRBACService, "table": "users", "filter": "id = 1"}

	t.Run("read-only key", func(t *testing.T) {
		c := newMCPAPIKeyClient(t, ctx, ts.URL, readKey)

		res, text := callTool(t, ctx, c, "faucet_query", queryArgs)
		if res.IsError {
			t.Fatalf("faucet_query with read-only key: unexpected error: %s", text)
		}
		if !strings.Contains(text, "alice@example.com") {
			t.Errorf("faucet_query result missing seeded row: %s", text)
		}

		res, text = callTool(t, ctx, c, "faucet_insert", insertArgs)
		if !res.IsError {
			t.Fatalf("faucet_insert with read-only key: expected IsError=true, got %s", text)
		}
		if !strings.Contains(text, "Forbidden") {
			t.Errorf("faucet_insert error text = %q, want it to contain \"Forbidden\"", text)
		}

		res, text = callTool(t, ctx, c, "faucet_delete", deleteArgs)
		if !res.IsError {
			t.Fatalf("faucet_delete with read-only key: expected IsError=true, got %s", text)
		}
		if !strings.Contains(text, "Forbidden") {
			t.Errorf("faucet_delete error text = %q, want it to contain \"Forbidden\"", text)
		}

		// Nothing was written.
		conn, _ := env.registry.Get(mcpRBACService)
		var n int
		if err := conn.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
			t.Fatalf("count users: %v", err)
		}
		if n != 1 {
			t.Errorf("users row count = %d after forbidden writes, want 1", n)
		}
	})

	t.Run("full-access key", func(t *testing.T) {
		c := newMCPAPIKeyClient(t, ctx, ts.URL, fullKey)

		res, text := callTool(t, ctx, c, "faucet_insert", insertArgs)
		if res.IsError {
			t.Fatalf("faucet_insert with full-access key: unexpected error: %s", text)
		}

		res, text = callTool(t, ctx, c, "faucet_query", queryArgs)
		if res.IsError {
			t.Fatalf("faucet_query with full-access key: unexpected error: %s", text)
		}
		if !strings.Contains(text, "bob@example.com") {
			t.Errorf("faucet_query result missing inserted row: %s", text)
		}
	})
}

func TestMCPEndpoint_E2E_RBACServiceListingFiltered(t *testing.T) {
	env := setupMCPRBACEnv(t)

	// This role only has rules on a service that does not exist, so it
	// must not see "demo" in listings, and must not be able to query it.
	scopedKey := createRoleWithKey(t, env, "mcp-elsewhere", "faucet_mcp_rbac_scoped_key_00001", []model.RoleAccess{
		{ServiceName: "other_service", Component: "*", VerbMask: model.VerbAll},
	})

	// Registered as a cleanup (not a defer) so the MCP clients created
	// below, whose cleanups run first, close before the server goes away.
	ts := httptest.NewServer(env.server.Router())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := newMCPAPIKeyClient(t, ctx, ts.URL, scopedKey)

	res, text := callTool(t, ctx, c, "faucet_list_services", map[string]interface{}{})
	if res.IsError {
		t.Fatalf("faucet_list_services: unexpected error: %s", text)
	}
	if strings.Contains(text, mcpRBACService) {
		t.Errorf("faucet_list_services leaked service %q to a role without access: %s", mcpRBACService, text)
	}

	res, text = callTool(t, ctx, c, "faucet_query", map[string]interface{}{"service": mcpRBACService, "table": "users"})
	if !res.IsError || !strings.Contains(text, "Forbidden") {
		t.Errorf("faucet_query on unlisted service: IsError=%v text=%q, want Forbidden", res.IsError, text)
	}

	// The services resource is filtered the same way; the schema resource
	// is refused outright.
	readRes, err := c.ReadResource(ctx, mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{URI: "faucet://services"}})
	if err != nil {
		t.Fatalf("ReadResource(faucet://services): %v", err)
	}
	for _, content := range readRes.Contents {
		if tc, ok := content.(mcp.TextResourceContents); ok && strings.Contains(tc.Text, mcpRBACService) {
			t.Errorf("faucet://services leaked service %q: %s", mcpRBACService, tc.Text)
		}
	}
	if _, err := c.ReadResource(ctx, mcp.ReadResourceRequest{Params: mcp.ReadResourceParams{URI: "faucet://schema/" + mcpRBACService}}); err == nil {
		t.Error("ReadResource(faucet://schema/demo): expected error for role without access, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "forbidden") {
		t.Errorf("ReadResource(faucet://schema/demo) error = %v, want forbidden", err)
	}
}

func TestMCPEndpoint_E2E_NoServiceDisclosure(t *testing.T) {
	env := setupMCPRBACEnv(t)

	// A role scoped entirely to another service must not learn that
	// "demo" exists from any tool error text.
	scopedKey := createRoleWithKey(t, env, "mcp-otherdb", "faucet_mcp_rbac_otherdb_key_0001", []model.RoleAccess{
		{ServiceName: "otherdb", Component: "*", VerbMask: model.VerbAll},
	})
	fullKey := createRoleWithKey(t, env, "mcp-full", "faucet_mcp_rbac_fullkey_key_0001", []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})

	ts := httptest.NewServer(env.server.Router())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Run("scoped key", func(t *testing.T) {
		c := newMCPAPIKeyClient(t, ctx, ts.URL, scopedKey)

		// Forbidden on the real service: no listing of other services.
		res, text := callTool(t, ctx, c, "faucet_query", map[string]interface{}{"service": mcpRBACService, "table": "users"})
		if !res.IsError || !strings.Contains(text, "Forbidden") {
			t.Fatalf("faucet_query: IsError=%v text=%q, want Forbidden", res.IsError, text)
		}
		if strings.Contains(text, "Available services") {
			t.Errorf("faucet_query denial carries a service listing: %s", text)
		}

		// Missing "service" argument: the hint must not name "demo".
		for _, tool := range []string{"faucet_list_tables", "faucet_describe_table", "faucet_query", "faucet_insert", "faucet_update", "faucet_delete", "faucet_raw_sql"} {
			res, text := callTool(t, ctx, c, tool, map[string]interface{}{"table": "users", "sql": "SELECT 1"})
			if !res.IsError {
				t.Fatalf("%s without service: expected IsError=true, got %s", tool, text)
			}
			if strings.Contains(text, mcpRBACService) {
				t.Errorf("%s without service leaked %q: %s", tool, mcpRBACService, text)
			}
		}

		// Unknown service: still nothing about "demo".
		res, text = callTool(t, ctx, c, "faucet_list_tables", map[string]interface{}{"service": "nope"})
		if !res.IsError {
			t.Fatalf("faucet_list_tables(nope): expected IsError=true, got %s", text)
		}
		if strings.Contains(text, mcpRBACService) {
			t.Errorf("faucet_list_tables(nope) leaked %q: %s", mcpRBACService, text)
		}
	})

	t.Run("full-access key keeps the hint", func(t *testing.T) {
		c := newMCPAPIKeyClient(t, ctx, ts.URL, fullKey)

		res, text := callTool(t, ctx, c, "faucet_list_tables", map[string]interface{}{})
		if !res.IsError {
			t.Fatalf("faucet_list_tables without service: expected IsError=true, got %s", text)
		}
		if !strings.Contains(text, "Available services: ["+mcpRBACService+"]") {
			t.Errorf("faucet_list_tables without service lacks hint: %s", text)
		}

		res, text = callTool(t, ctx, c, "faucet_list_tables", map[string]interface{}{"service": "nope"})
		if !res.IsError {
			t.Fatalf("faucet_list_tables(nope): expected IsError=true, got %s", text)
		}
		if !strings.Contains(text, "Available services: ["+mcpRBACService+"]") {
			t.Errorf("faucet_list_tables(nope) lacks hint: %s", text)
		}
	})
}

func TestMCPEndpoint_E2E_ReadOnlyServiceRefusesWrites(t *testing.T) {
	env := setupMCPRBACEnv(t)
	ctx := context.Background()

	// Flip the service to read-only with raw SQL enabled: an all-verbs key
	// must still be unable to mutate anything through any tool.
	svc, err := env.store.GetServiceByName(ctx, mcpRBACService)
	if err != nil {
		t.Fatalf("GetServiceByName: %v", err)
	}
	svc.ReadOnly = true
	svc.RawSQL = true
	if err := env.store.UpdateService(ctx, svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}

	fullKey := createRoleWithKey(t, env, "mcp-writer-ro", "faucet_mcp_rbac_writer_ro_key_01", []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})

	ts := httptest.NewServer(env.server.Router())
	t.Cleanup(ts.Close)

	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c := newMCPAPIKeyClient(t, callCtx, ts.URL, fullKey)

	calls := []struct {
		tool string
		args map[string]interface{}
	}{
		{"faucet_raw_sql", map[string]interface{}{"service": mcpRBACService, "sql": "DELETE FROM users"}},
		{"faucet_raw_sql", map[string]interface{}{"service": mcpRBACService, "sql": "DROP TABLE users"}},
		{"faucet_insert", map[string]interface{}{
			"service": mcpRBACService, "table": "users",
			"records": []interface{}{map[string]interface{}{"name": "Bob", "email": "bob@example.com"}},
		}},
		{"faucet_update", map[string]interface{}{
			"service": mcpRBACService, "table": "users", "filter": "id = 1",
			"record": map[string]interface{}{"name": "Mallory"},
		}},
		{"faucet_delete", map[string]interface{}{"service": mcpRBACService, "table": "users", "filter": "id = 1"}},
	}
	for _, call := range calls {
		res, text := callTool(t, callCtx, c, call.tool, call.args)
		if !res.IsError {
			t.Fatalf("%s on read-only service: expected IsError=true, got %s", call.tool, text)
		}
		if !strings.Contains(text, "read-only") {
			t.Errorf("%s error text = %q, want it to mention read-only", call.tool, text)
		}
	}

	// Reads still work, and nothing changed.
	res, text := callTool(t, callCtx, c, "faucet_query", map[string]interface{}{"service": mcpRBACService, "table": "users"})
	if res.IsError {
		t.Fatalf("faucet_query on read-only service: unexpected error: %s", text)
	}
	if !strings.Contains(text, "Alice") || strings.Contains(text, "Mallory") {
		t.Errorf("faucet_query result changed on read-only service: %s", text)
	}
	conn, _ := env.registry.Get(mcpRBACService)
	var n int
	if err := conn.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if n != 1 {
		t.Errorf("users row count = %d after refused writes, want 1", n)
	}
}

func TestPausedAndFailedServicesExplainThemselves(t *testing.T) {
	env := setupMCPRBACEnv(t)
	ctx := context.Background()
	fullKey := createRoleWithKey(t, env, "mcp-full", "faucet_mcp_rbac_fullkey_key_0002", []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})

	// Pause the service the way the admin API does.
	svc, err := env.store.GetServiceByName(ctx, mcpRBACService)
	if err != nil {
		t.Fatal(err)
	}
	svc.IsActive = false
	if err := env.store.UpdateService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	env.registry.Pause(mcpRBACService)

	rr := env.doAPIKey(t, "GET", "/api/v1/"+mcpRBACService+"/_table", nil, fullKey)
	assertStatus(t, rr, http.StatusServiceUnavailable)
	if !strings.Contains(rr.Body.String(), "is paused") {
		t.Errorf("REST body = %s, want a paused message", rr.Body.String())
	}

	ts := httptest.NewServer(env.server.Router())
	t.Cleanup(ts.Close)
	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c := newMCPAPIKeyClient(t, tctx, ts.URL, fullKey)
	res, text := callTool(t, tctx, c, "faucet_list_tables", map[string]interface{}{"service": mcpRBACService})
	if !res.IsError || !strings.Contains(text, "is paused") {
		t.Errorf("faucet_list_tables on a paused service: IsError=%v text=%q", res.IsError, text)
	}
	if strings.Contains(text, "Available services: ["+mcpRBACService+"]") {
		t.Errorf("hint lists the paused service as available: %s", text)
	}

	// A service that failed to connect makes /readyz degraded without
	// exposing the driver error.
	env.registry.Fail("broken", errors.New("dial tcp 10.0.0.9:5432: connection refused"))
	rr = env.do(t, "GET", "/readyz", nil, nil)
	assertStatus(t, rr, http.StatusServiceUnavailable)
	if !strings.Contains(rr.Body.String(), `"broken":"error: not connected"`) || strings.Contains(rr.Body.String(), "10.0.0.9") {
		t.Errorf("readyz body = %s", rr.Body.String())
	}
}
