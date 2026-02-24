package connector

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/faucetdb/faucet/internal/model"
)

// mockConnector implements Connector for testing without a real database.
type mockConnector struct {
	connected    bool
	disconnected bool
	cfg          ConnectionConfig
}

func (m *mockConnector) Connect(cfg ConnectionConfig) error {
	if cfg.DSN == "fail" {
		return fmt.Errorf("mock connect failure")
	}
	m.connected = true
	m.cfg = cfg
	return nil
}
func (m *mockConnector) Disconnect() error {
	m.disconnected = true
	m.connected = false
	return nil
}
func (m *mockConnector) Ping(_ context.Context) error                          { return nil }
func (m *mockConnector) DB() *sqlx.DB                                          { return nil }
func (m *mockConnector) BeginTx(_ context.Context, _ *sql.TxOptions) (*sqlx.Tx, error) {
	return nil, fmt.Errorf("mock: transactions not supported")
}
func (m *mockConnector) IntrospectSchema(_ context.Context) (*model.Schema, error) {
	return nil, nil
}
func (m *mockConnector) IntrospectTable(_ context.Context, _ string) (*model.TableSchema, error) {
	return nil, nil
}
func (m *mockConnector) GetTableNames(_ context.Context) ([]string, error) { return nil, nil }
func (m *mockConnector) GetStoredProcedures(_ context.Context) ([]model.StoredProcedure, error) {
	return nil, nil
}
func (m *mockConnector) BuildSelect(_ context.Context, _ SelectRequest) (string, []interface{}, error) {
	return "", nil, nil
}
func (m *mockConnector) BuildInsert(_ context.Context, _ InsertRequest) (string, []interface{}, error) {
	return "", nil, nil
}
func (m *mockConnector) BuildUpdate(_ context.Context, _ UpdateRequest) (string, []interface{}, error) {
	return "", nil, nil
}
func (m *mockConnector) BuildDelete(_ context.Context, _ DeleteRequest) (string, []interface{}, error) {
	return "", nil, nil
}
func (m *mockConnector) BuildCount(_ context.Context, _ CountRequest) (string, []interface{}, error) {
	return "", nil, nil
}
func (m *mockConnector) CreateTable(_ context.Context, _ model.TableSchema) error  { return nil }
func (m *mockConnector) AlterTable(_ context.Context, _ string, _ []SchemaChange) error {
	return nil
}
func (m *mockConnector) DropTable(_ context.Context, _ string) error { return nil }
func (m *mockConnector) CallProcedure(_ context.Context, _ string, _ map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, nil
}
func (m *mockConnector) DriverName() string              { return "mock" }
func (m *mockConnector) QuoteIdentifier(name string) string { return `"` + name + `"` }
func (m *mockConnector) SupportsReturning() bool         { return false }
func (m *mockConnector) SupportsUpsert() bool            { return false }
func (m *mockConnector) ParameterPlaceholder(_ int) string { return "?" }

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if len(r.ListServices()) != 0 {
		t.Error("new registry should have no services")
	}
}

func TestRegisterDriver(t *testing.T) {
	r := NewRegistry()
	r.RegisterDriver("mock", func() Connector { return &mockConnector{} })

	if _, ok := r.factories["mock"]; !ok {
		t.Error("expected mock driver to be registered")
	}
}

func TestConnectAndGet(t *testing.T) {
	r := NewRegistry()
	r.RegisterDriver("mock", func() Connector { return &mockConnector{} })

	err := r.Connect("test-svc", ConnectionConfig{Driver: "mock", DSN: "test-dsn"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	conn, err := r.Get("test-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connector")
	}

	mc := conn.(*mockConnector)
	if !mc.connected {
		t.Error("connector should be connected")
	}
	if mc.cfg.DSN != "test-dsn" {
		t.Errorf("expected DSN test-dsn, got %s", mc.cfg.DSN)
	}
}

func TestConnectUnsupportedDriver(t *testing.T) {
	r := NewRegistry()

	err := r.Connect("test-svc", ConnectionConfig{Driver: "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestConnectFailure(t *testing.T) {
	r := NewRegistry()
	r.RegisterDriver("mock", func() Connector { return &mockConnector{} })

	err := r.Connect("test-svc", ConnectionConfig{Driver: "mock", DSN: "fail"})
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
}

func TestConnectReplacesExisting(t *testing.T) {
	r := NewRegistry()
	var first *mockConnector
	r.RegisterDriver("mock", func() Connector {
		mc := &mockConnector{}
		if first == nil {
			first = mc
		}
		return mc
	})

	r.Connect("svc", ConnectionConfig{Driver: "mock", DSN: "dsn1"})
	r.Connect("svc", ConnectionConfig{Driver: "mock", DSN: "dsn2"})

	if !first.disconnected {
		t.Error("first connector should have been disconnected on replacement")
	}

	conn, _ := r.Get("svc")
	mc := conn.(*mockConnector)
	if mc.cfg.DSN != "dsn2" {
		t.Errorf("expected DSN dsn2 after replacement, got %s", mc.cfg.DSN)
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent service")
	}
}

func TestDisconnect(t *testing.T) {
	r := NewRegistry()
	r.RegisterDriver("mock", func() Connector { return &mockConnector{} })

	r.Connect("svc", ConnectionConfig{Driver: "mock", DSN: "dsn"})
	err := r.Disconnect("svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = r.Get("svc")
	if err == nil {
		t.Error("expected error after disconnect")
	}
}

func TestDisconnectNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Disconnect("nonexistent")
	if err == nil {
		t.Fatal("expected error for disconnecting nonexistent service")
	}
}

func TestCloseAll(t *testing.T) {
	r := NewRegistry()
	r.RegisterDriver("mock", func() Connector { return &mockConnector{} })

	r.Connect("svc1", ConnectionConfig{Driver: "mock", DSN: "dsn1"})
	r.Connect("svc2", ConnectionConfig{Driver: "mock", DSN: "dsn2"})

	r.CloseAll()

	if len(r.ListServices()) != 0 {
		t.Error("expected no services after CloseAll")
	}
}

func TestListServices(t *testing.T) {
	r := NewRegistry()
	r.RegisterDriver("mock", func() Connector { return &mockConnector{} })

	r.Connect("alpha", ConnectionConfig{Driver: "mock", DSN: "dsn"})
	r.Connect("beta", ConnectionConfig{Driver: "mock", DSN: "dsn"})

	services := r.ListServices()
	sort.Strings(services)

	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	if services[0] != "alpha" || services[1] != "beta" {
		t.Errorf("expected [alpha beta], got %v", services)
	}
}
