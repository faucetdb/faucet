package mssql

import (
	"context"
	"reflect"
	"testing"

	"github.com/faucetdb/faucet/internal/connector"
)

// newTestConnector creates a MSSQLConnector with a known schema name
// and no database connection, suitable for testing query building methods.
func newTestConnector() *MSSQLConnector {
	return &MSSQLConnector{schemaName: "dbo"}
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
			wantSQL:  "SELECT * FROM [dbo].[users]",
			wantArgs: nil,
		},
		{
			name: "select with field selection",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"id", "name", "email"},
			},
			wantSQL:  "SELECT [id], [name], [email] FROM [dbo].[users]",
			wantArgs: nil,
		},
		{
			name: "select with single field",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"name"},
			},
			wantSQL:  "SELECT [name] FROM [dbo].[users]",
			wantArgs: nil,
		},
		{
			name: "select with filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > @p1",
			},
			wantSQL:  "SELECT * FROM [dbo].[users] WHERE age > @p1",
			wantArgs: nil,
		},
		{
			name: "select with complex filter",
			req: connector.SelectRequest{
				Table:  "users",
				Filter: "age > @p1 AND status = @p2",
			},
			wantSQL:  "SELECT * FROM [dbo].[users] WHERE age > @p1 AND status = @p2",
			wantArgs: nil,
		},
		{
			name: "select with ordering",
			req: connector.SelectRequest{
				Table: "users",
				Order: "[created_at] DESC",
			},
			wantSQL:  "SELECT * FROM [dbo].[users] ORDER BY [created_at] DESC",
			wantArgs: nil,
		},
		{
			name: "select with limit only uses OFFSET FETCH",
			req: connector.SelectRequest{
				Table: "users",
				Limit: 10,
			},
			wantSQL:  "SELECT * FROM [dbo].[users] ORDER BY (SELECT NULL) OFFSET @p1 ROWS FETCH NEXT @p2 ROWS ONLY",
			wantArgs: []interface{}{0, 10},
		},
		{
			name: "select with limit and offset",
			req: connector.SelectRequest{
				Table:  "users",
				Limit:  10,
				Offset: 20,
			},
			wantSQL:  "SELECT * FROM [dbo].[users] ORDER BY (SELECT NULL) OFFSET @p1 ROWS FETCH NEXT @p2 ROWS ONLY",
			wantArgs: []interface{}{20, 10},
		},
		{
			name: "select with offset but no limit",
			req: connector.SelectRequest{
				Table:  "users",
				Offset: 20,
			},
			wantSQL:  "SELECT * FROM [dbo].[users] ORDER BY (SELECT NULL) OFFSET @p1 ROWS",
			wantArgs: []interface{}{20},
		},
		{
			name: "select with all options",
			req: connector.SelectRequest{
				Table:  "orders",
				Fields: []string{"id", "total"},
				Filter: "status = @p1",
				Order:  "[total] DESC",
				Limit:  25,
				Offset: 50,
			},
			wantSQL:  "SELECT [id], [total] FROM [dbo].[orders] WHERE status = @p1 ORDER BY [total] DESC OFFSET @p1 ROWS FETCH NEXT @p2 ROWS ONLY",
			wantArgs: []interface{}{50, 25},
		},
		{
			name: "pagination with explicit ORDER BY does not add SELECT NULL",
			req: connector.SelectRequest{
				Table: "users",
				Order: "[id] ASC",
				Limit: 5,
			},
			wantSQL:  "SELECT * FROM [dbo].[users] ORDER BY [id] ASC OFFSET @p1 ROWS FETCH NEXT @p2 ROWS ONLY",
			wantArgs: []interface{}{0, 5},
		},
		{
			name: "field names with special characters are quoted",
			req: connector.SelectRequest{
				Table:  "users",
				Fields: []string{"first name", "last name"},
			},
			wantSQL:  "SELECT [first name], [last name] FROM [dbo].[users]",
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
			wantSQL:  "INSERT INTO [dbo].[users] ([email], [name]) OUTPUT INSERTED.* VALUES (@p1, @p2)",
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
			wantSQL:  "INSERT INTO [dbo].[users] ([email], [name]) OUTPUT INSERTED.* VALUES (@p1, @p2), (@p3, @p4)",
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
			wantSQL:  "INSERT INTO [dbo].[tags] ([name]) OUTPUT INSERTED.* VALUES (@p1)",
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
			wantSQL:  "INSERT INTO [dbo].[products] ([name], [price]) OUTPUT INSERTED.* VALUES (@p1, @p2)",
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
			wantSQL:  "INSERT INTO [dbo].[items] ([val]) OUTPUT INSERTED.* VALUES (@p1), (@p2), (@p3)",
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
				Filter: "id = @p1",
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
			wantSQL:  "UPDATE [dbo].[users] SET [name] = @p1 OUTPUT INSERTED.* WHERE status = 'inactive'",
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
			wantSQL:  "UPDATE [dbo].[users] SET [email] = @p1, [name] = @p2 OUTPUT INSERTED.* WHERE id = 1",
			wantArgs: []interface{}{"new@example.com", "New Name"},
		},
		{
			name: "update with IDs",
			req: connector.UpdateRequest{
				Table:  "users",
				Record: map[string]interface{}{"active": true},
				IDs:    []interface{}{1, 2, 3},
			},
			wantSQL:  "UPDATE [dbo].[users] SET [active] = @p1 OUTPUT INSERTED.* WHERE [id] IN (@p2, @p3, @p4)",
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
			wantSQL:  "UPDATE [dbo].[users] SET [name] = @p1 OUTPUT INSERTED.* WHERE status = 'active' AND [id] IN (@p2)",
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
			wantSQL:  "DELETE FROM [dbo].[users] WHERE status = 'deleted'",
			wantArgs: nil,
		},
		{
			name: "delete with IDs",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{1, 2, 3},
			},
			wantSQL:  "DELETE FROM [dbo].[users] WHERE [id] IN (@p1, @p2, @p3)",
			wantArgs: []interface{}{1, 2, 3},
		},
		{
			name: "delete with single ID",
			req: connector.DeleteRequest{
				Table: "users",
				IDs:   []interface{}{42},
			},
			wantSQL:  "DELETE FROM [dbo].[users] WHERE [id] IN (@p1)",
			wantArgs: []interface{}{42},
		},
		{
			name: "delete with filter and IDs",
			req: connector.DeleteRequest{
				Table:  "users",
				Filter: "active = false",
				IDs:    []interface{}{10, 20},
			},
			wantSQL:  "DELETE FROM [dbo].[users] WHERE active = false AND [id] IN (@p1, @p2)",
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
			wantSQL:  "SELECT COUNT(*) FROM [dbo].[users]",
			wantArgs: nil,
		},
		{
			name: "count with filter",
			req: connector.CountRequest{
				Table:  "users",
				Filter: "active = true",
			},
			wantSQL:  "SELECT COUNT(*) FROM [dbo].[users] WHERE active = true",
			wantArgs: nil,
		},
		{
			name: "count with complex filter",
			req: connector.CountRequest{
				Table:  "orders",
				Filter: "status = 'pending' AND total > 100",
			},
			wantSQL:  "SELECT COUNT(*) FROM [dbo].[orders] WHERE status = 'pending' AND total > 100",
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

func TestMSSQLDialect(t *testing.T) {
	c := newTestConnector()

	t.Run("QuoteIdentifier uses brackets", func(t *testing.T) {
		got := c.QuoteIdentifier("users")
		want := "[users]"
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})

	t.Run("QuoteIdentifier escapes embedded closing brackets", func(t *testing.T) {
		got := c.QuoteIdentifier("my]table")
		want := "[my]]table]"
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})

	t.Run("ParameterPlaceholder returns @pN format", func(t *testing.T) {
		cases := map[int]string{
			1:   "@p1",
			2:   "@p2",
			3:   "@p3",
			100: "@p100",
		}
		for idx, want := range cases {
			got := c.ParameterPlaceholder(idx)
			if got != want {
				t.Errorf("ParameterPlaceholder(%d) = %s, want %s", idx, got, want)
			}
		}
	})

	t.Run("SupportsReturning is false", func(t *testing.T) {
		if c.SupportsReturning() {
			t.Error("expected SupportsReturning() == false")
		}
	})

	t.Run("SupportsUpsert is true", func(t *testing.T) {
		if !c.SupportsUpsert() {
			t.Error("expected SupportsUpsert() == true")
		}
	})

	t.Run("DriverName is mssql", func(t *testing.T) {
		if c.DriverName() != "mssql" {
			t.Errorf("expected DriverName() == mssql, got %s", c.DriverName())
		}
	})
}

// TestBuildInsertUsesOutputInserted verifies that MSSQL INSERT uses
// OUTPUT INSERTED.* instead of RETURNING.
func TestBuildInsertUsesOutputInserted(t *testing.T) {
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
	if !contains(sql, "OUTPUT INSERTED.*") {
		t.Errorf("MSSQL INSERT should contain OUTPUT INSERTED.*, got: %s", sql)
	}
	if contains(sql, "RETURNING") {
		t.Errorf("MSSQL INSERT should not contain RETURNING, got: %s", sql)
	}
}

// TestBuildUpdateUsesOutputInserted verifies that MSSQL UPDATE uses
// OUTPUT INSERTED.* instead of RETURNING.
func TestBuildUpdateUsesOutputInserted(t *testing.T) {
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
	if !contains(sql, "OUTPUT INSERTED.*") {
		t.Errorf("MSSQL UPDATE should contain OUTPUT INSERTED.*, got: %s", sql)
	}
	if contains(sql, "RETURNING") {
		t.Errorf("MSSQL UPDATE should not contain RETURNING, got: %s", sql)
	}
}

// TestBuildSelectSchemaQualified verifies that table references are
// schema-qualified with the configured schema name.
func TestBuildSelectSchemaQualified(t *testing.T) {
	c := &MSSQLConnector{schemaName: "myschema"}
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{Table: "users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT * FROM [myschema].[users]"
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
	wantSQL := "INSERT INTO [dbo].[users] ([apple], [mango], [zebra]) OUTPUT INSERTED.* VALUES (@p1, @p2, @p3)"
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
	wantSQL := "UPDATE [dbo].[users] SET [apple] = @p1, [zebra] = @p2 OUTPUT INSERTED.* WHERE id = 1"
	if sql != wantSQL {
		t.Errorf("SQL mismatch\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantArgs := []interface{}{"a", "z"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args mismatch\n  got:  %v\n  want: %v", args, wantArgs)
	}
}

// TestMSSQLUsesAtPPlaceholders verifies that MSSQL uses @p1, @p2, etc. for
// parameter placeholders in generated queries.
func TestMSSQLUsesAtPPlaceholders(t *testing.T) {
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
		t.Errorf("MSSQL should not use $ placeholders, got: %s", sql)
	}
	if contains(sql, "?") {
		t.Errorf("MSSQL should not use ? placeholders, got: %s", sql)
	}
	if !contains(sql, "@p") {
		t.Errorf("MSSQL should use @p placeholders, got: %s", sql)
	}
}

// TestMSSQLSelectPaginationRequiresOrderBy verifies that OFFSET/FETCH NEXT
// pagination automatically adds ORDER BY (SELECT NULL) when no order is specified.
func TestMSSQLSelectPaginationRequiresOrderBy(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{
		Table: "users",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(sql, "ORDER BY (SELECT NULL)") {
		t.Errorf("MSSQL pagination without explicit ORDER should add ORDER BY (SELECT NULL), got: %s", sql)
	}
	if !contains(sql, "OFFSET") {
		t.Errorf("expected OFFSET in SQL, got: %s", sql)
	}
	if !contains(sql, "FETCH NEXT") {
		t.Errorf("expected FETCH NEXT in SQL, got: %s", sql)
	}
}

// TestMSSQLSelectNoPaginationNoOrderByInjected verifies that when there is no
// pagination, no ORDER BY (SELECT NULL) is added.
func TestMSSQLSelectNoPaginationNoOrderByInjected(t *testing.T) {
	c := newTestConnector()
	ctx := context.Background()

	sql, _, err := c.BuildSelect(ctx, connector.SelectRequest{
		Table: "users",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(sql, "ORDER BY") {
		t.Errorf("MSSQL SELECT without pagination should not add ORDER BY, got: %s", sql)
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
