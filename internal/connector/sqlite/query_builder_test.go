package sqlite

import (
	"context"
	"reflect"
	"testing"

	"github.com/faucetdb/faucet/internal/connector"
)

// newTestConnector creates a SQLiteConnector with a known schema name
// and no database connection, suitable for testing query building methods.
func newTestConnector() *SQLiteConnector {
	return &SQLiteConnector{schemaName: "main"}
}

// ---------------------------------------------------------------------------
// BuildSelect tests
// ---------------------------------------------------------------------------

func TestBuildSelect(t *testing.T) {
	tests := []struct {
		name     string
		req      connector.SelectRequest
		wantSQL  string
		wantArgs []interface{}
		wantErr  bool
	}{
		{
			name:    "empty table returns error",
			req:     connector.SelectRequest{},
			wantErr: true,
		},
		{
			name: "simple select all",
			req: connector.SelectRequest{
				Table: "users",
			},
			wantSQL:  `SELECT * FROM "users"`,
			wantArgs: nil,
		},
		{
			name: "select with field selection",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"id", "name", "email"},
			},
			wantSQL:  `SELECT "id", "name", "email" FROM "users"`,
			wantArgs: nil,
		},
		{
			name: "select with single field",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"name"},
			},
			wantSQL:  `SELECT "name" FROM "users"`,
			wantArgs: nil,
		},
		{
			name: "select with filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > ?",
			},
			wantSQL:  `SELECT * FROM "users" WHERE age > ?`,
			wantArgs: nil,
		},
		{
			name: "select with complex filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > ? AND status = ?",
			},
			wantSQL:  `SELECT * FROM "users" WHERE age > ? AND status = ?`,
			wantArgs: nil,
		},
		{
			name: "select with ordering",
			req: connector.SelectRequest{
				Table: "users",
				Order: `"created_at" DESC`,
			},
			wantSQL:  `SELECT * FROM "users" ORDER BY "created_at" DESC`,
			wantArgs: nil,
		},
		{
			name: "select with limit only",
			req: connector.SelectRequest{
				Table: "users",
				Limit: 10,
			},
			wantSQL:  `SELECT * FROM "users" LIMIT ?`,
			wantArgs: []interface{}{10},
		},
		{
			name: "select with limit and offset",
			req: connector.SelectRequest{
				Table:  "users",
				Limit:  10,
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "users" LIMIT ? OFFSET ?`,
			wantArgs: []interface{}{10, 20},
		},
		{
			name: "select with offset but no limit",
			req: connector.SelectRequest{
				Table:  "users",
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "users" OFFSET ?`,
			wantArgs: []interface{}{20},
		},
		{
			name: "select with all options",
			req: connector.SelectRequest{
				Table:  "orders",
				Fields: []string{"id", "total"},
				Filter: "status = ?",
				Order:  `"total" DESC`,
				Limit:  25,
				Offset: 50,
			},
			wantSQL:  `SELECT "id", "total" FROM "orders" WHERE status = ? ORDER BY "total" DESC LIMIT ? OFFSET ?`,
			wantArgs: []interface{}{25, 50},
		},
		{
			name: "field names with special characters are quoted",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"first name", "last name"},
			},
			wantSQL:  `SELECT "first name", "last name" FROM "users"`,
			wantArgs: nil,
		},
		{
			name: "no schema qualification for SQLite",
			req: connector.SelectRequest{
				Table: "products",
			},
			wantSQL:  `SELECT * FROM "products"`,
			wantArgs: nil,
		},
	}

	c := newTestConnector()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := c.BuildSelect(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, tt.wantArgs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildInsert tests
// ---------------------------------------------------------------------------

func TestBuildInsert(t *testing.T) {
	tests := []struct {
		name     string
		req      connector.InsertRequest
		wantSQL  string
		wantArgs []interface{}
		wantErr  bool
	}{
		{
			name:    "empty table returns error",
			req:     connector.InsertRequest{Table: ""},
			wantErr: true,
		},
		{
			name:    "no records returns error",
			req:     connector.InsertRequest{Table: "users", Records: nil},
			wantErr: true,
		},
		{
			name: "single record",
			req: connector.InsertRequest{
				Table: "users",
				Records: []map[string]interface{}{
					{"email": "alice@example.com", "name": "Alice"},
				},
			},
			// Columns are sorted: email, name
			wantSQL:  `INSERT INTO "users" ("email", "name") VALUES (?, ?) RETURNING *`,
			wantArgs: []interface{}{"alice@example.com", "Alice"},
		},
		{
			name: "multiple records",
			req: connector.InsertRequest{
				Table: "users",
				Records: []map[string]interface{}{
					{"email": "alice@example.com", "name": "Alice"},
					{"email": "bob@example.com", "name": "Bob"},
				},
			},
			wantSQL:  `INSERT INTO "users" ("email", "name") VALUES (?, ?), (?, ?) RETURNING *`,
			wantArgs: []interface{}{"alice@example.com", "Alice", "bob@example.com", "Bob"},
		},
		{
			name: "single column record",
			req: connector.InsertRequest{
				Table: "tags",
				Records: []map[string]interface{}{
					{"name": "golang"},
				},
			},
			wantSQL:  `INSERT INTO "tags" ("name") VALUES (?) RETURNING *`,
			wantArgs: []interface{}{"golang"},
		},
		{
			name: "numeric values",
			req: connector.InsertRequest{
				Table: "products",
				Records: []map[string]interface{}{
					{"name": "Widget", "price": 9.99},
				},
			},
			// Columns sorted: name, price
			wantSQL:  `INSERT INTO "products" ("name", "price") VALUES (?, ?) RETURNING *`,
			wantArgs: []interface{}{"Widget", 9.99},
		},
		{
			name: "three records",
			req: connector.InsertRequest{
				Table: "items",
				Records: []map[string]interface{}{
					{"val": 1},
					{"val": 2},
					{"val": 3},
				},
			},
			wantSQL:  `INSERT INTO "items" ("val") VALUES (?), (?), (?) RETURNING *`,
			wantArgs: []interface{}{1, 2, 3},
		},
	}

	c := newTestConnector()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := c.BuildInsert(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, tt.wantArgs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildUpdate tests
// ---------------------------------------------------------------------------

func TestBuildUpdate(t *testing.T) {
	tests := []struct {
		name     string
		req      connector.UpdateRequest
		wantSQL  string
		wantArgs []interface{}
		wantErr  bool
	}{
		{
			name:    "empty table returns error",
			req:     connector.UpdateRequest{Table: ""},
			wantErr: true,
		},
		{
			name: "no fields returns error",
			req: connector.UpdateRequest{
				Table:  "users",
				Filter: "id = ?",
				Record: nil,
			},
			wantErr: true,
		},
		{
			name: "no filter and no IDs returns error",
			req: connector.UpdateRequest{
				Table:  "users",
				Record: map[string]interface{}{"name": "Alice"},
			},
			wantErr: true,
		},
		{
			name: "update with filter",
			req: connector.UpdateRequest{
				Table:  "users",
				Filter: "status = 'inactive'",
				Record: map[string]interface{}{"name": "Updated"},
			},
			wantSQL:  `UPDATE "users" SET "name" = ? WHERE status = 'inactive' RETURNING *`,
			wantArgs: []interface{}{"Updated"},
		},
		{
			name: "update with multiple fields and filter",
			req: connector.UpdateRequest{
				Table:  "users",
				Filter: "id = 1",
				Record: map[string]interface{}{"email": "new@example.com", "name": "New Name"},
			},
			// Columns sorted: email, name
			wantSQL:  `UPDATE "users" SET "email" = ?, "name" = ? WHERE id = 1 RETURNING *`,
			wantArgs: []interface{}{"new@example.com", "New Name"},
		},
		{
			name: "update with IDs",
			req: connector.UpdateRequest{
				Table:  "users",
				Record: map[string]interface{}{"active": true},
				IDs:    []interface{}{1, 2, 3},
			},
			wantSQL:  `UPDATE "users" SET "active" = ? WHERE "id" IN (?, ?, ?) RETURNING *`,
			wantArgs: []interface{}{true, 1, 2, 3},
		},
		{
			name: "update with filter and IDs",
			req: connector.UpdateRequest{
				Table:  "users",
				Filter: "status = 'active'",
				Record: map[string]interface{}{"name": "Test"},
				IDs:    []interface{}{5},
			},
			wantSQL:  `UPDATE "users" SET "name" = ? WHERE status = 'active' AND "id" IN (?) RETURNING *`,
			wantArgs: []interface{}{"Test", 5},
		},
	}

	c := newTestConnector()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := c.BuildUpdate(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, tt.wantArgs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildDelete tests
// ---------------------------------------------------------------------------

func TestBuildDelete(t *testing.T) {
	tests := []struct {
		name     string
		req      connector.DeleteRequest
		wantSQL  string
		wantArgs []interface{}
		wantErr  bool
	}{
		{
			name:    "empty table returns error",
			req:     connector.DeleteRequest{Table: ""},
			wantErr: true,
		},
		{
			name:    "no filter and no IDs returns error",
			req:     connector.DeleteRequest{Table: "users"},
			wantErr: true,
		},
		{
			name: "delete with filter",
			req: connector.DeleteRequest{
				Table:  "users",
				Filter: "status = 'deleted'",
			},
			wantSQL:  `DELETE FROM "users" WHERE status = 'deleted'`,
			wantArgs: nil,
		},
		{
			name: "delete with IDs",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{1, 2, 3},
			},
			wantSQL:  `DELETE FROM "users" WHERE "id" IN (?, ?, ?)`,
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name: "delete with single ID",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{42},
			},
			wantSQL:  `DELETE FROM "users" WHERE "id" IN (?)`,
			wantArgs: []interface{}{42},
		},
		{
			name: "delete with filter and IDs",
			req: connector.DeleteRequest{
				Table:  "users",
				Filter: "active = false",
				IDs:    []interface{}{10, 20},
			},
			wantSQL:  `DELETE FROM "users" WHERE active = false AND "id" IN (?, ?)`,
			wantArgs: []interface{}{10, 20},
		},
	}

	c := newTestConnector()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := c.BuildDelete(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, tt.wantArgs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildCount tests
// ---------------------------------------------------------------------------

func TestBuildCount(t *testing.T) {
	tests := []struct {
		name     string
		req      connector.CountRequest
		wantSQL  string
		wantArgs []interface{}
		wantErr  bool
	}{
		{
			name:    "empty table returns error",
			req:     connector.CountRequest{Table: ""},
			wantErr: true,
		},
		{
			name: "count all rows",
			req: connector.CountRequest{
				Table: "users",
			},
			wantSQL:  `SELECT COUNT(*) FROM "users"`,
			wantArgs: nil,
		},
		{
			name: "count with filter",
			req: connector.CountRequest{
				Table:  "users",
				Filter: "active = true",
			},
			wantSQL:  `SELECT COUNT(*) FROM "users" WHERE active = true`,
			wantArgs: nil,
		},
		{
			name: "count with complex filter",
			req: connector.CountRequest{
				Table:  "orders",
				Filter: "status = 'pending' AND total > 100",
			},
			wantSQL:  `SELECT COUNT(*) FROM "orders" WHERE status = 'pending' AND total > 100`,
			wantArgs: nil,
		},
	}

	c := newTestConnector()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := c.BuildCount(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, tt.wantArgs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Dialect-specific behavior tests
// ---------------------------------------------------------------------------

func TestSQLiteDialect(t *testing.T) {
	c := newTestConnector()

	t.Run("QuoteIdentifier uses double quotes", func(t *testing.T) {
		got := c.QuoteIdentifier("users")
		want := `"users"`
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})

	t.Run("QuoteIdentifier escapes embedded double quotes", func(t *testing.T) {
		got := c.QuoteIdentifier(`my"table`)
		want := `"my""table"`
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})

	t.Run("ParameterPlaceholder always returns question mark", func(t *testing.T) {
		for _, idx := range []int{1, 2, 3, 100} {
			got := c.ParameterPlaceholder(idx)
			if got != "?" {
				t.Errorf("ParameterPlaceholder(%d) = %s, want ?", idx, got)
			}
		}
	})

	t.Run("SupportsReturning is true", func(t *testing.T) {
		if !c.SupportsReturning() {
			t.Error("expected SupportsReturning() == true")
		}
	})

	t.Run("SupportsUpsert is true", func(t *testing.T) {
		if !c.SupportsUpsert() {
			t.Error("expected SupportsUpsert() == true")
		}
	})

	t.Run("DriverName is sqlite", func(t *testing.T) {
		if c.DriverName() != "sqlite" {
			t.Errorf("expected DriverName() == sqlite, got %s", c.DriverName())
		}
	})
}

// TestBuildInsertHasReturning verifies that SQLite INSERT includes
// RETURNING (since SQLite 3.35+).
func TestBuildInsertHasReturning(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, _, err := c.BuildInsert(ctx, connector.InsertRequest{
		Table: "users",
		Records: []map[string]interface{}{
			{"name": "test"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(sql, "RETURNING") {
		t.Errorf("SQLite INSERT should contain RETURNING, got: %s", sql)
	}
}

// TestBuildUpdateHasReturning verifies that SQLite UPDATE includes
// RETURNING (since SQLite 3.35+).
func TestBuildUpdateHasReturning(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, _, err := c.BuildUpdate(ctx, connector.UpdateRequest{
		Table:  "users",
		Filter: "id = 1",
		Record: map[string]interface{}{"name": "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(sql, "RETURNING") {
		t.Errorf("SQLite UPDATE should contain RETURNING, got: %s", sql)
	}
}

// TestBuildSelectNoSchemaQualification verifies that SQLite does NOT use
// schema-qualified table names (unlike PostgreSQL, MySQL, etc.).
func TestBuildSelectNoSchemaQualification(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{Table: "users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT * FROM "users"`
	if sql != want {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, want)
	}
	// Ensure no "main"."users" qualification
	if contains(sql, `"main"`) {
		t.Errorf("SQLite should not schema-qualify table names, got: %s", sql)
	}
}

// TestBuildInsertColumnOrdering verifies that columns are sorted
// alphabetically for deterministic output.
func TestBuildInsertColumnOrdering(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, args, err := c.BuildInsert(ctx, connector.InsertRequest{
		Table: "users",
		Records: []map[string]interface{}{
			{"zebra": "z", "apple": "a", "mango": "m"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantSQL := `INSERT INTO "users" ("apple", "mango", "zebra") VALUES (?, ?, ?) RETURNING *`
	if sql != wantSQL {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantArgs := []interface{}{"a", "m", "z"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, wantArgs)
	}
}

// TestBuildUpdateColumnOrdering verifies that SET columns are sorted
// alphabetically for deterministic output.
func TestBuildUpdateColumnOrdering(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, args, err := c.BuildUpdate(ctx, connector.UpdateRequest{
		Table:  "users",
		Filter: "id = 1",
		Record: map[string]interface{}{"zebra": "z", "apple": "a"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantSQL := `UPDATE "users" SET "apple" = ?, "zebra" = ? WHERE id = 1 RETURNING *`
	if sql != wantSQL {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantArgs := []interface{}{"a", "z"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, wantArgs)
	}
}

// TestSQLiteUsesQuestionMarkPlaceholders verifies that SQLite uses ? for all
// parameter placeholders in generated queries.
func TestSQLiteUsesQuestionMarkPlaceholders(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{
		Table:  "users",
		Limit:  10,
		Offset: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(sql, "$") {
		t.Errorf("SQLite should not use $ placeholders, got: %s", sql)
	}
	if !contains(sql, "?") {
		t.Errorf("SQLite should use ? placeholders, got: %s", sql)
	}
}

// TestSQLiteDefaultSchemaIsMain verifies the default schema name.
func TestSQLiteDefaultSchemaIsMain(t *testing.T) {
	c := New().(*SQLiteConnector)
	if c.schemaName != "main" {
		t.Errorf("expected default schema 'main', got %q", c.schemaName)
	}
}

// TestMapSQLiteType verifies type mapping from SQLite types to Go/JSON types.
func TestMapSQLiteType(t *testing.T) {
	tests := []struct {
		input    string
		wantGo   string
		wantJSON string
	}{
		{"INTEGER", "int64", "integer"},
		{"INT", "int64", "integer"},
		{"BIGINT", "int64", "integer"},
		{"TINYINT", "int64", "integer"},
		{"TEXT", "string", "string"},
		{"VARCHAR(255)", "string", "string"},
		{"CHARACTER(20)", "string", "string"},
		{"CLOB", "string", "string"},
		{"REAL", "float64", "number"},
		{"DOUBLE", "float64", "number"},
		{"FLOAT", "float64", "number"},
		{"BLOB", "[]byte", "string(byte)"},
		{"", "[]byte", "string(byte)"},
		{"BOOLEAN", "bool", "boolean"},
		{"DATETIME", "time.Time", "string(date-time)"},
		{"TIMESTAMP", "time.Time", "string(date-time)"},
		{"DATE", "time.Time", "string(date-time)"},
		{"NUMERIC", "float64", "number"},
		{"DECIMAL(10,2)", "float64", "number"},
		{"JSON", "interface{}", "object"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			goType, jsonType := mapSQLiteType(tt.input)
			if goType != tt.wantGo {
				t.Errorf("mapSQLiteType(%q) goType = %q, want %q", tt.input, goType, tt.wantGo)
			}
			if jsonType != tt.wantJSON {
				t.Errorf("mapSQLiteType(%q) jsonType = %q, want %q", tt.input, jsonType, tt.wantJSON)
			}
		})
	}
}

// contains checks if substr is present in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
