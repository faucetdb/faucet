package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/model"
)

// columnRow holds the result of querying information_schema.columns.
type columnRow struct {
	TableName  string  `db:"table_name"`
	ColumnName string  `db:"column_name"`
	DataType   string  `db:"data_type"`
	IsNullable string  `db:"is_nullable"`
	Default    *string `db:"column_default"`
	MaxLength  *int64  `db:"character_maximum_length"`
	Position   int     `db:"ordinal_position"`
	UDTName    string  `db:"udt_name"`
}

// tableRow holds the result of querying information_schema.tables.
type tableRow struct {
	TableName string `db:"table_name"`
	TableType string `db:"table_type"`
}

// pkRow holds a primary key column mapping.
type pkRow struct {
	TableName  string `db:"table_name"`
	ColumnName string `db:"column_name"`
}

// fkRow holds a foreign key relationship.
type fkRow struct {
	TableName        string `db:"table_name"`
	ColumnName       string `db:"column_name"`
	ReferencedTable  string `db:"referenced_table"`
	ReferencedColumn string `db:"referenced_column"`
	DeleteRule       string `db:"delete_rule"`
	UpdateRule       string `db:"update_rule"`
}

// routineRow holds a stored procedure or function from information_schema.routines.
type routineRow struct {
	RoutineName string `db:"routine_name"`
	RoutineType string `db:"routine_type"`
	DataType    string `db:"data_type"`
}

// IntrospectSchema returns the full schema for the configured PostgreSQL
// schema, including all tables, views, procedures, and functions.
func (c *PostgresConnector) IntrospectSchema(ctx context.Context) (*model.Schema, error) {
	// Fetch tables and views
	tables, err := c.fetchTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect tables: %w", err)
	}

	// Fetch all columns in the schema
	columns, err := c.fetchColumns(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("introspect columns: %w", err)
	}

	// Fetch primary keys
	pks, err := c.fetchPrimaryKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect primary keys: %w", err)
	}

	// Fetch foreign keys
	fks, err := c.fetchForeignKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect foreign keys: %w", err)
	}

	// Build primary key lookup: table_name -> set of pk column names
	pkMap := make(map[string]map[string]bool)
	for _, pk := range pks {
		if pkMap[pk.TableName] == nil {
			pkMap[pk.TableName] = make(map[string]bool)
		}
		pkMap[pk.TableName][pk.ColumnName] = true
	}

	// Build foreign key lookup: table_name -> []ForeignKey
	fkMap := make(map[string][]model.ForeignKey)
	for _, fk := range fks {
		fkMap[fk.TableName] = append(fkMap[fk.TableName], model.ForeignKey{
			Name:             fmt.Sprintf("fk_%s_%s", fk.TableName, fk.ColumnName),
			ColumnName:       fk.ColumnName,
			ReferencedTable:  fk.ReferencedTable,
			ReferencedColumn: fk.ReferencedColumn,
			OnDelete:         fk.DeleteRule,
			OnUpdate:         fk.UpdateRule,
		})
	}

	// Build column lookup: table_name -> []Column
	colMap := make(map[string][]model.Column)
	for _, col := range columns {
		isPK := pkMap[col.TableName] != nil && pkMap[col.TableName][col.ColumnName]
		isAuto := col.Default != nil && strings.Contains(*col.Default, "nextval")

		goType, jsonType := mapPostgresType(col.UDTName, col.DataType)

		colMap[col.TableName] = append(colMap[col.TableName], model.Column{
			Name:            col.ColumnName,
			Position:        col.Position,
			Type:            col.UDTName,
			GoType:          goType,
			JsonType:        jsonType,
			Nullable:        col.IsNullable == "YES",
			Default:         col.Default,
			MaxLength:       col.MaxLength,
			IsPrimaryKey:    isPK,
			IsAutoIncrement: isAuto,
		})
	}

	// Build primary key column name lists: table_name -> []string
	pkColMap := make(map[string][]string)
	for _, pk := range pks {
		pkColMap[pk.TableName] = append(pkColMap[pk.TableName], pk.ColumnName)
	}

	// Assemble table schemas
	schema := &model.Schema{}

	for _, t := range tables {
		ts := model.TableSchema{
			Name:        t.TableName,
			Columns:     colMap[t.TableName],
			PrimaryKey:  pkColMap[t.TableName],
			ForeignKeys: fkMap[t.TableName],
		}

		if ts.Columns == nil {
			ts.Columns = []model.Column{}
		}
		if ts.PrimaryKey == nil {
			ts.PrimaryKey = []string{}
		}
		if ts.ForeignKeys == nil {
			ts.ForeignKeys = []model.ForeignKey{}
		}

		switch t.TableType {
		case "VIEW":
			ts.Type = "view"
			schema.Views = append(schema.Views, ts)
		default:
			ts.Type = "table"
			schema.Tables = append(schema.Tables, ts)
		}
	}

	// Fetch procedures and functions
	routines, err := c.fetchRoutines(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect routines: %w", err)
	}

	for _, r := range routines {
		sp := model.StoredProcedure{
			Name:       r.RoutineName,
			ReturnType: r.DataType,
		}
		switch strings.ToUpper(r.RoutineType) {
		case "PROCEDURE":
			sp.Type = "procedure"
			schema.Procedures = append(schema.Procedures, sp)
		default:
			sp.Type = "function"
			schema.Functions = append(schema.Functions, sp)
		}
	}

	// Ensure nil slices are empty slices for clean JSON
	if schema.Tables == nil {
		schema.Tables = []model.TableSchema{}
	}
	if schema.Views == nil {
		schema.Views = []model.TableSchema{}
	}
	if schema.Procedures == nil {
		schema.Procedures = []model.StoredProcedure{}
	}
	if schema.Functions == nil {
		schema.Functions = []model.StoredProcedure{}
	}

	return schema, nil
}

// IntrospectTable returns the schema for a single table or view.
func (c *PostgresConnector) IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error) {
	// Verify the table exists and get its type
	const tableQuery = `SELECT table_name, table_type FROM information_schema.tables
		WHERE table_schema = $1 AND table_name = $2`

	var t tableRow
	if err := c.db.GetContext(ctx, &t, tableQuery, c.schemaName, tableName); err != nil {
		return nil, fmt.Errorf("table %q not found in schema %q: %w", tableName, c.schemaName, err)
	}

	// Fetch columns for this specific table
	columns, err := c.fetchColumns(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("introspect columns for %q: %w", tableName, err)
	}

	// Fetch primary keys for this table
	const pkQuery = `SELECT kcu.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_schema = $1
			AND tc.table_name = $2`

	var pks []pkRow
	if err := c.db.SelectContext(ctx, &pks, pkQuery, c.schemaName, tableName); err != nil {
		return nil, fmt.Errorf("introspect primary keys for %q: %w", tableName, err)
	}

	pkSet := make(map[string]bool, len(pks))
	pkCols := make([]string, 0, len(pks))
	for _, pk := range pks {
		pkSet[pk.ColumnName] = true
		pkCols = append(pkCols, pk.ColumnName)
	}

	// Fetch foreign keys for this table
	const fkQuery = `SELECT
			tc.table_name,
			kcu.column_name,
			ccu.table_name AS referenced_table,
			ccu.column_name AS referenced_column,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
		JOIN information_schema.referential_constraints rc
			ON tc.constraint_name = rc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = $1
			AND tc.table_name = $2`

	var fks []fkRow
	if err := c.db.SelectContext(ctx, &fks, fkQuery, c.schemaName, tableName); err != nil {
		return nil, fmt.Errorf("introspect foreign keys for %q: %w", tableName, err)
	}

	foreignKeys := make([]model.ForeignKey, 0, len(fks))
	for _, fk := range fks {
		foreignKeys = append(foreignKeys, model.ForeignKey{
			Name:             fmt.Sprintf("fk_%s_%s", fk.TableName, fk.ColumnName),
			ColumnName:       fk.ColumnName,
			ReferencedTable:  fk.ReferencedTable,
			ReferencedColumn: fk.ReferencedColumn,
			OnDelete:         fk.DeleteRule,
			OnUpdate:         fk.UpdateRule,
		})
	}

	// Build columns with pk/auto-increment info
	modelColumns := make([]model.Column, 0, len(columns))
	for _, col := range columns {
		isPK := pkSet[col.ColumnName]
		isAuto := col.Default != nil && strings.Contains(*col.Default, "nextval")
		goType, jsonType := mapPostgresType(col.UDTName, col.DataType)

		modelColumns = append(modelColumns, model.Column{
			Name:            col.ColumnName,
			Position:        col.Position,
			Type:            col.UDTName,
			GoType:          goType,
			JsonType:        jsonType,
			Nullable:        col.IsNullable == "YES",
			Default:         col.Default,
			MaxLength:       col.MaxLength,
			IsPrimaryKey:    isPK,
			IsAutoIncrement: isAuto,
		})
	}

	tableType := "table"
	if t.TableType == "VIEW" {
		tableType = "view"
	}

	return &model.TableSchema{
		Name:        tableName,
		Type:        tableType,
		Columns:     modelColumns,
		PrimaryKey:  pkCols,
		ForeignKeys: foreignKeys,
		Indexes:     []model.Index{},
	}, nil
}

// GetTableNames returns a list of all table names in the configured schema.
func (c *PostgresConnector) GetTableNames(ctx context.Context) ([]string, error) {
	const query = `SELECT table_name FROM information_schema.tables
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name`

	var names []string
	if err := c.db.SelectContext(ctx, &names, query, c.schemaName); err != nil {
		return nil, fmt.Errorf("get table names: %w", err)
	}
	return names, nil
}

// GetStoredProcedures returns all stored procedures and functions in the
// configured schema.
func (c *PostgresConnector) GetStoredProcedures(ctx context.Context) ([]model.StoredProcedure, error) {
	routines, err := c.fetchRoutines(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]model.StoredProcedure, 0, len(routines))
	for _, r := range routines {
		sp := model.StoredProcedure{
			Name:       r.RoutineName,
			ReturnType: r.DataType,
		}
		switch strings.ToUpper(r.RoutineType) {
		case "PROCEDURE":
			sp.Type = "procedure"
		default:
			sp.Type = "function"
		}
		result = append(result, sp)
	}
	return result, nil
}

// --- internal fetch helpers ---

func (c *PostgresConnector) fetchTables(ctx context.Context) ([]tableRow, error) {
	const query = `SELECT table_name, table_type
		FROM information_schema.tables
		WHERE table_schema = $1
		ORDER BY table_name`

	var rows []tableRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *PostgresConnector) fetchColumns(ctx context.Context, tableName string) ([]columnRow, error) {
	query := `SELECT
			c.table_name,
			c.column_name,
			c.data_type,
			c.is_nullable,
			c.column_default,
			c.character_maximum_length,
			c.ordinal_position,
			c.udt_name
		FROM information_schema.columns c
		WHERE c.table_schema = $1`

	args := []interface{}{c.schemaName}

	if tableName != "" {
		query += ` AND c.table_name = $2`
		args = append(args, tableName)
	}

	query += ` ORDER BY c.table_name, c.ordinal_position`

	var rows []columnRow
	if err := c.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *PostgresConnector) fetchPrimaryKeys(ctx context.Context) ([]pkRow, error) {
	const query = `SELECT kcu.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_schema = $1`

	var rows []pkRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *PostgresConnector) fetchForeignKeys(ctx context.Context) ([]fkRow, error) {
	const query = `SELECT
			tc.table_name,
			kcu.column_name,
			ccu.table_name AS referenced_table,
			ccu.column_name AS referenced_column,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
		JOIN information_schema.referential_constraints rc
			ON tc.constraint_name = rc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = $1`

	var rows []fkRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *PostgresConnector) fetchRoutines(ctx context.Context) ([]routineRow, error) {
	const query = `SELECT routine_name, routine_type, data_type
		FROM information_schema.routines
		WHERE routine_schema = $1
		ORDER BY routine_name`

	var rows []routineRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

// mapPostgresType maps a PostgreSQL UDT name and data_type to a Go type string
// and a JSON Schema type string.
func mapPostgresType(udtName, dataType string) (goType, jsonType string) {
	switch strings.ToLower(udtName) {
	case "int2", "smallint":
		return "int32", "integer"
	case "int4", "integer", "serial":
		return "int32", "integer"
	case "int8", "bigint", "bigserial":
		return "int64", "integer"
	case "float4", "real":
		return "float32", "number"
	case "float8", "double precision":
		return "float64", "number"
	case "numeric", "decimal":
		return "float64", "number"
	case "varchar", "character varying", "char", "character", "text", "name", "citext":
		return "string", "string"
	case "bool", "boolean":
		return "bool", "boolean"
	case "timestamp", "timestamptz", "timestamp without time zone", "timestamp with time zone":
		return "time.Time", "string(date-time)"
	case "date":
		return "time.Time", "string(date)"
	case "time", "timetz", "time without time zone", "time with time zone":
		return "string", "string(time)"
	case "uuid":
		return "string", "string(uuid)"
	case "json", "jsonb":
		return "interface{}", "object"
	case "bytea":
		return "[]byte", "string(byte)"
	case "inet", "cidr", "macaddr":
		return "string", "string"
	case "interval":
		return "string", "string"
	case "point", "line", "lseg", "box", "path", "polygon", "circle":
		return "string", "string"
	case "xml":
		return "string", "string"
	case "money":
		return "string", "string"
	case "tsvector", "tsquery":
		return "string", "string"
	default:
		// Fall back to the data_type column for USER-DEFINED or ARRAY types
		lower := strings.ToLower(dataType)
		if lower == "array" {
			return "interface{}", "array"
		}
		if lower == "user-defined" {
			return "string", "string"
		}
		return "interface{}", "string"
	}
}
