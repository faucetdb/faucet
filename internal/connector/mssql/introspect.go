package mssql

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/model"
)

// columnRow holds the result of querying information_schema.columns for SQL Server.
type columnRow struct {
	TableName  string  `db:"TABLE_NAME"`
	ColumnName string  `db:"COLUMN_NAME"`
	DataType   string  `db:"DATA_TYPE"`
	IsNullable string  `db:"IS_NULLABLE"`
	Default    *string `db:"COLUMN_DEFAULT"`
	MaxLength  *int64  `db:"CHARACTER_MAXIMUM_LENGTH"`
	Position   int     `db:"ORDINAL_POSITION"`
}

// identityRow holds identity (auto-increment) information from sys.columns.
type identityRow struct {
	TableName  string `db:"table_name"`
	ColumnName string `db:"column_name"`
}

// tableRow holds the result of querying information_schema.tables.
type tableRow struct {
	TableName string `db:"TABLE_NAME"`
	TableType string `db:"TABLE_TYPE"`
}

// pkRow holds a primary key column mapping.
type pkRow struct {
	TableName  string `db:"TABLE_NAME"`
	ColumnName string `db:"COLUMN_NAME"`
}

// fkRow holds a foreign key relationship.
type fkRow struct {
	TableName        string `db:"TABLE_NAME"`
	ColumnName       string `db:"COLUMN_NAME"`
	ReferencedTable  string `db:"REFERENCED_TABLE_NAME"`
	ReferencedColumn string `db:"REFERENCED_COLUMN_NAME"`
	DeleteRule       string `db:"DELETE_RULE"`
	UpdateRule       string `db:"UPDATE_RULE"`
}

// routineRow holds a stored procedure or function from information_schema.routines.
type routineRow struct {
	RoutineName string  `db:"ROUTINE_NAME"`
	RoutineType string  `db:"ROUTINE_TYPE"`
	DataType    *string `db:"DATA_TYPE"`
}

// IntrospectSchema returns the full schema for the configured SQL Server
// schema, including all tables, views, procedures, and functions.
func (c *MSSQLConnector) IntrospectSchema(ctx context.Context) (*model.Schema, error) {
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

	// Fetch identity columns
	identities, err := c.fetchIdentityColumns(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect identity columns: %w", err)
	}

	// Build identity lookup: "table.column" -> true
	identityMap := make(map[string]bool)
	for _, id := range identities {
		identityMap[id.TableName+"."+id.ColumnName] = true
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
		isAuto := identityMap[col.TableName+"."+col.ColumnName]

		goType, jsonType := mapMSSQLType(col.DataType)

		colMap[col.TableName] = append(colMap[col.TableName], model.Column{
			Name:            col.ColumnName,
			Position:        col.Position,
			Type:            col.DataType,
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
		returnType := ""
		if r.DataType != nil {
			returnType = *r.DataType
		}
		sp := model.StoredProcedure{
			Name:       r.RoutineName,
			ReturnType: returnType,
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
func (c *MSSQLConnector) IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error) {
	// Verify the table exists and get its type
	const tableQuery = `SELECT TABLE_NAME, TABLE_TYPE FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = @p1 AND TABLE_NAME = @p2`

	var t tableRow
	if err := c.db.GetContext(ctx, &t, tableQuery, c.schemaName, tableName); err != nil {
		return nil, fmt.Errorf("table %q not found in schema %q: %w", tableName, c.schemaName, err)
	}

	// Fetch columns for this specific table
	columns, err := c.fetchColumns(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("introspect columns for %q: %w", tableName, err)
	}

	// Fetch identity columns for this table
	const identityQuery = `SELECT t.name AS table_name, col.name AS column_name
		FROM sys.columns col
		JOIN sys.tables t ON col.object_id = t.object_id
		JOIN sys.schemas s ON t.schema_id = s.schema_id
		WHERE s.name = @p1 AND t.name = @p2 AND col.is_identity = 1`

	var identities []identityRow
	if err := c.db.SelectContext(ctx, &identities, identityQuery, c.schemaName, tableName); err != nil {
		return nil, fmt.Errorf("introspect identity columns for %q: %w", tableName, err)
	}

	identitySet := make(map[string]bool, len(identities))
	for _, id := range identities {
		identitySet[id.ColumnName] = true
	}

	// Fetch primary keys for this table
	const pkQuery = `SELECT kcu.TABLE_NAME, kcu.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
			ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
			AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
		WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
			AND tc.TABLE_SCHEMA = @p1
			AND tc.TABLE_NAME = @p2`

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
			fk_tab.name AS TABLE_NAME,
			fk_col.name AS COLUMN_NAME,
			pk_tab.name AS REFERENCED_TABLE_NAME,
			pk_col.name AS REFERENCED_COLUMN_NAME,
			fk.delete_referential_action_desc AS DELETE_RULE,
			fk.update_referential_action_desc AS UPDATE_RULE
		FROM sys.foreign_keys fk
		JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
		JOIN sys.tables fk_tab ON fkc.parent_object_id = fk_tab.object_id
		JOIN sys.columns fk_col ON fkc.parent_object_id = fk_col.object_id AND fkc.parent_column_id = fk_col.column_id
		JOIN sys.tables pk_tab ON fkc.referenced_object_id = pk_tab.object_id
		JOIN sys.columns pk_col ON fkc.referenced_object_id = pk_col.object_id AND fkc.referenced_column_id = pk_col.column_id
		JOIN sys.schemas s ON fk_tab.schema_id = s.schema_id
		WHERE s.name = @p1 AND fk_tab.name = @p2`

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
		isAuto := identitySet[col.ColumnName]
		goType, jsonType := mapMSSQLType(col.DataType)

		modelColumns = append(modelColumns, model.Column{
			Name:            col.ColumnName,
			Position:        col.Position,
			Type:            col.DataType,
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
func (c *MSSQLConnector) GetTableNames(ctx context.Context) ([]string, error) {
	const query = `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = @p1 AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`

	var names []string
	if err := c.db.SelectContext(ctx, &names, query, c.schemaName); err != nil {
		return nil, fmt.Errorf("get table names: %w", err)
	}
	return names, nil
}

// GetStoredProcedures returns all stored procedures and functions in the
// configured schema.
func (c *MSSQLConnector) GetStoredProcedures(ctx context.Context) ([]model.StoredProcedure, error) {
	routines, err := c.fetchRoutines(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]model.StoredProcedure, 0, len(routines))
	for _, r := range routines {
		returnType := ""
		if r.DataType != nil {
			returnType = *r.DataType
		}
		sp := model.StoredProcedure{
			Name:       r.RoutineName,
			ReturnType: returnType,
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

func (c *MSSQLConnector) fetchTables(ctx context.Context) ([]tableRow, error) {
	const query = `SELECT TABLE_NAME, TABLE_TYPE
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = @p1
		ORDER BY TABLE_NAME`

	var rows []tableRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *MSSQLConnector) fetchColumns(ctx context.Context, tableName string) ([]columnRow, error) {
	query := `SELECT
			c.TABLE_NAME,
			c.COLUMN_NAME,
			c.DATA_TYPE,
			c.IS_NULLABLE,
			c.COLUMN_DEFAULT,
			c.CHARACTER_MAXIMUM_LENGTH,
			c.ORDINAL_POSITION
		FROM INFORMATION_SCHEMA.COLUMNS c
		WHERE c.TABLE_SCHEMA = @p1`

	args := []interface{}{c.schemaName}

	if tableName != "" {
		query += ` AND c.TABLE_NAME = @p2`
		args = append(args, tableName)
	}

	query += ` ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION`

	var rows []columnRow
	if err := c.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *MSSQLConnector) fetchIdentityColumns(ctx context.Context) ([]identityRow, error) {
	const query = `SELECT t.name AS table_name, col.name AS column_name
		FROM sys.columns col
		JOIN sys.tables t ON col.object_id = t.object_id
		JOIN sys.schemas s ON t.schema_id = s.schema_id
		WHERE s.name = @p1 AND col.is_identity = 1`

	var rows []identityRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *MSSQLConnector) fetchPrimaryKeys(ctx context.Context) ([]pkRow, error) {
	const query = `SELECT kcu.TABLE_NAME, kcu.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
			ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
			AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
		WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
			AND tc.TABLE_SCHEMA = @p1`

	var rows []pkRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *MSSQLConnector) fetchForeignKeys(ctx context.Context) ([]fkRow, error) {
	const query = `SELECT
			fk_tab.name AS TABLE_NAME,
			fk_col.name AS COLUMN_NAME,
			pk_tab.name AS REFERENCED_TABLE_NAME,
			pk_col.name AS REFERENCED_COLUMN_NAME,
			fk.delete_referential_action_desc AS DELETE_RULE,
			fk.update_referential_action_desc AS UPDATE_RULE
		FROM sys.foreign_keys fk
		JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
		JOIN sys.tables fk_tab ON fkc.parent_object_id = fk_tab.object_id
		JOIN sys.columns fk_col ON fkc.parent_object_id = fk_col.object_id AND fkc.parent_column_id = fk_col.column_id
		JOIN sys.tables pk_tab ON fkc.referenced_object_id = pk_tab.object_id
		JOIN sys.columns pk_col ON fkc.referenced_object_id = pk_col.object_id AND fkc.referenced_column_id = pk_col.column_id
		JOIN sys.schemas s ON fk_tab.schema_id = s.schema_id
		WHERE s.name = @p1`

	var rows []fkRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *MSSQLConnector) fetchRoutines(ctx context.Context) ([]routineRow, error) {
	const query = `SELECT ROUTINE_NAME, ROUTINE_TYPE, DATA_TYPE
		FROM INFORMATION_SCHEMA.ROUTINES
		WHERE ROUTINE_SCHEMA = @p1
		ORDER BY ROUTINE_NAME`

	var rows []routineRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

// mapMSSQLType maps a SQL Server data type to a Go type string and a JSON
// Schema type string.
func mapMSSQLType(dataType string) (goType, jsonType string) {
	switch strings.ToLower(dataType) {
	case "tinyint", "smallint", "int":
		return "int32", "integer"
	case "bigint":
		return "int64", "integer"
	case "float", "real":
		return "float64", "number"
	case "decimal", "numeric", "money", "smallmoney":
		return "float64", "number"
	case "varchar", "nvarchar", "char", "nchar", "text", "ntext":
		return "string", "string"
	case "datetime", "datetime2", "smalldatetime":
		return "time.Time", "string(date-time)"
	case "datetimeoffset":
		return "time.Time", "string(date-time)"
	case "date":
		return "time.Time", "string(date)"
	case "time":
		return "string", "string(time)"
	case "bit":
		return "bool", "boolean"
	case "uniqueidentifier":
		return "string", "string(uuid)"
	case "varbinary", "binary", "image":
		return "[]byte", "string(byte)"
	case "xml":
		return "string", "string"
	case "sql_variant":
		return "interface{}", "string"
	case "hierarchyid":
		return "string", "string"
	case "geography", "geometry":
		return "string", "string"
	default:
		return "interface{}", "string"
	}
}
