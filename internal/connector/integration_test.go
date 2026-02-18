package connector_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/mssql"
	"github.com/faucetdb/faucet/internal/connector/mysql"
	"github.com/faucetdb/faucet/internal/connector/postgres"
)

func TestMain(m *testing.M) {
	if os.Getenv("FAUCET_INTEGRATION") == "" {
		fmt.Println("skipping integration tests: set FAUCET_INTEGRATION=1 to run")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// DSN helpers
// ---------------------------------------------------------------------------

func mysqlDSN() string {
	return "corporate:O@aEq3Tk5#&S0AM#2##V@tcp(demo.mysql.dreamfactory.com:3306)/employees"
}

func postgresDSN() string {
	pass := url.QueryEscape("Ua*YD3JLNU#YcAID#v@1")
	return fmt.Sprintf("postgres://mysuperuser2:%s@198.199.73.149:5432/demo?sslmode=disable", pass)
}

func mssqlDSN() string {
	pass := url.QueryEscape("T3C3HHqMtxw%vb455555555")
	return fmt.Sprintf("sqlserver://admin:%s@sql-server.cmz2vpny0neq.us-east-1.rds.amazonaws.com:1433?database=wwi", pass)
}

// ---------------------------------------------------------------------------
// Helper: run a common suite of sub-tests against any connector
// ---------------------------------------------------------------------------

func runConnectorSuite(t *testing.T, conn connector.Connector, cfg connector.ConnectionConfig, knownTables []string, queryTable string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Connect ---
	t.Run("Connect", func(t *testing.T) {
		if err := conn.Connect(cfg); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
	})

	// All subsequent subtests depend on a successful connection.
	if conn.DB() == nil {
		t.Fatal("DB() is nil after Connect; aborting remaining subtests")
	}

	// --- Ping ---
	t.Run("Ping", func(t *testing.T) {
		if err := conn.Ping(ctx); err != nil {
			t.Fatalf("Ping failed: %v", err)
		}
	})

	// --- IntrospectSchema ---
	t.Run("IntrospectSchema", func(t *testing.T) {
		schema, err := conn.IntrospectSchema(ctx)
		if err != nil {
			t.Fatalf("IntrospectSchema failed: %v", err)
		}
		if schema == nil {
			t.Fatal("IntrospectSchema returned nil schema")
		}
		if len(schema.Tables) == 0 {
			t.Fatal("IntrospectSchema returned zero tables")
		}
		t.Logf("IntrospectSchema found %d tables", len(schema.Tables))
	})

	// --- GetTableNames ---
	t.Run("GetTableNames", func(t *testing.T) {
		names, err := conn.GetTableNames(ctx)
		if err != nil {
			t.Fatalf("GetTableNames failed: %v", err)
		}
		if len(names) == 0 {
			t.Fatal("GetTableNames returned zero names")
		}
		t.Logf("GetTableNames returned: %v", names)

		// Verify expected tables are present.
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[strings.ToLower(n)] = true
		}
		for _, expected := range knownTables {
			if !nameSet[strings.ToLower(expected)] {
				t.Errorf("expected table %q not found in %v", expected, names)
			}
		}
	})

	// --- IntrospectTable ---
	t.Run("IntrospectTable", func(t *testing.T) {
		table, err := conn.IntrospectTable(ctx, knownTables[0])
		if err != nil {
			t.Fatalf("IntrospectTable(%q) failed: %v", knownTables[0], err)
		}
		if table == nil {
			t.Fatal("IntrospectTable returned nil")
		}
		if len(table.Columns) == 0 {
			t.Fatal("IntrospectTable returned zero columns")
		}
		t.Logf("IntrospectTable(%q): %d columns", knownTables[0], len(table.Columns))
		for _, col := range table.Columns {
			t.Logf("  column: %s  type: %s  nullable: %v", col.Name, col.Type, col.Nullable)
		}
	})

	// --- BuildSelect + Query ---
	t.Run("BuildSelectAndQuery", func(t *testing.T) {
		req := connector.SelectRequest{
			Table: queryTable,
			Limit: 2,
		}
		query, args, err := conn.BuildSelect(ctx, req)
		if err != nil {
			t.Fatalf("BuildSelect failed: %v", err)
		}
		if query == "" {
			t.Fatal("BuildSelect returned empty query string")
		}
		t.Logf("BuildSelect SQL: %s  args: %v", query, args)

		rows, err := conn.DB().QueryxContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("query execution failed: %v", err)
		}
		defer rows.Close()

		var count int
		for rows.Next() {
			result := make(map[string]interface{})
			if err := rows.MapScan(result); err != nil {
				t.Fatalf("MapScan failed: %v", err)
			}
			count++
			t.Logf("  row %d: %v", count, result)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows iteration error: %v", err)
		}
		if count == 0 {
			t.Fatal("query returned zero rows")
		}
		if count > 2 {
			t.Errorf("expected at most 2 rows, got %d", count)
		}
	})

	// --- Disconnect ---
	t.Run("Disconnect", func(t *testing.T) {
		if err := conn.Disconnect(); err != nil {
			t.Fatalf("Disconnect failed: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Per-database integration tests
// ---------------------------------------------------------------------------

func TestPostgresIntegration(t *testing.T) {
	conn := postgres.New()
	cfg := connector.ConnectionConfig{
		Driver: "postgres",
		DSN:    postgresDSN(),
	}
	knownTables := []string{"account", "supplies"}
	runConnectorSuite(t, conn, cfg, knownTables, "account")
}

func TestMySQLIntegration(t *testing.T) {
	conn := mysql.New()
	cfg := connector.ConnectionConfig{
		Driver: "mysql",
		DSN:    mysqlDSN(),
	}
	knownTables := []string{"employees", "departments", "salaries"}
	runConnectorSuite(t, conn, cfg, knownTables, "employees")
}

func TestMSSQLIntegration(t *testing.T) {
	conn := mssql.New()
	cfg := connector.ConnectionConfig{
		Driver: "mssql",
		DSN:    mssqlDSN(),
	}
	knownTables := []string{"Customers", "Shipments"}
	runConnectorSuite(t, conn, cfg, knownTables, "Customers")
}

// ---------------------------------------------------------------------------
// Registry integration test
// ---------------------------------------------------------------------------

func TestRegistryIntegration(t *testing.T) {
	registry := connector.NewRegistry()

	// Register all three drivers.
	registry.RegisterDriver("postgres", func() connector.Connector { return postgres.New() })
	registry.RegisterDriver("mysql", func() connector.Connector { return mysql.New() })
	registry.RegisterDriver("mssql", func() connector.Connector { return mssql.New() })

	type svcDef struct {
		name   string
		driver string
		dsn    string
	}

	services := []svcDef{
		{"pg-demo", "postgres", postgresDSN()},
		{"mysql-employees", "mysql", mysqlDSN()},
		{"mssql-wwi", "mssql", mssqlDSN()},
	}

	// Connect all services.
	t.Run("ConnectAll", func(t *testing.T) {
		for _, svc := range services {
			t.Run(svc.name, func(t *testing.T) {
				err := registry.Connect(svc.name, connector.ConnectionConfig{
					Driver: svc.driver,
					DSN:    svc.dsn,
				})
				if err != nil {
					t.Fatalf("registry.Connect(%q) failed: %v", svc.name, err)
				}
			})
		}
	})

	// Verify all services are listed.
	t.Run("ListServices", func(t *testing.T) {
		list := registry.ListServices()
		if len(list) != len(services) {
			t.Fatalf("expected %d services, got %d: %v", len(services), len(list), list)
		}
		t.Logf("active services: %v", list)
	})

	// Ping each through the registry.
	t.Run("PingAll", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		for _, svc := range services {
			t.Run(svc.name, func(t *testing.T) {
				conn, err := registry.Get(svc.name)
				if err != nil {
					t.Fatalf("registry.Get(%q) failed: %v", svc.name, err)
				}
				if err := conn.Ping(ctx); err != nil {
					t.Fatalf("Ping via registry failed for %q: %v", svc.name, err)
				}
			})
		}
	})

	// Disconnect one and verify it's gone.
	t.Run("DisconnectOne", func(t *testing.T) {
		err := registry.Disconnect("pg-demo")
		if err != nil {
			t.Fatalf("registry.Disconnect(pg-demo) failed: %v", err)
		}
		_, err = registry.Get("pg-demo")
		if err == nil {
			t.Fatal("expected error after disconnecting pg-demo, got nil")
		}
		remaining := registry.ListServices()
		if len(remaining) != len(services)-1 {
			t.Errorf("expected %d services after disconnect, got %d", len(services)-1, len(remaining))
		}
	})

	// CloseAll remaining.
	t.Run("CloseAll", func(t *testing.T) {
		registry.CloseAll()
		if len(registry.ListServices()) != 0 {
			t.Error("expected zero services after CloseAll")
		}
	})
}
