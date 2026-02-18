package postgres

import (
	"context"
	"reflect"
	"testing"

	"github.com/faucetdb/faucet/internal/connector"
)

// newTestConnector creates a PostgresConnector with a known schema name
// and no database connection, suitable for testing query building methods.
func newTestConnector() *PostgresConnector {
	return &PostgresConnector{schemaName: "public"}
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
			name:     "empty table returns error",
			req:      connector.SelectRequest{},
			wantErr:  true,
		},
		{
			name: "simple select all",
			req: connector.SelectRequest{
				Table: "users",
			},
			wantSQL:  `SELECT * FROM "public"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with field selection",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"id", "name", "email"},
			},
			wantSQL:  `SELECT "id", "name", "email" FROM "public"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with single field",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"name"},
			},
			wantSQL:  `SELECT "name" FROM "public"."users"`,
			wantArgs: nil,
		},
		{
			name: "select with filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > $1",
			},
			wantSQL:  `SELECT * FROM "public"."users" WHERE age > $1`,
			wantArgs: nil,
		},
		{
			name: "select with complex filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > $1 AND status = $2",
			},
			wantSQL:  `SELECT * FROM "public"."users" WHERE age > $1 AND status = $2`,
			wantArgs: nil,
		},
		{
			name: "select with ordering",
			req: connector.SelectRequest{
				Table: "users",
				Order: `"created_at" DESC`,
			},
			wantSQL:  `SELECT * FROM "public"."users" ORDER BY "created_at" DESC`,
			wantArgs: nil,
		},
		{
			name: "select with limit only",
			req: connector.SelectRequest{
				Table: "users",
				Limit: 10,
			},
			wantSQL:  `SELECT * FROM "public"."users" LIMIT $1`,
			wantArgs: []interface{}{10},
		},
		{
			name: "select with limit and offset",
			req: connector.SelectRequest{
				Table:  "users",
				Limit:  10,
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "public"."users" LIMIT $1 OFFSET $2`,
			wantArgs: []interface{}{10, 20},
		},
		{
			name: "select with offset but no limit (offset ignored by implementation)",
			req: connector.SelectRequest{
				Table:  "users",
				Offset: 20,
			},
			wantSQL:  `SELECT * FROM "public"."users" OFFSET $1`,
			wantArgs: []interface{}{20},
		},
		{
			name: "select with all options",
			req: connector.SelectRequest{
				Table:  "orders",
				Fields: []string{"id", "total"},
				Filter: "status = $1",
				Order:  `"total" DESC`,
				Limit:  25,
				Offset: 50,
			},
			// Note: paramIdx starts at 1 for LIMIT/OFFSET regardless of
			// filter content, since filters are passed as raw SQL strings.
			wantSQL:  `SELECT "id", "total" FROM "public"."orders" WHERE status = $1 ORDER BY "total" DESC LIMIT $1 OFFSET $2`,
			wantArgs: []interface{}{25, 50},
		},
		{
			name: "field names with special characters are quoted",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"first name", "last name"},
			},
			wantSQL:  `SELECT "first name", "last name" FROM "public"."users"`,
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
			wantSQL:  `INSERT INTO "public"."users" ("email", "name") VALUES ($1, $2) RETURNING *`,
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
			wantSQL:  `INSERT INTO "public"."users" ("email", "name") VALUES ($1, $2), ($3, $4) RETURNING *`,
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
			wantSQL:  `INSERT INTO "public"."tags" ("name") VALUES ($1) RETURNING *`,
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
			wantSQL:  `INSERT INTO "public"."products" ("name", "price") VALUES ($1, $2) RETURNING *`,
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
			wantSQL:  `INSERT INTO "public"."items" ("val") VALUES ($1), ($2), ($3) RETURNING *`,
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
				Filter: "id = $1",
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
			wantSQL:  `UPDATE "public"."users" SET "name" = $1 WHERE status = 'inactive' RETURNING *`,
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
			wantSQL:  `UPDATE "public"."users" SET "email" = $1, "name" = $2 WHERE id = 1 RETURNING *`,
			wantArgs: []interface{}{"new@example.com", "New Name"},
		},
		{
			name: "update with IDs",
			req: connector.UpdateRequest{
				Table:  "users",
				Record: map[string]interface{}{"active": true},
				IDs:    []interface{}{1, 2, 3},
			},
			wantSQL:  `UPDATE "public"."users" SET "active" = $1 WHERE "id" IN ($2, $3, $4) RETURNING *`,
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
			wantSQL:  `UPDATE "public"."users" SET "name" = $1 WHERE status = 'active' AND "id" IN ($2) RETURNING *`,
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
			wantSQL:  `DELETE FROM "public"."users" WHERE status = 'deleted'`,
			wantArgs: nil,
		},
		{
			name: "delete with IDs",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{1, 2, 3},
			},
			wantSQL:  `DELETE FROM "public"."users" WHERE "id" IN ($1, $2, $3)`,
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name: "delete with single ID",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{42},
			},
			wantSQL:  `DELETE FROM "public"."users" WHERE "id" IN ($1)`,
			wantArgs: []interface{}{42},
		},
		{
			name: "delete with filter and IDs",
			req: connector.DeleteRequest{
				Table:  "users",
				Filter: "active = false",
				IDs:    []interface{}{10, 20},
			},
			wantSQL:  `DELETE FROM "public"."users" WHERE active = false AND "id" IN ($1, $2)`,
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
			wantSQL:  `SELECT COUNT(*) FROM "public"."users"`,
			wantArgs: nil,
		},
		{
			name: "count with filter",
			req: connector.CountRequest{
				Table:  "users",
				Filter: "active = true",
			},
			wantSQL:  `SELECT COUNT(*) FROM "public"."users" WHERE active = true`,
			wantArgs: nil,
		},
		{
			name: "count with complex filter",
			req: connector.CountRequest{
				Table:  "orders",
				Filter: "status = 'pending' AND total > 100",
			},
			wantSQL:  `SELECT COUNT(*) FROM "public"."orders" WHERE status = 'pending' AND total > 100`,
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

func TestPostgresDialect(t *testing.T) {
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

	t.Run("ParameterPlaceholder returns dollar-numbered placeholders", func(t *testing.T) {
		for i, want := range []string{"$1", "$2", "$3", "$10"} {
			indices := []int{1, 2, 3, 10}
			got := c.ParameterPlaceholder(indices[i])
			if got != want {
				t.Errorf("ParameterPlaceholder(%d) = %s, want %s", indices[i], got, want)
			}
		}
	})

	t.Run("SupportsReturning is true", func(t *testing.T) {
		if !c.SupportsReturning() {
			t.Error("expected SupportsReturning() == true")
		}
	})

	t.Run("DriverName is postgres", func(t *testing.T) {
		if c.DriverName() != "postgres" {
			t.Errorf("expected DriverName() == postgres, got %s", c.DriverName())
		}
	})
}

// TestBuildInsertIncludesReturning verifies that PostgreSQL INSERT always
// includes RETURNING *.
func TestBuildInsertIncludesReturning(t *testing.T) {
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
	suffix := " RETURNING *"
	if len(sql) < len(suffix) || sql[len(sql)-len(suffix):] != suffix {
		t.Errorf("INSERT should end with RETURNING *, got: %s", sql)
	}
}

// TestBuildUpdateIncludesReturning verifies that PostgreSQL UPDATE always
// includes RETURNING *.
func TestBuildUpdateIncludesReturning(t *testing.T) {
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
	suffix := " RETURNING *"
	if len(sql) < len(suffix) || sql[len(sql)-len(suffix):] != suffix {
		t.Errorf("UPDATE should end with RETURNING *, got: %s", sql)
	}
}

// TestBuildSelectSchemaQualified verifies that table references are
// schema-qualified with the configured schema name.
func TestBuildSelectSchemaQualified(t *testing.T) {
	c := &PostgresConnector{schemaName: "myschema"}
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{Table: "users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT * FROM "myschema"."users"`
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
	wantSQL := `INSERT INTO "public"."users" ("apple", "mango", "zebra") VALUES ($1, $2, $3) RETURNING *`
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
	wantSQL := `UPDATE "public"."users" SET "apple" = $1, "zebra" = $2 WHERE id = 1 RETURNING *`
	if sql != wantSQL {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantArgs := []interface{}{"a", "z"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, wantArgs)
	}
}

// TestBuildSelectParameterIndexing verifies that LIMIT and OFFSET use
// correctly incrementing $N placeholders.
func TestBuildSelectParameterIndexing(t *testing.T) {
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
	if sql != `SELECT * FROM "public"."users" LIMIT $1 OFFSET $2` {
		t.Errorf("unexpected SQL: %s", sql)
	}
	if len(args) != 2 || args[0] != 50 || args[1] != 100 {
		t.Errorf("unexpected args: %v", args)
	}
}
