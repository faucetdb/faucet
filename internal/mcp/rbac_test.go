package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/sqlite"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/rbac"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const rbacTestService = "testdb"

// rbacTestEnv wires an MCPServer to an in-memory config store and an
// in-memory SQLite service with a "users" table and an "orders" table.
type rbacTestEnv struct {
	srv      *MCPServer
	store    *config.Store
	registry *connector.Registry
	roles    map[string]*model.Role
}

func newRBACTestEnv(t *testing.T) *rbacTestEnv {
	t.Helper()
	ctx := context.Background()

	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("config.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	registry := connector.NewRegistry()
	registry.RegisterDriver("sqlite", func() connector.Connector { return sqlite.New() })
	if err := registry.Connect(rbacTestService, connector.ConnectionConfig{
		Driver: "sqlite",
		DSN:    ":memory:",
	}); err != nil {
		t.Fatalf("registry.Connect: %v", err)
	}
	t.Cleanup(func() { registry.Disconnect(rbacTestService) })

	conn, _ := registry.Get(rbacTestService)
	if _, err := conn.DB().ExecContext(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		);
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			total REAL NOT NULL
		);
		INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com');
	`); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	// Service row so faucet_list_services / faucet_raw_sql have config.
	if err := store.CreateService(ctx, &model.ServiceConfig{
		Name:     rbacTestService,
		Driver:   "sqlite",
		DSN:      ":memory:",
		IsActive: true,
		RawSQL:   true,
	}); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	env := &rbacTestEnv{
		srv:      NewMCPServer(registry, store, slog.New(slog.NewTextHandler(io.Discard, nil))),
		store:    store,
		registry: registry,
		roles:    map[string]*model.Role{},
	}

	env.createRole(t, "readonly", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbGet},
	})
	env.createRole(t, "writer", true, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})
	env.createRole(t, "scoped", true, []model.RoleAccess{
		{ServiceName: rbacTestService, Component: "_table/users", VerbMask: model.VerbGet},
	})
	env.createRole(t, "norules", true, nil)
	env.createRole(t, "inactive", false, []model.RoleAccess{
		{ServiceName: "*", Component: "*", VerbMask: model.VerbAll},
	})
	env.createRole(t, "otherservice", true, []model.RoleAccess{
		{ServiceName: "does_not_exist", Component: "*", VerbMask: model.VerbAll},
	})
	return env
}

func (e *rbacTestEnv) createRole(t *testing.T, name string, active bool, access []model.RoleAccess) {
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
	e.roles[name] = role
}

// ctxFor returns a context carrying an API-key principal bound to the named
// role. The special names "admin" and "nil" yield an admin principal and no
// principal respectively.
func (e *rbacTestEnv) ctxFor(t *testing.T, roleName string) context.Context {
	t.Helper()
	switch roleName {
	case "nil":
		return context.Background()
	case "admin":
		return rbac.WithPrincipal(context.Background(), &rbac.Principal{Type: rbac.PrincipalAdmin, IsAdmin: true})
	}
	role, ok := e.roles[roleName]
	if !ok {
		t.Fatalf("unknown test role %q", roleName)
	}
	return rbac.WithPrincipal(context.Background(), &rbac.Principal{Type: rbac.PrincipalAPIKey, RoleID: role.ID})
}

func (e *rbacTestEnv) userCount(t *testing.T) int {
	t.Helper()
	conn, _ := e.registry.Get(rbacTestService)
	var n int
	if err := conn.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

func callReq(args map[string]interface{}) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

// expectForbidden returns a checker that fails the test unless the handler
// result is a tool error whose text starts with "Forbidden". It is curried
// so it can wrap a multi-value handler call: expectForbidden(t)(s.handleX(...)).
func expectForbidden(t *testing.T) func(res *mcp.CallToolResult, err error) {
	return func(res *mcp.CallToolResult, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("handler returned protocol error: %v", err)
		}
		if !res.IsError {
			t.Fatalf("expected IsError=true, got success: %s", resultText(t, res))
		}
		text := resultText(t, res)
		if !strings.HasPrefix(text, "Forbidden") {
			t.Fatalf("expected text starting with \"Forbidden\", got %q", text)
		}
	}
}

// expectOK returns a checker that fails the test unless the handler result
// is a successful (non-error) tool result.
func expectOK(t *testing.T) func(res *mcp.CallToolResult, err error) {
	return func(res *mcp.CallToolResult, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("handler returned protocol error: %v", err)
		}
		if res.IsError {
			t.Fatalf("expected success, got error: %s", resultText(t, res))
		}
	}
}

// toolCall is a table entry describing one tool invocation.
type toolCall struct {
	name string
	call func(ctx context.Context) (*mcp.CallToolResult, error)
}

func (e *rbacTestEnv) allTools(table string) []toolCall {
	s := e.srv
	return []toolCall{
		{"faucet_list_services", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleListServices(ctx, callReq(map[string]interface{}{}))
		}},
		{"faucet_list_tables", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleListTables(ctx, callReq(map[string]interface{}{"service": rbacTestService}))
		}},
		{"faucet_describe_table", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleDescribeTable(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": table}))
		}},
		{"faucet_query", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleQuery(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": table}))
		}},
		{"faucet_insert", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleInsert(ctx, callReq(map[string]interface{}{
				"service": rbacTestService, "table": table,
				"records": []interface{}{map[string]interface{}{"name": "Bob", "email": "bob@example.com"}},
			}))
		}},
		{"faucet_update", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleUpdate(ctx, callReq(map[string]interface{}{
				"service": rbacTestService, "table": table, "filter": "id = 1",
				"record": map[string]interface{}{"name": "Alicia"},
			}))
		}},
		{"faucet_delete", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleDelete(ctx, callReq(map[string]interface{}{
				"service": rbacTestService, "table": table, "filter": "id = 1",
			}))
		}},
		{"faucet_raw_sql", func(ctx context.Context) (*mcp.CallToolResult, error) {
			return s.handleRawSQL(ctx, callReq(map[string]interface{}{
				"service": rbacTestService, "sql": "SELECT 1",
			}))
		}},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestMCPRBAC_ReadOnlyRole(t *testing.T) {
	env := newRBACTestEnv(t)
	ctx := env.ctxFor(t, "readonly")
	s := env.srv

	expectOK(t)(s.handleQuery(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": "users"})))
	expectOK(t)(s.handleListTables(ctx, callReq(map[string]interface{}{"service": rbacTestService})))
	expectOK(t)(s.handleDescribeTable(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": "users"})))
	expectOK(t)(s.handleListServices(ctx, callReq(map[string]interface{}{})))

	before := env.userCount(t)
	for _, tc := range env.allTools("users") {
		switch tc.name {
		case "faucet_insert", "faucet_update", "faucet_delete", "faucet_raw_sql":
			t.Run(tc.name, func(t *testing.T) {
				expectForbidden(t)(tc.call(ctx))
			})
		}
	}
	if after := env.userCount(t); after != before {
		t.Fatalf("forbidden writes changed row count: before=%d after=%d", before, after)
	}
}

func TestMCPRBAC_WriterRole(t *testing.T) {
	env := newRBACTestEnv(t)
	ctx := env.ctxFor(t, "writer")

	before := env.userCount(t)
	for _, tc := range env.allTools("users") {
		t.Run(tc.name, func(t *testing.T) {
			expectOK(t)(tc.call(ctx))
		})
	}
	// insert (+1) and delete of id=1 (-1) net to the original count.
	if after := env.userCount(t); after != before {
		t.Fatalf("writer row count: before=%d after=%d", before, after)
	}
}

func TestMCPRBAC_ScopedRole(t *testing.T) {
	env := newRBACTestEnv(t)
	ctx := env.ctxFor(t, "scoped")
	s := env.srv

	expectOK(t)(s.handleQuery(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": "users"})))

	// Other tables are not covered by the "_table/users" rule.
	expectForbidden(t)(s.handleQuery(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": "orders"})))

	// The bare "_table" listing needs its own grant (or "_table/*").
	expectForbidden(t)(s.handleListTables(ctx, callReq(map[string]interface{}{"service": rbacTestService})))

	// "_schema/users" is a different component from "_table/users".
	expectForbidden(t)(s.handleDescribeTable(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": "users"})))

	// Writes to the granted table are still denied (GET only).
	before := env.userCount(t)
	expectForbidden(t)(s.handleInsert(ctx, callReq(map[string]interface{}{
		"service": rbacTestService, "table": "users",
		"records": []interface{}{map[string]interface{}{"name": "Bob", "email": "bob@example.com"}},
	})))
	if after := env.userCount(t); after != before {
		t.Fatalf("forbidden insert changed row count: before=%d after=%d", before, after)
	}

	// The role can see the service in listings since it has a rule on it.
	res, err := s.handleListServices(ctx, callReq(map[string]interface{}{}))
	expectOK(t)(res, err)
	if names := serviceNames(t, res); len(names) != 1 || names[0] != rbacTestService {
		t.Fatalf("scoped list_services = %v, want [%s]", names, rbacTestService)
	}
}

func TestMCPRBAC_DeniedPrincipals(t *testing.T) {
	env := newRBACTestEnv(t)

	for _, principal := range []string{"norules", "inactive", "nil"} {
		ctx := env.ctxFor(t, principal)
		before := env.userCount(t)
		for _, tc := range env.allTools("users") {
			t.Run(principal+"/"+tc.name, func(t *testing.T) {
				if principal == "norules" && tc.name == "faucet_list_services" {
					// Listing is filtered rather than refused: a role with
					// no rules simply sees no services.
					res, err := tc.call(ctx)
					expectOK(t)(res, err)
					if names := serviceNames(t, res); len(names) != 0 {
						t.Fatalf("norules list_services = %v, want empty", names)
					}
					return
				}
				expectForbidden(t)(tc.call(ctx))
			})
		}
		if after := env.userCount(t); after != before {
			t.Fatalf("%s: forbidden writes changed row count: before=%d after=%d", principal, before, after)
		}
	}
}

func TestMCPRBAC_AdminBypass(t *testing.T) {
	env := newRBACTestEnv(t)
	ctx := env.ctxFor(t, "admin")
	s := env.srv

	before := env.userCount(t)
	expectOK(t)(s.handleInsert(ctx, callReq(map[string]interface{}{
		"service": rbacTestService, "table": "users",
		"records": []interface{}{map[string]interface{}{"name": "Bob", "email": "bob@example.com"}},
	})))
	if after := env.userCount(t); after != before+1 {
		t.Fatalf("admin insert row count: before=%d after=%d", before, after)
	}
	expectOK(t)(s.handleRawSQL(ctx, callReq(map[string]interface{}{"service": rbacTestService, "sql": "SELECT 1"})))

	res, err := s.handleListServices(ctx, callReq(map[string]interface{}{}))
	expectOK(t)(res, err)
	if names := serviceNames(t, res); len(names) != 1 {
		t.Fatalf("admin list_services = %v, want one service", names)
	}
}

func TestMCPRBAC_ListServicesFiltered(t *testing.T) {
	env := newRBACTestEnv(t)
	s := env.srv

	// A role scoped to a service that does not exist sees nothing.
	res, err := s.handleListServices(env.ctxFor(t, "otherservice"), callReq(map[string]interface{}{}))
	expectOK(t)(res, err)
	if names := serviceNames(t, res); len(names) != 0 {
		t.Fatalf("otherservice list_services = %v, want empty", names)
	}

	// A role without rules sees an empty list as well.
	res, err = s.handleListServices(env.ctxFor(t, "norules"), callReq(map[string]interface{}{}))
	expectOK(t)(res, err)
	if names := serviceNames(t, res); len(names) != 0 {
		t.Fatalf("norules list_services = %v, want empty", names)
	}

	// Inactive role and missing principal are refused.
	expectForbidden(t)(s.handleListServices(env.ctxFor(t, "inactive"), callReq(map[string]interface{}{})))
	expectForbidden(t)(s.handleListServices(env.ctxFor(t, "nil"), callReq(map[string]interface{}{})))
}

func TestMCPRBAC_RawSQLRequiresAllVerbs(t *testing.T) {
	env := newRBACTestEnv(t)

	// A rule granting every verb but only on "_table/*" does not cover "_sql".
	env.createRole(t, "tablesonly", true, []model.RoleAccess{
		{ServiceName: "*", Component: "_table/*", VerbMask: model.VerbAll},
	})
	req := callReq(map[string]interface{}{"service": rbacTestService, "sql": "SELECT 1"})
	expectForbidden(t)(env.srv.handleRawSQL(env.ctxFor(t, "tablesonly"), req))

	// GET-only on "*" is not enough either.
	expectForbidden(t)(env.srv.handleRawSQL(env.ctxFor(t, "readonly"), req))

	// An explicit "_sql" rule with all verbs works.
	env.createRole(t, "sqlrole", true, []model.RoleAccess{
		{ServiceName: rbacTestService, Component: "_sql", VerbMask: model.VerbAll},
	})
	expectOK(t)(env.srv.handleRawSQL(env.ctxFor(t, "sqlrole"), req))
}

func TestMCPRBAC_Resources(t *testing.T) {
	env := newRBACTestEnv(t)
	s := env.srv

	servicesReq := mcp.ReadResourceRequest{}
	servicesReq.Params.URI = "faucet://services"
	schemaReq := mcp.ReadResourceRequest{}
	schemaReq.Params.URI = "faucet://schema/" + rbacTestService

	tests := []struct {
		principal    string
		wantServices bool // resource read succeeds
		wantSchema   bool
	}{
		{"admin", true, true},
		{"readonly", true, true},
		{"writer", true, true},
		{"scoped", true, false},  // has a rule on the service, but no "_schema" grant
		{"norules", true, false}, // listing is filtered to nothing, not refused
		{"inactive", false, false},
		{"nil", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.principal, func(t *testing.T) {
			ctx := env.ctxFor(t, tt.principal)

			_, err := s.handleServicesResource(ctx, servicesReq)
			if tt.wantServices && err != nil {
				t.Fatalf("services resource: unexpected error: %v", err)
			}
			if !tt.wantServices {
				if err == nil {
					t.Fatal("services resource: expected error, got nil")
				}
				if !errors.Is(err, rbac.ErrForbidden) {
					t.Fatalf("services resource: error %v does not wrap ErrForbidden", err)
				}
			}

			_, err = s.handleSchemaResource(ctx, schemaReq)
			if tt.wantSchema && err != nil {
				t.Fatalf("schema resource: unexpected error: %v", err)
			}
			if !tt.wantSchema {
				if err == nil {
					t.Fatal("schema resource: expected error, got nil")
				}
				if !errors.Is(err, rbac.ErrForbidden) {
					t.Fatalf("schema resource: error %v does not wrap ErrForbidden", err)
				}
			}
		})
	}

	// The filtered services resource is empty for a role scoped elsewhere.
	contents, err := s.handleServicesResource(env.ctxFor(t, "otherservice"), servicesReq)
	if err != nil {
		t.Fatalf("services resource: %v", err)
	}
	text := contents[0].(mcp.TextResourceContents).Text
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		t.Fatalf("unmarshal services resource: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("otherservice services resource = %v, want empty", items)
	}
}

func TestMCPRBAC_AuthorizeHelper(t *testing.T) {
	env := newRBACTestEnv(t)

	tests := []struct {
		name      string
		principal string
		component string
		verb      int
		wantNil   bool
		wantText  string
	}{
		{"admin bypass", "admin", "_table/users", model.VerbDelete, true, ""},
		{"readonly get", "readonly", "_table/users", model.VerbGet, true, ""},
		{"readonly post", "readonly", "_table/users", model.VerbPost, false, "Forbidden: Role \"readonly\" does not permit POST on testdb/_table/users"},
		{"nil principal", "nil", "_table/users", model.VerbGet, false, "Forbidden: Authentication required"},
		{"inactive role", "inactive", "_table/users", model.VerbGet, false, "Forbidden: Role \"inactive\" is inactive"},
		{"missing role", "missing", "_table/users", model.VerbGet, false, "Forbidden: API key is bound to a role that no longer exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx context.Context
			if tt.principal == "missing" {
				ctx = rbac.WithPrincipal(context.Background(), &rbac.Principal{Type: rbac.PrincipalAPIKey, RoleID: 999999})
			} else {
				ctx = env.ctxFor(t, tt.principal)
			}
			res := env.srv.authorize(ctx, rbacTestService, tt.component, tt.verb)
			if tt.wantNil {
				if res != nil {
					t.Fatalf("expected nil, got %q", resultText(t, res))
				}
				return
			}
			if res == nil {
				t.Fatal("expected denial, got nil")
			}
			if !res.IsError {
				t.Fatal("expected IsError=true")
			}
			if got := resultText(t, res); got != tt.wantText {
				t.Fatalf("text = %q, want %q", got, tt.wantText)
			}
		})
	}
}

// serviceNames decodes the names from a faucet_list_services result.
func serviceNames(t *testing.T, res *mcp.CallToolResult) []string {
	t.Helper()
	var items []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &items); err != nil {
		t.Fatalf("unmarshal list_services: %v", err)
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names
}

// ---------------------------------------------------------------------------
// Read-only services
// ---------------------------------------------------------------------------

// setServiceReadOnly flips the read_only flag on the test service.
func (e *rbacTestEnv) setServiceReadOnly(t *testing.T, readOnly bool) {
	t.Helper()
	ctx := context.Background()
	svc, err := e.store.GetServiceByName(ctx, rbacTestService)
	if err != nil {
		t.Fatalf("GetServiceByName: %v", err)
	}
	svc.ReadOnly = readOnly
	if err := e.store.UpdateService(ctx, svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
}

// expectToolError returns a checker that fails the test unless the handler
// result is a tool error whose text contains want.
func expectToolError(t *testing.T, want string) func(res *mcp.CallToolResult, err error) {
	return func(res *mcp.CallToolResult, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("handler returned protocol error: %v", err)
		}
		if !res.IsError {
			t.Fatalf("expected IsError=true, got success: %s", resultText(t, res))
		}
		if text := resultText(t, res); !strings.Contains(text, want) {
			t.Fatalf("error text = %q, want it to contain %q", text, want)
		}
	}
}

func TestMCPRBAC_RawSQLRefusedOnReadOnlyService(t *testing.T) {
	env := newRBACTestEnv(t)
	s := env.srv
	env.setServiceReadOnly(t, true)

	before := env.userCount(t)
	deleteReq := callReq(map[string]interface{}{"service": rbacTestService, "sql": "DELETE FROM users"})

	// An all-verbs role (and even an admin) cannot run raw SQL against a
	// read-only service, no matter what the statement is.
	for _, principal := range []string{"writer", "admin"} {
		t.Run(principal, func(t *testing.T) {
			ctx := env.ctxFor(t, principal)
			expectToolError(t, "read-only")(s.handleRawSQL(ctx, deleteReq))
			expectToolError(t, "read-only")(s.handleRawSQL(ctx, callReq(map[string]interface{}{
				"service": rbacTestService, "sql": "DROP TABLE users",
			})))
			expectToolError(t, "read-only")(s.handleRawSQL(ctx, callReq(map[string]interface{}{
				"service": rbacTestService, "sql": "SELECT 1",
			})))
		})
	}
	if after := env.userCount(t); after != before {
		t.Fatalf("raw SQL on read-only service changed row count: before=%d after=%d", before, after)
	}

	// The structured mutation tools are refused the same way.
	ctx := env.ctxFor(t, "writer")
	for _, tc := range env.allTools("users") {
		switch tc.name {
		case "faucet_insert", "faucet_update", "faucet_delete":
			t.Run(tc.name, func(t *testing.T) {
				expectToolError(t, "read-only")(tc.call(ctx))
			})
		}
	}
	if after := env.userCount(t); after != before {
		t.Fatalf("writes on read-only service changed row count: before=%d after=%d", before, after)
	}

	// Flipping the flag back restores raw SQL for the writer role.
	env.setServiceReadOnly(t, false)
	expectOK(t)(s.handleRawSQL(ctx, callReq(map[string]interface{}{"service": rbacTestService, "sql": "SELECT 1"})))
}

func TestMCPRBAC_ReadOnlyGuardFailsClosed(t *testing.T) {
	env := newRBACTestEnv(t)
	s := env.srv
	before := env.userCount(t)

	// Close the config store so the read-only lookup fails with an error
	// other than "not found". An admin principal never touches the store
	// for authorization, so the only thing standing between it and the
	// write is the read-only guard, which must fail closed.
	if err := env.store.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}
	ctx := env.ctxFor(t, "admin")

	for _, tc := range env.allTools("users") {
		switch tc.name {
		case "faucet_insert", "faucet_update", "faucet_delete", "faucet_raw_sql":
			t.Run(tc.name, func(t *testing.T) {
				expectToolError(t, "Authorization check failed")(tc.call(ctx))
			})
		}
	}
	if after := env.userCount(t); after != before {
		t.Fatalf("writes during store failure changed row count: before=%d after=%d", before, after)
	}

	// The raw_sql read-only check runs before the statement is even read.
	expectToolError(t, "Authorization check failed")(s.handleRawSQL(ctx, callReq(map[string]interface{}{
		"service": rbacTestService, "sql": "DELETE FROM users",
	})))
}

// ---------------------------------------------------------------------------
// Service-name disclosure
// ---------------------------------------------------------------------------

func TestMCPRBAC_NoServiceDisclosure(t *testing.T) {
	env := newRBACTestEnv(t)
	s := env.srv

	// A role scoped entirely to another service must never learn that
	// "testdb" exists from any error message.
	env.createRole(t, "otherdb", true, []model.RoleAccess{
		{ServiceName: "otherdb", Component: "*", VerbMask: model.VerbAll},
	})
	otherCtx := env.ctxFor(t, "otherdb")

	assertNoLeak := func(t *testing.T, res *mcp.CallToolResult, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("handler returned protocol error: %v", err)
		}
		if !res.IsError {
			t.Fatalf("expected IsError=true, got success: %s", resultText(t, res))
		}
		if text := resultText(t, res); strings.Contains(text, "Available services") {
			t.Fatalf("error text carries a service listing: %q", text)
		}
	}

	t.Run("forbidden query names no other services", func(t *testing.T) {
		res, err := s.handleQuery(otherCtx, callReq(map[string]interface{}{"service": rbacTestService, "table": "users"}))
		expectForbidden(t)(res, err)
		assertNoLeak(t, res, err)
	})

	t.Run("unknown service", func(t *testing.T) {
		res, err := s.handleQuery(otherCtx, callReq(map[string]interface{}{"service": "nope", "table": "users"}))
		expectForbidden(t)(res, err)
		assertNoLeak(t, res, err)
		if text := resultText(t, res); strings.Contains(text, rbacTestService) {
			t.Fatalf("error text leaks %q: %s", rbacTestService, text)
		}
	})

	t.Run("missing service argument", func(t *testing.T) {
		for _, call := range []func(ctx context.Context) (*mcp.CallToolResult, error){
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleListTables(ctx, callReq(map[string]interface{}{}))
			},
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleDescribeTable(ctx, callReq(map[string]interface{}{"table": "users"}))
			},
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleQuery(ctx, callReq(map[string]interface{}{"table": "users"}))
			},
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleInsert(ctx, callReq(map[string]interface{}{"table": "users"}))
			},
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleUpdate(ctx, callReq(map[string]interface{}{"table": "users"}))
			},
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleDelete(ctx, callReq(map[string]interface{}{"table": "users"}))
			},
			func(ctx context.Context) (*mcp.CallToolResult, error) {
				return s.handleRawSQL(ctx, callReq(map[string]interface{}{"sql": "SELECT 1"}))
			},
		} {
			for _, principal := range []string{"otherdb", "norules", "inactive", "nil"} {
				res, err := call(env.ctxFor(t, principal))
				assertNoLeak(t, res, err)
				if text := resultText(t, res); strings.Contains(text, rbacTestService) {
					t.Fatalf("%s: error text leaks %q: %s", principal, rbacTestService, text)
				}
			}

			// Principals that can see the service still get the hint.
			for _, principal := range []string{"admin", "writer", "scoped"} {
				res, err := call(env.ctxFor(t, principal))
				if err != nil {
					t.Fatalf("%s: handler returned protocol error: %v", principal, err)
				}
				if text := resultText(t, res); !strings.Contains(text, "Available services: ["+rbacTestService+"]") {
					t.Fatalf("%s: error text lacks service hint: %q", principal, text)
				}
			}
		}
	})

	t.Run("schema resource", func(t *testing.T) {
		req := mcp.ReadResourceRequest{}
		req.Params.URI = "faucet://schema/nope"

		_, err := s.handleSchemaResource(otherCtx, req)
		if err == nil {
			t.Fatal("expected error for unknown service, got nil")
		}
		if !errors.Is(err, rbac.ErrForbidden) {
			t.Fatalf("error %v does not wrap ErrForbidden", err)
		}
		if strings.Contains(err.Error(), rbacTestService) {
			t.Fatalf("resource error leaks %q: %v", rbacTestService, err)
		}

		// The writer role may access "nope" by wildcard, so it reaches the
		// registry lookup and is offered the services it can see.
		_, err = s.handleSchemaResource(env.ctxFor(t, "writer"), req)
		if err == nil {
			t.Fatal("expected error for unknown service, got nil")
		}
		if !strings.Contains(err.Error(), "Available services: ["+rbacTestService+"]") {
			t.Fatalf("resource error lacks service hint: %v", err)
		}
	})
}

func TestMCPRBAC_ServiceHint(t *testing.T) {
	env := newRBACTestEnv(t)
	s := env.srv

	tests := []struct {
		principal string
		want      string
	}{
		{"admin", " Available services: [" + rbacTestService + "]"},
		{"writer", " Available services: [" + rbacTestService + "]"},
		{"scoped", " Available services: [" + rbacTestService + "]"},
		{"otherservice", ""},
		{"norules", ""},
		{"inactive", ""},
		{"nil", ""},
	}
	for _, tt := range tests {
		t.Run(tt.principal, func(t *testing.T) {
			if got := s.serviceHint(env.ctxFor(t, tt.principal)); got != tt.want {
				t.Fatalf("serviceHint = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Verb and component mapping
// ---------------------------------------------------------------------------

// TestMCPRBAC_ExactVerbAndComponentMapping pins each tool to exactly one
// HTTP verb and component so a mix-up (e.g. faucet_update checking PUT
// instead of PATCH) cannot go unnoticed.
func TestMCPRBAC_ExactVerbAndComponentMapping(t *testing.T) {
	// runRole grants a single rule and asserts that exactly the tools in
	// allowed succeed while every other tool is Forbidden. Each role gets a
	// fresh environment so the mutating tools start from the same rows.
	runRole := func(t *testing.T, name string, rule model.RoleAccess, allowed ...string) {
		t.Helper()
		env := newRBACTestEnv(t)
		env.createRole(t, name, true, []model.RoleAccess{rule})
		ctx := env.ctxFor(t, name)

		isAllowed := func(tool string) bool {
			for _, a := range allowed {
				if a == tool {
					return true
				}
			}
			return false
		}

		for _, tc := range env.allTools("users") {
			t.Run(tc.name, func(t *testing.T) {
				if tc.name == "faucet_list_services" {
					// Listing is filtered, never refused; the rule makes
					// the service visible.
					res, err := tc.call(ctx)
					expectOK(t)(res, err)
					if names := serviceNames(t, res); len(names) != 1 || names[0] != rbacTestService {
						t.Fatalf("list_services = %v, want [%s]", names, rbacTestService)
					}
					return
				}
				if isAllowed(tc.name) {
					expectOK(t)(tc.call(ctx))
				} else {
					expectForbidden(t)(tc.call(ctx))
				}
			})
		}
	}

	// Single verbs on "_table/users".
	verbs := []struct {
		name string
		verb int
		tool string // "" when no MCP tool maps to the verb
	}{
		{"GET", model.VerbGet, "faucet_query"},
		{"POST", model.VerbPost, "faucet_insert"},
		{"PUT", model.VerbPut, ""},
		{"PATCH", model.VerbPatch, "faucet_update"},
		{"DELETE", model.VerbDelete, "faucet_delete"},
	}
	for _, v := range verbs {
		t.Run("table_users_"+v.name, func(t *testing.T) {
			rule := model.RoleAccess{ServiceName: rbacTestService, Component: "_table/users", VerbMask: v.verb}
			if v.tool == "" {
				runRole(t, "verb_"+v.name, rule)
			} else {
				runRole(t, "verb_"+v.name, rule, v.tool)
			}
		})
	}

	// GET on "_schema/users" unlocks only faucet_describe_table.
	t.Run("schema_users_GET", func(t *testing.T) {
		runRole(t, "schema_users",
			model.RoleAccess{ServiceName: rbacTestService, Component: "_schema/users", VerbMask: model.VerbGet},
			"faucet_describe_table")
	})

	// GET on the bare "_table" unlocks only faucet_list_tables.
	t.Run("table_GET", func(t *testing.T) {
		runRole(t, "table_list",
			model.RoleAccess{ServiceName: rbacTestService, Component: "_table", VerbMask: model.VerbGet},
			"faucet_list_tables")
	})

	// Even all verbs on "_table/users" do not reach _table, _schema/users
	// or _sql.
	t.Run("table_users_ALL", func(t *testing.T) {
		runRole(t, "table_users_all",
			model.RoleAccess{ServiceName: rbacTestService, Component: "_table/users", VerbMask: model.VerbAll},
			"faucet_query", "faucet_insert", "faucet_update", "faucet_delete")
	})
}

// ---------------------------------------------------------------------------
// Stdio transport
// ---------------------------------------------------------------------------

// stdioToolCall drives stdio over in-memory pipes: it sends an initialize
// request followed by a tools/call for faucet_insert and returns the tool
// result. It fails the test if the transport misbehaves.
func stdioToolCall(t *testing.T, stdio *server.StdioServer) (isError bool, text string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	listenErr := make(chan error, 1)
	go func() { listenErr <- stdio.Listen(ctx, inR, outW) }()
	t.Cleanup(func() {
		cancel()
		inW.Close()
		outR.Close()
		select {
		case err := <-listenErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Listen returned: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Listen did not return after cancel")
		}
	})

	out := bufio.NewReader(outR)

	// send writes one JSON-RPC message line.
	send := func(msg map[string]interface{}) {
		t.Helper()
		b, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		if _, err := inW.Write(append(b, '\n')); err != nil {
			t.Fatalf("write request: %v", err)
		}
	}

	// waitFor reads responses until one carries the given id.
	type rpcResponse struct {
		ID     json.Number     `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	waitFor := func(id string) rpcResponse {
		t.Helper()
		type read struct {
			line string
			err  error
		}
		for {
			ch := make(chan read, 1)
			go func() {
				line, err := out.ReadString('\n')
				ch <- read{line, err}
			}()
			var r read
			select {
			case r = <-ch:
			case <-time.After(10 * time.Second):
				t.Fatalf("timed out waiting for response id %s", id)
			}
			if r.err != nil {
				t.Fatalf("read response: %v", r.err)
			}
			var resp rpcResponse
			if err := json.Unmarshal([]byte(r.line), &resp); err != nil {
				t.Fatalf("decode response %q: %v", r.line, err)
			}
			if resp.ID.String() == id {
				if len(resp.Error) > 0 {
					t.Fatalf("response %s is a JSON-RPC error: %s", id, resp.Error)
				}
				return resp
			}
		}
	}

	send(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "stdio-test", "version": "1.0.0"},
		},
	})
	waitFor("1")
	send(map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"})

	send(map[string]interface{}{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]interface{}{
			"name": "faucet_insert",
			"arguments": map[string]interface{}{
				"service": rbacTestService,
				"table":   "users",
				"records": []interface{}{map[string]interface{}{"name": "Bob", "email": "bob@example.com"}},
			},
		},
	})
	resp := waitFor("2")

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tool result %s: %v", resp.Result, err)
	}
	if len(result.Content) > 0 {
		text = result.Content[0].Text
	}
	return result.IsError, text
}

func TestMCPServer_StdioInjectsAdminPrincipal(t *testing.T) {
	// The context function alone must yield an admin principal.
	p := rbac.PrincipalFromContext(stdioAdminContext(context.Background()))
	if p == nil || !p.IsAdmin || p.Type != rbac.PrincipalAdmin {
		t.Fatalf("stdioAdminContext principal = %+v, want admin", p)
	}

	t.Run("newStdioServer runs tools as admin", func(t *testing.T) {
		env := newRBACTestEnv(t)
		before := env.userCount(t)

		isError, text := stdioToolCall(t, env.srv.newStdioServer())
		if isError {
			t.Fatalf("faucet_insert over stdio failed: %s", text)
		}
		if after := env.userCount(t); after != before+1 {
			t.Fatalf("stdio insert row count: before=%d after=%d", before, after)
		}
	})

	t.Run("bare stdio transport is refused", func(t *testing.T) {
		// Without the context function no principal is present, which
		// proves the injection above is what grants access.
		env := newRBACTestEnv(t)
		before := env.userCount(t)

		isError, text := stdioToolCall(t, server.NewStdioServer(env.srv.Server()))
		if !isError || !strings.HasPrefix(text, "Forbidden") {
			t.Fatalf("faucet_insert without principal: isError=%v text=%q, want Forbidden", isError, text)
		}
		if after := env.userCount(t); after != before {
			t.Fatalf("forbidden stdio insert changed row count: before=%d after=%d", before, after)
		}
	})
}

// TestMCPRBAC_TableNameMustBeIdentifier guards against a table argument that
// authorizes as one component but would reach the connector as another
// (e.g. "users/orders" normalizes to "_table/users" for RBAC).
func TestMCPRBAC_TableNameMustBeIdentifier(t *testing.T) {
	env := newRBACTestEnv(t)
	env.createRole(t, "scoped-users", true, []model.RoleAccess{
		{ServiceName: rbacTestService, Component: "_table/users", VerbMask: model.VerbGet},
	})
	ctx := env.ctxFor(t, "scoped-users")

	for _, table := range []string{"users/orders", "users/../orders", "users orders", "users;drop"} {
		res, err := env.srv.handleQuery(ctx, callReq(map[string]interface{}{"service": rbacTestService, "table": table}))
		if err != nil {
			t.Fatalf("handleQuery(%q): unexpected transport error: %v", table, err)
		}
		if res == nil || !res.IsError {
			t.Fatalf("handleQuery(%q): expected an error result, got %+v", table, res)
		}
		text, _ := res.Content[0].(mcp.TextContent)
		if !strings.Contains(text.Text, "Invalid table name") {
			t.Errorf("handleQuery(%q): error = %q, want invalid table name", table, text.Text)
		}
	}
}

// TestMCPRBAC_SchemaResourceDoesNotLeakServiceNames checks that a failed
// schema resource read never names services the role cannot see.
func TestMCPRBAC_SchemaResourceDoesNotLeakServiceNames(t *testing.T) {
	env := newRBACTestEnv(t)
	if err := env.registry.Connect("private_hr", connector.ConnectionConfig{Driver: "sqlite", DSN: ":memory:"}); err != nil {
		t.Fatalf("registry.Connect(private_hr): %v", err)
	}
	t.Cleanup(func() { env.registry.Disconnect("private_hr") })
	env.createRole(t, "prefix", true, []model.RoleAccess{
		{ServiceName: "test*", Component: "*", VerbMask: model.VerbGet},
	})

	req := mcp.ReadResourceRequest{}
	req.Params.URI = "faucet://schema/testzzz"
	_, err := env.srv.handleSchemaResource(env.ctxFor(t, "prefix"), req)
	if err == nil {
		t.Fatal("expected an error for an unknown service")
	}
	if strings.Contains(err.Error(), "private_hr") {
		t.Errorf("error leaks a service the role cannot see: %v", err)
	}
}
