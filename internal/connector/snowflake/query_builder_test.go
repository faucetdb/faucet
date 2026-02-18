package snowflake

import (
	"context"
	"reflect"
	"testing"

	"github.com/faucetdb/faucet/internal/connector"
)

// newTestConnector creates a SnowflakeConnector with a known schema name
// and no database connection, suitable for testing query building methods.
func newTestConnector() *SnowflakeConnector {
	return &SnowflakeConnector{schemaName: "PUBLIC"}
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
			wantSQL:  `SELECT * FROM "PUBLIC"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with field selection",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"id", "name", "email"},
			},
			wantSQL:  `SELECT "id", "name", "email" FROM "PUBLIC"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with single field",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"name"},
			},
			wantSQL:  `SELECT "name" FROM "PUBLIC"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > ?",
			},
			wantSQL:  `SELECT * FROM "PUBLIC"."users" WHERE age > ?`,
			wantArgs: nil,
		},
		{
			name: "select with complex filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > ? AND status = ?",
			},
			wantSQL:  `SELECT * FROM "PUBLIC"."users" WHERE age > ? AND status = ?`,
			wantArgs: nil,
		},
		{
			name: "select with ordering",
			req: connector.SelectRequest{
				Table: "users",
				Order: `"created_at" DESC`,
			},
			wantSQL:  `SELECT * FROM "PUBLIC"."users" ORDER BY "created_at" DESC`,
			wantArgs: nil,
		},
		{
			name: "select with limit only",
			req: connector.SelectRequest{
				Table: "users",
				Limit: 10,
			},
			wantSQL:  `SELECT * FROM "PUBLIC"."users" LIMIT ?`,
			wantArgs: []interface{}{10},
		},
		{
			name: "select with limit and offset",
			req: connector.SelectRequest{
				Table:  "users",
				Limit:  10,
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "PUBLIC"."users" LIMIT ? OFFSET ?`,
			wantArgs: []interface{}{10, 20},
		},
		{
			name: "select with offset but no limit",
			req: connector.SelectRequest{
				Table:  "users",
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "PUBLIC"."users" OFFSET ?`,
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
			wantSQL:  `SELECT "id", "total" FROM "PUBLIC"."orders" WHERE status = ? ORDER BY "total" DESC LIMIT ? OFFSET ?`,
			wantArgs: []interface{}{25, 50},
		},
		{
			name: "field names with special characters are quoted",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"first name", "last name"},
			},
			wantSQL:  `SELECT "first name", "last name" FROM "PUBLIC"."users"`,
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
			// Columns sorted: email, name
			wantSQL:  `INSERT INTO "PUBLIC"."users" ("email", "name") VALUES (?, ?)`,
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
			wantSQL:  `INSERT INTO "PUBLIC"."users" ("email", "name") VALUES (?, ?), (?, ?)`,
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
			wantSQL:  `INSERT INTO "PUBLIC"."tags" ("name") VALUES (?)`,
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
			wantSQL:  `INSERT INTO "PUBLIC"."products" ("name", "price") VALUES (?, ?)`,
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
			wantSQL:  `INSERT INTO "PUBLIC"."items" ("val") VALUES (?), (?), (?)`,
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
			wantSQL:  `UPDATE "PUBLIC"."users" SET "name" = ? WHERE status = 'inactive'`,
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
			wantSQL:  `UPDATE "PUBLIC"."users" SET "email" = ?, "name" = ? WHERE id = 1`,
			wantArgs: []interface{}{"new@example.com", "New Name"},
		},
		{
			name: "update with IDs",
			req: connector.UpdateRequest{
				Table:  "users",
				Record: map[string]interface{}{"active": true},
				IDs:    []interface{}{1, 2, 3},
			},
			wantSQL:  `UPDATE "PUBLIC"."users" SET "active" = ? WHERE "id" IN (?, ?, ?)`,
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
			wantSQL:  `UPDATE "PUBLIC"."users" SET "name" = ? WHERE status = 'active' AND "id" IN (?)`,
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
			wantSQL:  `DELETE FROM "PUBLIC"."users" WHERE status = 'deleted'`,
			wantArgs: nil,
		},
		{
			name: "delete with IDs",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{1, 2, 3},
			},
			wantSQL:  `DELETE FROM "PUBLIC"."users" WHERE "id" IN (?, ?, ?)`,
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name: "delete with single ID",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{42},
			},
			wantSQL:  `DELETE FROM "PUBLIC"."users" WHERE "id" IN (?)`,
			wantArgs: []interface{}{42},
		},
		{
			name: "delete with filter and IDs",
			req: connector.DeleteRequest{
				Table:  "users",
				Filter: "active = false",
				IDs:    []interface{}{10, 20},
			},
			wantSQL:  `DELETE FROM "PUBLIC"."users" WHERE active = false AND "id" IN (?, ?)`,
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
			wantSQL:  `SELECT COUNT(*) FROM "PUBLIC"."users"`,
			wantArgs: nil,
		},
		{
			name: "count with filter",
			req: connector.CountRequest{
				Table:  "users",
				Filter: "active = true",
			},
			wantSQL:  `SELECT COUNT(*) FROM "PUBLIC"."users" WHERE active = true`,
			wantArgs: nil,
		},
		{
			name: "count with complex filter",
			req: connector.CountRequest{
				Table:  "orders",
				Filter: "status = 'pending' AND total > 100",
			},
			wantSQL:  `SELECT COUNT(*) FROM "PUBLIC"."orders" WHERE status = 'pending' AND total > 100`,
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

func TestSnowflakeDialect(t *testing.T) {
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

	t.Run("SupportsReturning is false", func(t *testing.T) {
		if c.SupportsReturning() {
			t.Error("expected SupportsReturning() == false")
		}
	})

	t.Run("SupportsUpsert is false", func(t *testing.T) {
		if c.SupportsUpsert() {
			t.Error("expected SupportsUpsert() == false")
		}
	})

	t.Run("DriverName is snowflake", func(t *testing.T) {
		if c.DriverName() != "snowflake" {
			t.Errorf("expected DriverName() == snowflake, got %s", c.DriverName())
		}
	})
}

// TestBuildInsertNoReturning verifies that Snowflake INSERT does NOT include
// RETURNING or OUTPUT.
func TestBuildInsertNoReturning(t *testing.T) {
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
	if contains(sql, "RETURNING") {
		t.Errorf("Snowflake INSERT should not contain RETURNING, got: %s", sql)
	}
	if contains(sql, "OUTPUT") {
		t.Errorf("Snowflake INSERT should not contain OUTPUT, got: %s", sql)
	}
}

// TestBuildUpdateNoReturning verifies that Snowflake UPDATE does NOT include
// RETURNING or OUTPUT.
func TestBuildUpdateNoReturning(t *testing.T) {
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
	if contains(sql, "RETURNING") {
		t.Errorf("Snowflake UPDATE should not contain RETURNING, got: %s", sql)
	}
	if contains(sql, "OUTPUT") {
		t.Errorf("Snowflake UPDATE should not contain OUTPUT, got: %s", sql)
	}
}

// TestBuildSelectSchemaQualified verifies that table references are
// schema-qualified with the configured schema name (defaults to PUBLIC).
func TestBuildSelectSchemaQualified(t *testing.T) {
	c := &SnowflakeConnector{schemaName: "MY_SCHEMA"}
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{Table: "users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT * FROM "MY_SCHEMA"."users"`
	if sql != want {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, want)
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
	wantSQL := `INSERT INTO "PUBLIC"."users" ("apple", "mango", "zebra") VALUES (?, ?, ?)`
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
	wantSQL := `UPDATE "PUBLIC"."users" SET "apple" = ?, "zebra" = ? WHERE id = 1`
	if sql != wantSQL {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantArgs := []interface{}{"a", "z"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, wantArgs)
	}
}

// TestSnowflakeUsesQuestionMarkPlaceholders verifies that Snowflake uses ? for
// all parameter placeholders in generated queries, not numbered ones.
func TestSnowflakeUsesQuestionMarkPlaceholders(t *testing.T) {
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
		t.Errorf("Snowflake should not use $ placeholders, got: %s", sql)
	}
	if contains(sql, "@p") {
		t.Errorf("Snowflake should not use @p placeholders, got: %s", sql)
	}
	if !contains(sql, "?") {
		t.Errorf("Snowflake should use ? placeholders, got: %s", sql)
	}
}

// TestSnowflakeDefaultSchemaIsPUBLIC verifies that the default schema
// name is "PUBLIC" (uppercase), following Snowflake conventions.
func TestSnowflakeDefaultSchemaIsPUBLIC(t *testing.T) {
	c := newTestConnector()
	if c.schemaName != "PUBLIC" {
		t.Errorf("expected default schema PUBLIC, got %s", c.schemaName)
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
