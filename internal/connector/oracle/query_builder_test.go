package oracle

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/faucetdb/faucet/internal/connector"
)

// newTestConnector creates an OracleConnector with a known schema name
// and no database connection, suitable for testing query building methods.
func newTestConnector() *OracleConnector {
	return &OracleConnector{schemaName: "TESTUSER"}
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
			wantSQL:  `SELECT * FROM "TESTUSER"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with field selection",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"id", "name", "email"},
			},
			wantSQL:  `SELECT "id", "name", "email" FROM "TESTUSER"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with single field",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"name"},
			},
			wantSQL:  `SELECT "name" FROM "TESTUSER"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > :1",
			},
			wantSQL:  `SELECT * FROM "TESTUSER"."users" WHERE age > :1`,
			wantArgs: nil,
		},
		{
			name: "select with ordering",
			req: connector.SelectRequest{
				Table: "users",
				Order: `"created_at" DESC`,
			},
			wantSQL:  `SELECT * FROM "TESTUSER"."users" ORDER BY "created_at" DESC`,
			wantArgs: nil,
		},
		{
			name: "select with limit only",
			req: connector.SelectRequest{
				Table: "users",
				Limit: 10,
			},
			wantSQL:  `SELECT * FROM "TESTUSER"."users" OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			wantArgs: []interface{}{0, 10},
		},
		{
			name: "select with limit and offset",
			req: connector.SelectRequest{
				Table:  "users",
				Limit:  10,
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "TESTUSER"."users" OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			wantArgs: []interface{}{20, 10},
		},
		{
			name: "select with offset but no limit",
			req: connector.SelectRequest{
				Table:  "users",
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "TESTUSER"."users" OFFSET :1 ROWS`,
			wantArgs: []interface{}{20},
		},
		{
			name: "select with all options",
			req: connector.SelectRequest{
				Table:  "orders",
				Fields: []string{"id", "total"},
				Filter: "status = :1",
				Order:  `"total" DESC`,
				Limit:  25,
				Offset: 50,
			},
			wantSQL:  `SELECT "id", "total" FROM "TESTUSER"."orders" WHERE status = :1 ORDER BY "total" DESC OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY`,
			wantArgs: []interface{}{50, 25},
		},
		{
			name: "field names with special characters are quoted",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"first name", "last name"},
			},
			wantSQL:  `SELECT "first name", "last name" FROM "TESTUSER"."users"`,
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
			wantSQL:  `INSERT INTO "TESTUSER"."users" ("email", "name") VALUES (:1, :2)`,
			wantArgs: []interface{}{"alice@example.com", "Alice"},
		},
		{
			name: "multiple records uses INSERT ALL",
			req: connector.InsertRequest{
				Table: "users",
				Records: []map[string]interface{}{
					{"email": "alice@example.com", "name": "Alice"},
					{"email": "bob@example.com", "name": "Bob"},
				},
			},
			wantSQL:  `INSERT ALL INTO "TESTUSER"."users" ("email", "name") VALUES (:1, :2) INTO "TESTUSER"."users" ("email", "name") VALUES (:3, :4) SELECT 1 FROM DUAL`,
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
			wantSQL:  `INSERT INTO "TESTUSER"."tags" ("name") VALUES (:1)`,
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
			wantSQL:  `INSERT INTO "TESTUSER"."products" ("name", "price") VALUES (:1, :2)`,
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
			wantSQL:  `INSERT ALL INTO "TESTUSER"."items" ("val") VALUES (:1) INTO "TESTUSER"."items" ("val") VALUES (:2) INTO "TESTUSER"."items" ("val") VALUES (:3) SELECT 1 FROM DUAL`,
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
				Filter: "id = :1",
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
			wantSQL:  `UPDATE "TESTUSER"."users" SET "name" = :1 WHERE status = 'inactive'`,
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
			wantSQL:  `UPDATE "TESTUSER"."users" SET "email" = :1, "name" = :2 WHERE id = 1`,
			wantArgs: []interface{}{"new@example.com", "New Name"},
		},
		{
			name: "update with IDs",
			req: connector.UpdateRequest{
				Table:  "users",
				Record: map[string]interface{}{"active": true},
				IDs:    []interface{}{1, 2, 3},
			},
			wantSQL:  `UPDATE "TESTUSER"."users" SET "active" = :1 WHERE "id" IN (:2, :3, :4)`,
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
			wantSQL:  `UPDATE "TESTUSER"."users" SET "name" = :1 WHERE status = 'active' AND "id" IN (:2)`,
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
			wantSQL:  `DELETE FROM "TESTUSER"."users" WHERE status = 'deleted'`,
			wantArgs: nil,
		},
		{
			name: "delete with IDs",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{1, 2, 3},
			},
			wantSQL:  `DELETE FROM "TESTUSER"."users" WHERE "id" IN (:1, :2, :3)`,
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name: "delete with single ID",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{42},
			},
			wantSQL:  `DELETE FROM "TESTUSER"."users" WHERE "id" IN (:1)`,
			wantArgs: []interface{}{42},
		},
		{
			name: "delete with filter and IDs",
			req: connector.DeleteRequest{
				Table:  "users",
				Filter: "active = 0",
				IDs:    []interface{}{10, 20},
			},
			wantSQL:  `DELETE FROM "TESTUSER"."users" WHERE active = 0 AND "id" IN (:1, :2)`,
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
			wantSQL:  `SELECT COUNT(*) FROM "TESTUSER"."users"`,
			wantArgs: nil,
		},
		{
			name: "count with filter",
			req: connector.CountRequest{
				Table:  "users",
				Filter: "active = 1",
			},
			wantSQL:  `SELECT COUNT(*) FROM "TESTUSER"."users" WHERE active = 1`,
			wantArgs: nil,
		},
		{
			name: "count with complex filter",
			req: connector.CountRequest{
				Table:  "orders",
				Filter: "status = 'pending' AND total > 100",
			},
			wantSQL:  `SELECT COUNT(*) FROM "TESTUSER"."orders" WHERE status = 'pending' AND total > 100`,
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

func TestOracleDialect(t *testing.T) {
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

	t.Run("ParameterPlaceholder returns colon-numbered placeholders", func(t *testing.T) {
		for i, want := range []string{":1", ":2", ":3", ":10"} {
			indices := []int{1, 2, 3, 10}
			got := c.ParameterPlaceholder(indices[i])
			if got != want {
				t.Errorf("ParameterPlaceholder(%d) = %s, want %s", indices[i], got, want)
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

	t.Run("DriverName is oracle", func(t *testing.T) {
		if c.DriverName() != "oracle" {
			t.Errorf("expected DriverName() == oracle, got %s", c.DriverName())
		}
	})
}

// TestBuildInsertNoReturning verifies that Oracle INSERT does NOT include RETURNING.
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
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("INSERT should NOT contain RETURNING for Oracle, got: %s", sql)
	}
}

// TestBuildSelectSchemaQualified verifies that table references are
// schema-qualified with the configured schema name.
func TestBuildSelectSchemaQualified(t *testing.T) {
	c := &OracleConnector{schemaName: "MYSCHEMA"}
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{Table: "users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT * FROM "MYSCHEMA"."users"`
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
	wantSQL := `INSERT INTO "TESTUSER"."users" ("apple", "mango", "zebra") VALUES (:1, :2, :3)`
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
	wantSQL := `UPDATE "TESTUSER"."users" SET "apple" = :1, "zebra" = :2 WHERE id = 1`
	if sql != wantSQL {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantArgs := []interface{}{"a", "z"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, wantArgs)
	}
}

// TestBuildSelectPagination verifies Oracle 12c+ OFFSET/FETCH pagination.
func TestBuildSelectPagination(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, args, err := c.BuildSelect(ctx, connector.SelectRequest{
		Table:  "users",
		Limit:  50,
		Offset: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sql != `SELECT * FROM "TESTUSER"."users" OFFSET :1 ROWS FETCH NEXT :2 ROWS ONLY` {
		t.Errorf("unexpected SQL: %s", sql)
	}
	if len(args) != 2 || args[0] != 100 || args[1] != 50 {
		t.Errorf("unexpected args: %v", args)
	}
}
