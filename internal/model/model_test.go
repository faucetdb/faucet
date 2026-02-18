package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDefaultPoolConfig(t *testing.T) {
	pc := DefaultPoolConfig()

	if pc.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns = %d, want 25", pc.MaxOpenConns)
	}
	if pc.MaxIdleConns != 5 {
		t.Errorf("MaxIdleConns = %d, want 5", pc.MaxIdleConns)
	}
	if pc.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("ConnMaxLifetime = %v, want %v", pc.ConnMaxLifetime, 5*time.Minute)
	}
	if pc.ConnMaxIdleTime != 1*time.Minute {
		t.Errorf("ConnMaxIdleTime = %v, want %v", pc.ConnMaxIdleTime, 1*time.Minute)
	}
	if pc.PingInterval != 30*time.Second {
		t.Errorf("PingInterval = %v, want %v", pc.PingInterval, 30*time.Second)
	}
}

func TestServiceConfigJSON(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	sc := ServiceConfig{
		ID:       1,
		Name:     "testdb",
		Label:    "Test Database",
		Driver:   "postgres",
		DSN:      "postgres://user:pass@localhost/db",
		Schema:   "public",
		ReadOnly: false,
		RawSQL:   true,
		IsActive: true,
		Pool:     DefaultPoolConfig(),
		CreatedAt: now,
		UpdatedAt: now,
	}

	b, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// DSN should be present when set (json:"dsn,omitempty")
	if _, ok := m["dsn"]; !ok {
		t.Error("expected 'dsn' key in JSON output when DSN is set")
	}
	if m["dsn"] != "postgres://user:pass@localhost/db" {
		t.Errorf("dsn = %v, want %q", m["dsn"], "postgres://user:pass@localhost/db")
	}

	// Verify DSN is omitted when empty
	sc.DSN = ""
	b2, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var m2 map[string]interface{}
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := m2["dsn"]; ok {
		t.Error("expected 'dsn' key to be omitted when DSN is empty")
	}

	// Round-trip: unmarshal back into ServiceConfig
	var sc2 ServiceConfig
	if err := json.Unmarshal(b, &sc2); err != nil {
		t.Fatalf("Unmarshal into ServiceConfig error: %v", err)
	}
	if sc2.Name != "testdb" {
		t.Errorf("Name = %q, want %q", sc2.Name, "testdb")
	}
	if sc2.Driver != "postgres" {
		t.Errorf("Driver = %q, want %q", sc2.Driver, "postgres")
	}
	if sc2.RawSQL != true {
		t.Error("RawSQL should be true after round-trip")
	}
}

func TestAdminPasswordHashNotInJSON(t *testing.T) {
	admin := Admin{
		ID:           1,
		Email:        "admin@example.com",
		PasswordHash: "$2a$10$somebcrypthash",
		Name:         "Admin User",
		IsActive:     true,
		IsSuperAdmin: true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	b, err := json.Marshal(admin)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if _, ok := m["password_hash"]; ok {
		t.Error("password_hash should NOT appear in JSON output (json:\"-\" tag)")
	}

	// Verify other fields are present
	if _, ok := m["email"]; !ok {
		t.Error("email should be present in JSON output")
	}
	if _, ok := m["name"]; !ok {
		t.Error("name should be present in JSON output")
	}
}

func TestAPIKeyKeyHashNotInJSON(t *testing.T) {
	apiKey := APIKey{
		ID:        1,
		KeyHash:   "sha256hashvalue",
		KeyPrefix: "faucet_a",
		Label:     "My Key",
		RoleID:    1,
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	b, err := json.Marshal(apiKey)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if _, ok := m["key_hash"]; ok {
		t.Error("key_hash should NOT appear in JSON output (json:\"-\" tag)")
	}

	// Verify other fields are present
	if _, ok := m["key_prefix"]; !ok {
		t.Error("key_prefix should be present in JSON output")
	}
	if _, ok := m["label"]; !ok {
		t.Error("label should be present in JSON output")
	}
}

func TestVerbMaskConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"VerbGet", VerbGet, 1},
		{"VerbPost", VerbPost, 2},
		{"VerbPut", VerbPut, 4},
		{"VerbPatch", VerbPatch, 8},
		{"VerbDelete", VerbDelete, 16},
		{"VerbAll", VerbAll, 1 | 2 | 4 | 8 | 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}

	// VerbAll should equal 31
	if VerbAll != 31 {
		t.Errorf("VerbAll = %d, want 31", VerbAll)
	}
}

func TestRequestorMaskConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"RequestorAPI", RequestorAPI, 1},
		{"RequestorScript", RequestorScript, 2},
		{"RequestorAdmin", RequestorAdmin, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestListResponseJSON(t *testing.T) {
	total := int64(100)
	lr := ListResponse{
		Resource: []map[string]interface{}{
			{"id": float64(1), "name": "Alice"},
			{"id": float64(2), "name": "Bob"},
		},
		Meta: &ResponseMeta{
			Count:  2,
			Total:  &total,
			Limit:  10,
			Offset: 0,
			TookMs: 1.5,
		},
	}

	b, err := json.Marshal(lr)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify top-level keys
	if _, ok := m["resource"]; !ok {
		t.Error("expected 'resource' key in JSON output")
	}
	if _, ok := m["meta"]; !ok {
		t.Error("expected 'meta' key in JSON output")
	}

	// Verify resource array
	resource, ok := m["resource"].([]interface{})
	if !ok {
		t.Fatal("resource should be an array")
	}
	if len(resource) != 2 {
		t.Errorf("resource length = %d, want 2", len(resource))
	}

	// Verify meta fields
	meta, ok := m["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("meta should be an object")
	}
	if meta["count"] != float64(2) {
		t.Errorf("meta.count = %v, want 2", meta["count"])
	}
	if meta["total"] != float64(100) {
		t.Errorf("meta.total = %v, want 100", meta["total"])
	}

	// Verify meta is omitted when nil
	lr2 := ListResponse{
		Resource: []map[string]interface{}{},
	}
	b2, err := json.Marshal(lr2)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var m2 map[string]interface{}
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := m2["meta"]; ok {
		t.Error("meta should be omitted when nil")
	}
}

func TestErrorResponseJSON(t *testing.T) {
	er := ErrorResponse{
		Error: ErrorDetail{
			Code:    404,
			Message: "Resource not found",
			Context: map[string]interface{}{
				"table": "users",
			},
		},
	}

	b, err := json.Marshal(er)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' key to be an object")
	}
	if errObj["code"] != float64(404) {
		t.Errorf("error.code = %v, want 404", errObj["code"])
	}
	if errObj["message"] != "Resource not found" {
		t.Errorf("error.message = %v, want %q", errObj["message"], "Resource not found")
	}
	ctx, ok := errObj["context"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'context' key to be an object")
	}
	if ctx["table"] != "users" {
		t.Errorf("error.context.table = %v, want %q", ctx["table"], "users")
	}

	// Context should be omitted when nil
	er2 := ErrorResponse{
		Error: ErrorDetail{
			Code:    500,
			Message: "Internal error",
		},
	}
	b2, err := json.Marshal(er2)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var m2 map[string]interface{}
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	errObj2 := m2["error"].(map[string]interface{})
	if _, ok := errObj2["context"]; ok {
		t.Error("context should be omitted when nil")
	}
}

func TestSchemaStructure(t *testing.T) {
	rowCount := int64(42)
	schema := Schema{
		Tables: []TableSchema{
			{
				Name: "users",
				Type: "table",
				Columns: []Column{
					{Name: "id", Position: 1, Type: "integer", GoType: "int64", JsonType: "number", IsPrimaryKey: true, IsAutoIncrement: true},
					{Name: "name", Position: 2, Type: "varchar", GoType: "string", JsonType: "string"},
				},
				PrimaryKey: []string{"id"},
				ForeignKeys: []ForeignKey{},
				Indexes:     []Index{{Name: "idx_users_name", Columns: []string{"name"}, IsUnique: false}},
				RowCount:    &rowCount,
			},
		},
		Views: []TableSchema{
			{
				Name: "active_users",
				Type: "view",
				Columns: []Column{
					{Name: "id", Position: 1, Type: "integer", GoType: "int64", JsonType: "number"},
				},
				PrimaryKey:  []string{},
				ForeignKeys: []ForeignKey{},
				Indexes:     []Index{},
			},
		},
		Procedures: []StoredProcedure{
			{
				Name: "get_user",
				Type: "procedure",
				Parameters: []ProcedureParam{
					{Name: "user_id", Type: "integer", Direction: "in"},
				},
			},
		},
		Functions: []StoredProcedure{
			{
				Name:       "count_users",
				Type:       "function",
				ReturnType: "integer",
			},
		},
	}

	b, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify all top-level keys
	for _, key := range []string{"tables", "views", "procedures", "functions"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected %q key in Schema JSON", key)
		}
	}

	// Verify tables array
	tables, ok := m["tables"].([]interface{})
	if !ok {
		t.Fatal("tables should be an array")
	}
	if len(tables) != 1 {
		t.Fatalf("tables length = %d, want 1", len(tables))
	}
	table := tables[0].(map[string]interface{})
	if table["name"] != "users" {
		t.Errorf("table.name = %v, want %q", table["name"], "users")
	}
	if table["row_count"] != float64(42) {
		t.Errorf("table.row_count = %v, want 42", table["row_count"])
	}

	// Verify views
	views, ok := m["views"].([]interface{})
	if !ok {
		t.Fatal("views should be an array")
	}
	if len(views) != 1 {
		t.Fatalf("views length = %d, want 1", len(views))
	}

	// Verify procedures
	procs := m["procedures"].([]interface{})
	if len(procs) != 1 {
		t.Fatalf("procedures length = %d, want 1", len(procs))
	}

	// Verify functions
	funcs := m["functions"].([]interface{})
	if len(funcs) != 1 {
		t.Fatalf("functions length = %d, want 1", len(funcs))
	}
}

func TestColumnFields(t *testing.T) {
	defVal := "now()"
	maxLen := int64(255)
	col := Column{
		Name:            "email",
		Position:        3,
		Type:            "varchar",
		GoType:          "string",
		JsonType:        "string",
		Nullable:        true,
		Default:         &defVal,
		MaxLength:       &maxLen,
		IsPrimaryKey:    false,
		IsAutoIncrement: false,
		IsUnique:        true,
		Comment:         "User email address",
	}

	b, err := json.Marshal(col)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	checks := map[string]interface{}{
		"name":              "email",
		"position":          float64(3),
		"db_type":           "varchar",
		"go_type":           "string",
		"json_type":         "string",
		"nullable":          true,
		"default":           "now()",
		"max_length":        float64(255),
		"is_primary_key":    false,
		"is_auto_increment": false,
		"is_unique":         true,
		"comment":           "User email address",
	}

	for key, want := range checks {
		got, ok := m[key]
		if !ok {
			t.Errorf("expected %q key in Column JSON", key)
			continue
		}
		if got != want {
			t.Errorf("%s = %v (%T), want %v (%T)", key, got, got, want, want)
		}
	}

	// Verify omitempty fields are absent when nil
	col2 := Column{
		Name:     "id",
		Position: 1,
		Type:     "integer",
		GoType:   "int64",
		JsonType: "number",
	}
	b2, err := json.Marshal(col2)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var m2 map[string]interface{}
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := m2["default"]; ok {
		t.Error("default should be omitted when nil")
	}
	if _, ok := m2["max_length"]; ok {
		t.Error("max_length should be omitted when nil")
	}
}
