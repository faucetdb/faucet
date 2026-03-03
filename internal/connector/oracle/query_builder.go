package oracle

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
)

// BuildSelect constructs a SELECT query from the given request.
// It quotes all identifiers, applies field selection, filtering, ordering,
// and pagination using Oracle :N parameter placeholders.
// Uses Oracle 12c+ OFFSET/FETCH syntax for pagination.
func (c *OracleConnector) BuildSelect(_ context.Context, req connector.SelectRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}

	var b strings.Builder
	var args []interface{}
	args = append(args, req.FilterArgs...)
	paramIdx := len(args) + 1

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

	// Oracle 12c+ pagination: OFFSET n ROWS FETCH NEXT m ROWS ONLY
	if req.Offset > 0 {
		b.WriteString(fmt.Sprintf(" OFFSET :%d ROWS", paramIdx))
		args = append(args, req.Offset)
		paramIdx++
	}

	if req.Limit > 0 {
		if req.Offset == 0 {
			// Must include OFFSET 0 for FETCH to work properly
			b.WriteString(fmt.Sprintf(" OFFSET :%d ROWS", paramIdx))
			args = append(args, 0)
			paramIdx++
		}
		b.WriteString(fmt.Sprintf(" FETCH NEXT :%d ROWS ONLY", paramIdx))
		args = append(args, req.Limit)
		paramIdx++ //nolint:ineffassign // keep paramIdx consistent for future clauses
	}

	return b.String(), args, nil
}

// BuildInsert constructs an INSERT query for Oracle.
// Oracle does not support multi-row VALUES in a single INSERT, so for
// multiple records it uses INSERT ALL ... SELECT FROM DUAL syntax.
func (c *OracleConnector) BuildInsert(_ context.Context, req connector.InsertRequest) (string, []interface{}, error) {
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

	qualifiedTable := c.QuoteIdentifier(c.schemaName) + "." + c.QuoteIdentifier(req.Table)

	var b strings.Builder
	var args []interface{}
	paramIdx := 1

	if len(req.Records) == 1 {
		// Single record: standard INSERT INTO ... VALUES (...)
		b.WriteString("INSERT INTO ")
		b.WriteString(qualifiedTable)

		// Column list
		b.WriteString(" (")
		quotedCols := make([]string, len(columns))
		for i, col := range columns {
			quotedCols[i] = c.QuoteIdentifier(col)
		}
		b.WriteString(strings.Join(quotedCols, ", "))
		b.WriteString(")")

		// VALUES clause
		b.WriteString(" VALUES (")
		for colIdx, col := range columns {
			if colIdx > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf(":%d", paramIdx))
			args = append(args, req.Records[0][col])
			paramIdx++
		}
		b.WriteString(")")
	} else {
		// Multiple records: INSERT ALL INTO ... VALUES (...) SELECT FROM DUAL
		b.WriteString("INSERT ALL")
		quotedCols := make([]string, len(columns))
		for i, col := range columns {
			quotedCols[i] = c.QuoteIdentifier(col)
		}
		colList := strings.Join(quotedCols, ", ")

		for _, record := range req.Records {
			b.WriteString(" INTO ")
			b.WriteString(qualifiedTable)
			b.WriteString(" (")
			b.WriteString(colList)
			b.WriteString(") VALUES (")
			for colIdx, col := range columns {
				if colIdx > 0 {
					b.WriteString(", ")
				}
				b.WriteString(fmt.Sprintf(":%d", paramIdx))
				args = append(args, record[col])
				paramIdx++
			}
			b.WriteString(")")
		}
		b.WriteString(" SELECT 1 FROM DUAL")
	}

	return b.String(), args, nil
}

// BuildUpdate constructs an UPDATE query with parameterized SET values.
// It supports both filter-based and ID-based updates.
func (c *OracleConnector) BuildUpdate(_ context.Context, req connector.UpdateRequest) (string, []interface{}, error) {
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
		b.WriteString(fmt.Sprintf(" = :%d", paramIdx))
		args = append(args, req.Record[col])
		paramIdx++
	}

	// Append filter args after SET args so placeholders in the filter resolve correctly
	args = append(args, req.FilterArgs...)
	paramIdx += len(req.FilterArgs)

	// WHERE clause
	b.WriteString(" WHERE ")
	whereParts := make([]string, 0, 2)

	if req.Filter != "" {
		whereParts = append(whereParts, req.Filter)
	}

	if len(req.IDs) > 0 {
		placeholders := make([]string, len(req.IDs))
		for i, id := range req.IDs {
			placeholders[i] = fmt.Sprintf(":%d", paramIdx)
			args = append(args, id)
			paramIdx++
		}
		idClause := fmt.Sprintf("%s IN (%s)", c.QuoteIdentifier("id"), strings.Join(placeholders, ", "))
		whereParts = append(whereParts, idClause)
	}

	b.WriteString(strings.Join(whereParts, " AND "))

	return b.String(), args, nil
}

// BuildDelete constructs a DELETE query with parameterized WHERE conditions.
// It supports both filter-based and ID-based deletes.
func (c *OracleConnector) BuildDelete(_ context.Context, req connector.DeleteRequest) (string, []interface{}, error) {
	if req.Table == "" {
		return "", nil, fmt.Errorf("table name is required")
	}
	if req.Filter == "" && len(req.IDs) == 0 {
		return "", nil, fmt.Errorf("filter or IDs required for delete (refusing to delete all rows)")
	}

	var b strings.Builder
	var args []interface{}
	args = append(args, req.FilterArgs...)
	paramIdx := len(args) + 1

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
			placeholders[i] = fmt.Sprintf(":%d", paramIdx)
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
func (c *OracleConnector) BuildCount(_ context.Context, req connector.CountRequest) (string, []interface{}, error) {
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
// Go/model types back to Oracle column types.
func (c *OracleConnector) CreateTable(ctx context.Context, def model.TableSchema) error {
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

		if col.IsAutoIncrement {
			b.WriteString("NUMBER GENERATED BY DEFAULT AS IDENTITY")
		} else {
			b.WriteString(goTypeToOracle(col))
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
	}

	b.WriteString("\n)")

	_, err := c.db.ExecContext(ctx, b.String())
	if err != nil {
		return fmt.Errorf("create table %q: %w", def.Name, err)
	}
	return nil
}

// AlterTable applies a list of schema changes to an existing table.
func (c *OracleConnector) AlterTable(ctx context.Context, tableName string, changes []connector.SchemaChange) error {
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
			colType := goTypeToOracle(*change.Definition)
			nullStr := ""
			if !change.Definition.Nullable {
				nullStr = " NOT NULL"
			}
			defaultStr := ""
			if change.Definition.Default != nil {
				defaultStr = " DEFAULT " + *change.Definition.Default
			}
			stmt = fmt.Sprintf("ALTER TABLE %s ADD %s %s%s%s",
				qualifiedTable,
				c.QuoteIdentifier(change.Column),
				colType,
				defaultStr,
				nullStr,
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
			colType := goTypeToOracle(*change.Definition)
			stmt = fmt.Sprintf("ALTER TABLE %s MODIFY %s %s",
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
func (c *OracleConnector) DropTable(ctx context.Context, tableName string) error {
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

// CallProcedure executes a stored procedure using Oracle's BEGIN ... END; block.
func (c *OracleConnector) CallProcedure(ctx context.Context, name string, params map[string]interface{}) ([]map[string]interface{}, error) {
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
		placeholders[i] = fmt.Sprintf(":%d", i+1)
		args[i] = params[pn]
	}

	// Oracle stored procedure call
	query := fmt.Sprintf("BEGIN %s.%s(%s); END;",
		c.QuoteIdentifier(c.schemaName),
		c.QuoteIdentifier(name),
		strings.Join(placeholders, ", "),
	)

	_, err := c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("call procedure %q: %w", name, err)
	}

	// Oracle procedures don't return result sets via BEGIN...END;
	// Return empty results
	return []map[string]interface{}{}, nil
}

// goTypeToOracle maps a model.Column's GoType back to an Oracle type
// for use in CREATE TABLE and ALTER TABLE statements.
func goTypeToOracle(col model.Column) string {
	// If the original DB type is set and looks like a real Oracle type, use it
	if col.Type != "" {
		return oracleTypeWithLength(col.Type, col.MaxLength)
	}

	// Fall back to mapping from GoType
	switch col.GoType {
	case "int32":
		return "NUMBER(10)"
	case "int64":
		return "NUMBER(19)"
	case "float32":
		return "BINARY_FLOAT"
	case "float64":
		return "BINARY_DOUBLE"
	case "string":
		if col.MaxLength != nil {
			return fmt.Sprintf("VARCHAR2(%d)", *col.MaxLength)
		}
		return "VARCHAR2(4000)"
	case "bool":
		return "NUMBER(1)"
	case "time.Time":
		return "TIMESTAMP"
	case "[]byte":
		return "BLOB"
	case "interface{}":
		return "CLOB"
	default:
		return "VARCHAR2(4000)"
	}
}

// oracleTypeWithLength appends a length specifier to VARCHAR2/CHAR types if a
// max length is defined.
func oracleTypeWithLength(typeName string, maxLength *int64) string {
	upper := strings.ToUpper(typeName)
	if maxLength != nil && (upper == "VARCHAR2" || upper == "NVARCHAR2" || upper == "CHAR" || upper == "NCHAR") {
		return fmt.Sprintf("%s(%d)", typeName, *maxLength)
	}
	return typeName
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
