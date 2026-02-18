package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
)

// BuildSelect constructs a SELECT query from the given request.
// SQLite uses double-quote identifier quoting and ? parameter placeholders.
func (c *SQLiteConnector) BuildSelect(_ context.Context, req connector.SelectRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}

	var b strings.Builder
	var args []interface{}

	// SELECT clause
	b.WriteString("SELECT ")
	if len(req.Fields) > 0 {
		quoted := make([]string, len(req.Fields))
		for i, f := range req.Fields {
			quoted[i] = c.QuoteIdentifier(f)
		}
		b.WriteString(strings.Join(quoted, ", "))
	} else {
		b.WriteString("*")
	}

	// FROM clause — SQLite doesn't use schema-qualified names for the main db
	b.WriteString(" FROM ")
	b.WriteString(c.QuoteIdentifier(req.Table))

	// WHERE clause
	if req.Filter != "" {
		b.WriteString(" WHERE ")
		b.WriteString(req.Filter)
	}

	// ORDER BY clause
	if req.Order != "" {
		b.WriteString(" ORDER BY ")
		b.WriteString(req.Order)
	}

	// LIMIT clause
	if req.Limit > 0 {
		b.WriteString(" LIMIT ?")
		args = append(args, req.Limit)
	}

	// OFFSET clause
	if req.Offset > 0 {
		b.WriteString(" OFFSET ?")
		args = append(args, req.Offset)
	}

	return b.String(), args, nil
}

// BuildInsert constructs an INSERT query for SQLite.
// SQLite 3.35+ supports RETURNING, so we include it.
func (c *SQLiteConnector) BuildInsert(_ context.Context, req connector.InsertRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}
	if len(req.Records) == 0 {
		return "", nil, fmt.Errorf("at least one record is required")
	}

	// Extract column names from the first record in deterministic order
	firstRecord := req.Records[0]
	columns := make([]string, 0, len(firstRecord))
	for col := range firstRecord {
		columns = append(columns, col)
	}
	sortStrings(columns)

	var b strings.Builder
	var args []interface{}

	// INSERT INTO
	b.WriteString("INSERT INTO ")
	b.WriteString(c.QuoteIdentifier(req.Table))

	// Column list
	b.WriteString(" (")
	quotedCols := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = c.QuoteIdentifier(col)
	}
	b.WriteString(strings.Join(quotedCols, ", "))
	b.WriteString(")")

	// VALUES clause with multiple rows
	b.WriteString(" VALUES ")
	for rowIdx, record := range req.Records {
		if rowIdx > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for colIdx, col := range columns {
			if colIdx > 0 {
				b.WriteString(", ")
			}
			b.WriteString("?")
			args = append(args, record[col])
		}
		b.WriteString(")")
	}

	// RETURNING clause
	b.WriteString(" RETURNING *")

	return b.String(), args, nil
}

// BuildUpdate constructs an UPDATE query with parameterized SET values.
// Includes RETURNING clause since SQLite 3.35+ supports it.
func (c *SQLiteConnector) BuildUpdate(_ context.Context, req connector.UpdateRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}
	if len(req.Record) == 0 {
		return "", nil, fmt.Errorf("at least one field to update is required")
	}
	if req.Filter == "" && len(req.IDs) == 0 {
		return "", nil, fmt.Errorf("filter or IDs required for update (refusing to update all rows)")
	}

	// Extract column names in deterministic order
	columns := make([]string, 0, len(req.Record))
	for col := range req.Record {
		columns = append(columns, col)
	}
	sortStrings(columns)

	var b strings.Builder
	var args []interface{}

	// UPDATE
	b.WriteString("UPDATE ")
	b.WriteString(c.QuoteIdentifier(req.Table))

	// SET clause
	b.WriteString(" SET ")
	for i, col := range columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(c.QuoteIdentifier(col))
		b.WriteString(" = ?")
		args = append(args, req.Record[col])
	}

	// WHERE clause
	b.WriteString(" WHERE ")
	whereParts := make([]string, 0, 2)

	if req.Filter != "" {
		whereParts = append(whereParts, req.Filter)
	}

	if len(req.IDs) > 0 {
		placeholders := make([]string, len(req.IDs))
		for i, id := range req.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		idClause := fmt.Sprintf("%s IN (%s)", c.QuoteIdentifier("id"), strings.Join(placeholders, ", "))
		whereParts = append(whereParts, idClause)
	}

	b.WriteString(strings.Join(whereParts, " AND "))

	// RETURNING clause
	b.WriteString(" RETURNING *")

	return b.String(), args, nil
}

// BuildDelete constructs a DELETE query with parameterized WHERE conditions.
func (c *SQLiteConnector) BuildDelete(_ context.Context, req connector.DeleteRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}
	if req.Filter == "" && len(req.IDs) == 0 {
		return "", nil, fmt.Errorf("filter or IDs required for delete (refusing to delete all rows)")
	}

	var b strings.Builder
	var args []interface{}

	// DELETE FROM
	b.WriteString("DELETE FROM ")
	b.WriteString(c.QuoteIdentifier(req.Table))

	// WHERE clause
	b.WriteString(" WHERE ")
	whereParts := make([]string, 0, 2)

	if req.Filter != "" {
		whereParts = append(whereParts, req.Filter)
	}

	if len(req.IDs) > 0 {
		placeholders := make([]string, len(req.IDs))
		for i, id := range req.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		idClause := fmt.Sprintf("%s IN (%s)", c.QuoteIdentifier("id"), strings.Join(placeholders, ", "))
		whereParts = append(whereParts, idClause)
	}

	b.WriteString(strings.Join(whereParts, " AND "))

	return b.String(), args, nil
}

// BuildCount constructs a SELECT COUNT(*) query with optional filtering.
func (c *SQLiteConnector) BuildCount(_ context.Context, req connector.CountRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}

	var b strings.Builder

	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteString(c.QuoteIdentifier(req.Table))

	if req.Filter != "" {
		b.WriteString(" WHERE ")
		b.WriteString(req.Filter)
	}

	return b.String(), nil, nil
}

// CreateTable creates a new table from a TableSchema definition.
func (c *SQLiteConnector) CreateTable(ctx context.Context, def model.TableSchema) error {
	if def.Name == "" {
		return fmt.Errorf("table name is required")
	}

	var b strings.Builder

	b.WriteString("CREATE TABLE ")
	b.WriteString(c.QuoteIdentifier(def.Name))
	b.WriteString(" (\n")

	for i, col := range def.Columns {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  ")
		b.WriteString(c.QuoteIdentifier(col.Name))
		b.WriteString(" ")

		if col.IsAutoIncrement {
			b.WriteString("INTEGER PRIMARY KEY AUTOINCREMENT")
		} else {
			b.WriteString(goTypeToSQLite(col))
			if !col.Nullable {
				b.WriteString(" NOT NULL")
			}
			if col.Default != nil {
				b.WriteString(" DEFAULT ")
				b.WriteString(*col.Default)
			}
		}
	}

	// Primary key constraint (skip if a column already has AUTOINCREMENT)
	hasAutoIncrement := false
	for _, col := range def.Columns {
		if col.IsAutoIncrement {
			hasAutoIncrement = true
			break
		}
	}

	if len(def.PrimaryKey) > 0 && !hasAutoIncrement {
		b.WriteString(",\n  PRIMARY KEY (")
		quotedPKs := make([]string, len(def.PrimaryKey))
		for i, pk := range def.PrimaryKey {
			quotedPKs[i] = c.QuoteIdentifier(pk)
		}
		b.WriteString(strings.Join(quotedPKs, ", "))
		b.WriteString(")")
	}

	// Foreign key constraints
	for _, fk := range def.ForeignKeys {
		b.WriteString(",\n  CONSTRAINT ")
		b.WriteString(c.QuoteIdentifier(fk.Name))
		b.WriteString(" FOREIGN KEY (")
		b.WriteString(c.QuoteIdentifier(fk.ColumnName))
		b.WriteString(") REFERENCES ")
		b.WriteString(c.QuoteIdentifier(fk.ReferencedTable))
		b.WriteString(" (")
		b.WriteString(c.QuoteIdentifier(fk.ReferencedColumn))
		b.WriteString(")")
		if fk.OnDelete != "" {
			b.WriteString(" ON DELETE ")
			b.WriteString(fk.OnDelete)
		}
		if fk.OnUpdate != "" {
			b.WriteString(" ON UPDATE ")
			b.WriteString(fk.OnUpdate)
		}
	}

	b.WriteString("\n)")

	_, err := c.db.ExecContext(ctx, b.String())
	if err != nil {
		return fmt.Errorf("create table %q: %w", def.Name, err)
	}
	return nil
}

// AlterTable applies a list of schema changes to an existing table.
// Note: SQLite has limited ALTER TABLE support (no DROP COLUMN before 3.35,
// no MODIFY COLUMN). We use the supported operations where possible.
func (c *SQLiteConnector) AlterTable(ctx context.Context, tableName string, changes []connector.SchemaChange) error {
	if tableName == "" {
		return fmt.Errorf("table name is required")
	}
	if len(changes) == 0 {
		return nil
	}

	for _, change := range changes {
		var stmt string

		switch change.Type {
		case "add_column":
			if change.Definition == nil {
				return fmt.Errorf("column definition required for add_column")
			}
			colType := goTypeToSQLite(*change.Definition)
			nullStr := ""
			if !change.Definition.Nullable {
				nullStr = " NOT NULL"
			}
			defaultStr := ""
			if change.Definition.Default != nil {
				defaultStr = " DEFAULT " + *change.Definition.Default
			}
			stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s%s",
				c.QuoteIdentifier(tableName),
				c.QuoteIdentifier(change.Column),
				colType,
				nullStr,
				defaultStr,
			)

		case "drop_column":
			// SQLite 3.35.0+ supports DROP COLUMN
			stmt = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
				c.QuoteIdentifier(tableName),
				c.QuoteIdentifier(change.Column),
			)

		case "rename_column":
			if change.NewName == "" {
				return fmt.Errorf("new name required for rename_column")
			}
			// SQLite 3.25.0+ supports RENAME COLUMN
			stmt = fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
				c.QuoteIdentifier(tableName),
				c.QuoteIdentifier(change.Column),
				c.QuoteIdentifier(change.NewName),
			)

		case "modify_column":
			return fmt.Errorf("SQLite does not support MODIFY COLUMN; recreate the table instead")

		default:
			return fmt.Errorf("unsupported schema change type: %s", change.Type)
		}

		if _, err := c.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("alter table %q (%s %s): %w", tableName, change.Type, change.Column, err)
		}
	}

	return nil
}

// DropTable drops a table from the database.
func (c *SQLiteConnector) DropTable(ctx context.Context, tableName string) error {
	if tableName == "" {
		return fmt.Errorf("table name is required")
	}

	stmt := fmt.Sprintf("DROP TABLE %s", c.QuoteIdentifier(tableName))

	if _, err := c.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("drop table %q: %w", tableName, err)
	}
	return nil
}

// CallProcedure is not supported for SQLite (no stored procedures).
func (c *SQLiteConnector) CallProcedure(_ context.Context, name string, _ map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("SQLite does not support stored procedures (attempted to call %q)", name)
}

// goTypeToSQLite maps a model.Column's GoType to a SQLite column type.
func goTypeToSQLite(col model.Column) string {
	// If the original DB type is set, use it directly
	if col.Type != "" {
		return col.Type
	}

	switch col.GoType {
	case "int32", "int64":
		return "INTEGER"
	case "float32", "float64":
		return "REAL"
	case "string":
		return "TEXT"
	case "bool":
		return "INTEGER" // SQLite stores booleans as 0/1
	case "time.Time":
		return "TEXT" // SQLite stores dates as TEXT, REAL, or INTEGER
	case "[]byte":
		return "BLOB"
	case "interface{}":
		return "TEXT" // JSON stored as TEXT in SQLite
	default:
		return "TEXT"
	}
}

// sortStrings sorts a string slice in place using a simple insertion sort.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
