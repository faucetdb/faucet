package handler

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newSQLiteFile creates a SQLite database with two tables and returns its path.
func newSQLiteFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE customers(id INTEGER PRIMARY KEY, name TEXT);
		CREATE TABLE orders(id INTEGER PRIMARY KEY, customer_id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateService_FromConnectionFields(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, map[string]interface{}{
		"name":   "pg",
		"driver": "postgres",
		"connection": map[string]interface{}{
			"host": "db.internal", "database": "app", "username": "api", "password": "p@ss#word",
		},
	}))
	assertStatus(t, rr, http.StatusCreated)

	var created map[string]interface{}
	decodeJSON(t, rr, &created)
	conn, ok := created["connection"].(map[string]interface{})
	if !ok {
		t.Fatalf("response has no connection summary: %v", created)
	}
	if conn["host"] != "db.internal" || conn["port"] != float64(5432) || conn["database"] != "app" || conn["username"] != "api" {
		t.Errorf("connection summary = %v", conn)
	}
	if _, leaked := conn["password"]; leaked {
		t.Error("password leaked in connection summary")
	}

	svc, err := env.store.GetServiceByName(t.Context(), "pg")
	if err != nil {
		t.Fatal(err)
	}
	if svc.DSN != "postgres://api:p%40ss%23word@db.internal:5432/app" {
		t.Errorf("stored DSN = %q", svc.DSN)
	}
}

func TestCreateService_InvalidConnectionFields(t *testing.T) {
	env := newTestEnv(t)
	rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, map[string]interface{}{
		"name": "pg", "driver": "postgres", "connection": map[string]interface{}{"database": "app"},
	}))
	assertStatus(t, rr, http.StatusBadRequest)
}

func TestUpdateService_ConnectionFieldsKeepStoredPassword(t *testing.T) {
	env := newTestEnv(t)
	rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, map[string]interface{}{
		"name": "pg", "driver": "postgres",
		"connection": map[string]interface{}{"host": "old", "database": "app", "username": "api", "password": "secret"},
	}))
	assertStatus(t, rr, http.StatusCreated)

	rr = env.do(t, "PUT", "/api/v1/system/service/pg", toJSON(t, map[string]interface{}{
		"is_active":  false,
		"connection": map[string]interface{}{"host": "old", "database": "app2", "username": "api"},
	}))
	assertStatus(t, rr, http.StatusOK)

	svc, err := env.store.GetServiceByName(t.Context(), "pg")
	if err != nil {
		t.Fatal(err)
	}
	if svc.DSN != "postgres://api:secret@old:5432/app2" {
		t.Errorf("stored DSN = %q", svc.DSN)
	}

	// Moving to another host requires the password again.
	rr = env.do(t, "PUT", "/api/v1/system/service/pg", toJSON(t, map[string]interface{}{
		"is_active":  false,
		"connection": map[string]interface{}{"host": "elsewhere", "database": "app2", "username": "api"},
	}))
	assertStatus(t, rr, http.StatusOK)
	svc, _ = env.store.GetServiceByName(t.Context(), "pg")
	if strings.Contains(svc.DSN, "secret") {
		t.Errorf("stored password carried to a new host: %q", svc.DSN)
	}
}

func TestProbeConnection_SQLite(t *testing.T) {
	env := newTestEnv(t)
	path := newSQLiteFile(t)

	rr := env.do(t, "POST", "/api/v1/system/connection/test", toJSON(t, map[string]interface{}{
		"driver": "sqlite", "connection": map[string]interface{}{"path": path},
	}))
	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Success    bool     `json:"success"`
		TableCount int      `json:"table_count"`
		Tables     []string `json:"tables"`
	}
	decodeJSON(t, rr, &resp)
	if !resp.Success || resp.TableCount != 2 {
		t.Errorf("got %+v", resp)
	}

	// Nothing should have been saved or registered.
	services, _ := env.store.ListServices(t.Context())
	if len(services) != 0 {
		t.Errorf("probe persisted %d services", len(services))
	}
}

func TestProbeConnection_Failure(t *testing.T) {
	env := newTestEnv(t)

	rr := env.do(t, "POST", "/api/v1/system/connection/test", toJSON(t, map[string]interface{}{
		"driver": "sqlite", "dsn": "file:" + filepath.Join(t.TempDir(), "missing", "x.db") + "?mode=ro",
	}))
	assertStatus(t, rr, http.StatusUnprocessableEntity)

	rr = env.do(t, "POST", "/api/v1/system/connection/test", toJSON(t, map[string]interface{}{"driver": "sqlite"}))
	assertStatus(t, rr, http.StatusBadRequest)

	rr = env.do(t, "POST", "/api/v1/system/connection/test", toJSON(t, map[string]interface{}{
		"driver": "db2", "dsn": "x",
	}))
	assertStatus(t, rr, http.StatusUnprocessableEntity)
}

func TestUpdateService_ClearsSchemaAndKeyWhenSentEmpty(t *testing.T) {
	env := newTestEnv(t)
	rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, map[string]interface{}{
		"name": "sf", "driver": "snowflake", "dsn": "SVC@acct/DB/S?warehouse=WH",
		"schema": "S", "private_key_path": "/keys/k.p8",
	}))
	assertStatus(t, rr, http.StatusCreated)

	// Omitted fields are kept.
	rr = env.do(t, "PUT", "/api/v1/system/service/sf", toJSON(t, map[string]interface{}{"is_active": false}))
	assertStatus(t, rr, http.StatusOK)
	svc, _ := env.store.GetServiceByName(t.Context(), "sf")
	if svc.Schema != "S" || svc.PrivateKeyPath != "/keys/k.p8" {
		t.Fatalf("omitted fields changed: %+v", svc)
	}

	// Explicitly empty fields are cleared.
	rr = env.do(t, "PUT", "/api/v1/system/service/sf", toJSON(t, map[string]interface{}{
		"is_active": false, "schema": "", "private_key_path": "",
	}))
	assertStatus(t, rr, http.StatusOK)
	svc, _ = env.store.GetServiceByName(t.Context(), "sf")
	if svc.Schema != "" || svc.PrivateKeyPath != "" {
		t.Errorf("fields not cleared: schema=%q key=%q", svc.Schema, svc.PrivateKeyPath)
	}
}

func TestProbeConnection_SQLiteMissingFileIsNotCreated(t *testing.T) {
	env := newTestEnv(t)
	missing := filepath.Join(t.TempDir(), "nope.db")
	rr := env.do(t, "POST", "/api/v1/system/connection/test", toJSON(t, map[string]interface{}{
		"driver": "sqlite", "connection": map[string]interface{}{"path": missing},
	}))
	assertStatus(t, rr, http.StatusUnprocessableEntity)
	if !strings.Contains(rr.Body.String(), "no such file") {
		t.Errorf("body = %s", rr.Body.String())
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("connection test created the database file")
	}
}

func TestProbeConnection_EditWithBlankDSNUsesStoredOne(t *testing.T) {
	env := newTestEnv(t)
	path := newSQLiteFile(t)
	rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, map[string]interface{}{
		"name": "shop", "driver": "sqlite", "dsn": path, "is_active": true,
	}))
	assertStatus(t, rr, http.StatusCreated)
	rr = env.do(t, "POST", "/api/v1/system/connection/test", toJSON(t, map[string]interface{}{
		"name": "shop", "driver": "sqlite",
	}))
	assertStatus(t, rr, http.StatusOK)
}

func TestTestConnection_PausedServiceStaysPaused(t *testing.T) {
	env := newTestEnv(t)
	path := newSQLiteFile(t)
	rr := env.do(t, "POST", "/api/v1/system/service", toJSON(t, map[string]interface{}{
		"name": "shop", "driver": "sqlite", "dsn": path,
	}))
	assertStatus(t, rr, http.StatusCreated)
	rr = env.do(t, "PUT", "/api/v1/system/service/shop", toJSON(t, map[string]interface{}{"is_active": false}))
	assertStatus(t, rr, http.StatusOK)

	rr = env.do(t, "GET", "/api/v1/system/service/shop/test", nil)
	assertStatus(t, rr, http.StatusOK)
	if _, err := env.handler.registry.Get("shop"); err == nil {
		t.Error("testing a paused service reconnected its API")
	}
}

func TestInfo(t *testing.T) {
	env := newTestEnv(t)
	env.handler.Version = "v9.9.9"
	rr := env.do(t, "GET", "/api/v1/system/info", nil)
	assertStatus(t, rr, http.StatusOK)
	var resp struct {
		Version string   `json:"version"`
		Drivers []string `json:"drivers"`
	}
	decodeJSON(t, rr, &resp)
	if resp.Version != "v9.9.9" || len(resp.Drivers) != 1 || resp.Drivers[0] != "sqlite" {
		t.Errorf("got %+v", resp)
	}
}
