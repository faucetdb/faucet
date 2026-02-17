package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
)

// BuildSelect constructs a SELECT query from the given request.
// It quotes all identifiers, applies field selection, filtering, ordering,
// and pagination using PostgreSQL $N parameter placeholders.
func (c *PostgresConnector) BuildSelect(_ context.Context, req connector.SelectRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}

	var b strings.Builder
	var args []interface{}
	paramIdx := 1

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

	// FROM clause
	b.WriteString(" FROM ")
	b.WriteString(c.QuoteIdentifier(c.schemaName))
	b.WriteString(".")
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
		b.WriteString(fmt.Sprintf(" LIMIT $%d", paramIdx))
		args = append(args, req.Limit)
		paramIdx++
	}

	// OFFSET clause
	if req.Offset > 0 {
		b.WriteString(fmt.Sprintf(" OFFSET $%d", paramIdx))
		args = append(args, req.Offset)
		paramIdx++
	}

	return b.String(), args, nil
}

// BuildInsert constructs an INSERT query with RETURNING * for PostgreSQL.
// It supports batch inserts with multiple value rows. All column names are
// derived from the first record, and subsequent records must have the same
// columns.
func (c *PostgresConnector) BuildInsert(_ context.Context, req connector.InsertRequest) (string, []interface{}, error) {
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
	// Sort for deterministic output
	sortStrings(columns)

	var b strings.Builder
	var args []interface{}
	paramIdx := 1

	// INSERT INTO
	b.WriteString("INSERT INTO ")
	b.WriteString(c.QuoteIdentifier(c.schemaName))
	b.WriteString(".")
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
			b.WriteString(fmt.Sprintf("$%d", paramIdx))
			args = append(args, record[col])
			paramIdx++
		}
		b.WriteString(")")
	}

	// RETURNING * for PostgreSQL
	b.WriteString(" RETURNING *")

	return b.String(), args, nil
}

// BuildUpdate constructs an UPDATE query with parameterized SET values.
// It supports both filter-based and ID-based updates. When IDs are provided
// without a filter, the first primary key column would need to be specified;
// for now, IDs are applied as an IN clause on the "id" column by convention.
func (c *PostgresConnector) BuildUpdate(_ context.Context, req connector.UpdateRequest) (string, []interface{}, error) {
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
	paramIdx := 1

	// UPDATE
	b.WriteString("UPDATE ")
	b.WriteString(c.QuoteIdentifier(c.schemaName))
	b.WriteString(".")
	b.WriteString(c.QuoteIdentifier(req.Table))

	// SET clause
	b.WriteString(" SET ")
	for i, col := range columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(c.QuoteIdentifier(col))
		b.WriteString(fmt.Sprintf(" = $%d", paramIdx))
		args = append(args, req.Record[col])
		paramIdx++
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
			placeholders[i] = fmt.Sprintf("$%d", paramIdx)
			args = append(args, id)
			paramIdx++
		}
		idClause := fmt.Sprintf("%s IN (%s)", c.QuoteIdentifier("id"), strings.Join(placeholders, ", "))
		whereParts = append(whereParts, idClause)
	}

	b.WriteString(strings.Join(whereParts, " AND "))

	// RETURNING * for PostgreSQL
	b.WriteString(" RETURNING *")

	return b.String(), args, nil
}

// BuildDelete constructs a DELETE query with parameterized WHERE conditions.
// It supports both filter-based and ID-based deletes.
func (c *PostgresConnector) BuildDelete(_ context.Context, req connector.DeleteRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}
	if req.Filter == "" && len(req.IDs) == 0 {
		return "", nil, fmt.Errorf("filter or IDs required for delete (refusing to delete all rows)")
	}

	var b strings.Builder
	var args []interface{}
	paramIdx := 1

	// DELETE FROM
	b.WriteString("DELETE FROM ")
	b.WriteString(c.QuoteIdentifier(c.schemaName))
	b.WriteString(".")
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
			placeholders[i] = fmt.Sprintf("$%d", paramIdx)
			args = append(args, id)
			paramIdx++
		}
		idClause := fmt.Sprintf("%s IN (%s)", c.QuoteIdentifier("id"), strings.Join(placeholders, ", "))
		whereParts = append(whereParts, idClause)
	}

	b.WriteString(strings.Join(whereParts, " AND "))

	return b.String(), args, nil
}

// BuildCount constructs a SELECT COUNT(*) query with optional filtering.
func (c *PostgresConnector) BuildCount(_ context.Context, req connector.CountRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}

	var b strings.Builder

	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteString(c.QuoteIdentifier(c.schemaName))
	b.WriteString(".")
	b.WriteString(c.QuoteIdentifier(req.Table))

	if req.Filter != "" {
		b.WriteString(" WHERE ")
		b.WriteString(req.Filter)
	}

	return b.String(), nil, nil
}

// CreateTable creates a new table from a TableSchema definition, translating
// Go/model types back to PostgreSQL column types.
func (c *PostgresConnector) CreateTable(ctx context.Context, def model.TableSchema) error {
	if def.Name == "" {
		return fmt.Errorf("table name is required")
	}

	var b strings.Builder

	b.WriteString("CREATE TABLE ")
	b.WriteString(c.QuoteIdentifier(c.schemaName))
	b.WriteString(".")
	b.WriteString(c.QuoteIdentifier(def.Name))
	b.WriteString(" (\n")

	for i, col := range def.Columns {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString("  ")
		b.WriteString(c.QuoteIdentifier(col.Name))
		b.WriteString(" ")

		// Use auto-increment types for serial columns
		if col.IsAutoIncrement {
			if col.GoType == "int64" {
				b.WriteString("BIGSERIAL")
			} else {
				b.WriteString("SERIAL")
			}
		} else {
			b.WriteString(goTypeToPostgres(col))
		}

		if !col.Nullable {
			b.WriteString(" NOT NULL")
		}
		if col.Default != nil && !col.IsAutoIncrement {
			b.WriteString(" DEFAULT ")
			b.WriteString(*col.Default)
		}
	}

	// Primary key constraint
	if len(def.PrimaryKey) > 0 {
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
		b.WriteString(c.QuoteIdentifier(c.schemaName))
		b.WriteString(".")
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
func (c *PostgresConnector) AlterTable(ctx context.Context, tableName string, changes []connector.SchemaChange) error {
	if tableName == "" {
		return fmt.Errorf("table name is required")
	}
	if len(changes) == 0 {
		return nil
	}

	qualifiedTable := c.QuoteIdentifier(c.schemaName) + "." + c.QuoteIdentifier(tableName)

	for _, change := range changes {
		var stmt string

		switch change.Type {
		case "add_column":
			if change.Definition == nil {
				return fmt.Errorf("column definition required for add_column")
			}
			colType := goTypeToPostgres(*change.Definition)
			nullStr := ""
			if !change.Definition.Nullable {
				nullStr = " NOT NULL"
			}
			defaultStr := ""
			if change.Definition.Default != nil {
				defaultStr = " DEFAULT " + *change.Definition.Default
			}
			stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s%s",
				qualifiedTable,
				c.QuoteIdentifier(change.Column),
				colType,
				nullStr,
				defaultStr,
			)

		case "drop_column":
			stmt = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
				qualifiedTable,
				c.QuoteIdentifier(change.Column),
			)

		case "rename_column":
			if change.NewName == "" {
				return fmt.Errorf("new name required for rename_column")
			}
			stmt = fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s",
				qualifiedTable,
				c.QuoteIdentifier(change.Column),
				c.QuoteIdentifier(change.NewName),
			)

		case "modify_column":
			if change.Definition == nil {
				return fmt.Errorf("column definition required for modify_column")
			}
			colType := goTypeToPostgres(*change.Definition)
			stmt = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s",
				qualifiedTable,
				c.QuoteIdentifier(change.Column),
				colType,
			)

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
func (c *PostgresConnector) DropTable(ctx context.Context, tableName string) error {
	if tableName == "" {
		return fmt.Errorf("table name is required")
	}

	stmt := fmt.Sprintf("DROP TABLE %s.%s",
		c.QuoteIdentifier(c.schemaName),
		c.QuoteIdentifier(tableName),
	)

	if _, err := c.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("drop table %q: %w", tableName, err)
	}
	return nil
}

// CallProcedure executes a stored procedure or function using SELECT * FROM
// notation, which works for PostgreSQL functions that return result sets.
func (c *PostgresConnector) CallProcedure(ctx context.Context, name string, params map[string]interface{}) ([]map[string]interface{}, error) {
	if name == "" {
		return nil, fmt.Errorf("procedure name is required")
	}

	// Build parameter list
	paramNames := make([]string, 0, len(params))
	for k := range params {
		paramNames = append(paramNames, k)
	}
	sortStrings(paramNames)

	placeholders := make([]string, len(paramNames))
	args := make([]interface{}, len(paramNames))
	for i, pn := range paramNames {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = params[pn]
	}

	query := fmt.Sprintf("SELECT * FROM %s.%s(%s)",
		c.QuoteIdentifier(c.schemaName),
		c.QuoteIdentifier(name),
		strings.Join(placeholders, ", "),
	)

	rows, err := c.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("call procedure %q: %w", name, err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, fmt.Errorf("scan procedure result: %w", err)
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate procedure results: %w", err)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return results, nil
}

// goTypeToPostgres maps a model.Column's GoType back to a PostgreSQL type
// for use in CREATE TABLE and ALTER TABLE statements.
func goTypeToPostgres(col model.Column) string {
	// If the original DB type is set and looks like a real PG type, use it
	if col.Type != "" {
		return pgTypeWithLength(col.Type, col.MaxLength)
	}

	// Fall back to mapping from GoType
	switch col.GoType {
	case "int32":
		return "INTEGER"
	case "int64":
		return "BIGINT"
	case "float32":
		return "REAL"
	case "float64":
		return "DOUBLE PRECISION"
	case "string":
		if col.MaxLength != nil {
			return fmt.Sprintf("VARCHAR(%d)", *col.MaxLength)
		}
		return "TEXT"
	case "bool":
		return "BOOLEAN"
	case "time.Time":
		return "TIMESTAMPTZ"
	case "[]byte":
		return "BYTEA"
	case "interface{}":
		return "JSONB"
	default:
		return "TEXT"
	}
}

// pgTypeWithLength appends a length specifier to varchar/char types if a
// max length is defined.
func pgTypeWithLength(typeName string, maxLength *int64) string {
	lower := strings.ToLower(typeName)
	if maxLength != nil && (lower == "varchar" || lower == "character varying" || lower == "char" || lower == "character") {
		return fmt.Sprintf("%s(%d)", typeName, *maxLength)
	}
	return typeName
}

// sortStrings sorts a string slice in place using a simple insertion sort.
// This avoids importing "sort" for a small utility used only on column name
// lists (typically < 100 elements).
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
