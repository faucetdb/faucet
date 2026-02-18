package snowflake

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/model"
)

// columnRow holds the result of querying information_schema.columns for Snowflake.
type columnRow struct {
	TableName  string  `db:"TABLE_NAME"`
	ColumnName string  `db:"COLUMN_NAME"`
	DataType   string  `db:"DATA_TYPE"`
	IsNullable string  `db:"IS_NULLABLE"`
	Default    *string `db:"COLUMN_DEFAULT"`
	MaxLength  *int64  `db:"CHARACTER_MAXIMUM_LENGTH"`
	Position   int     `db:"ORDINAL_POSITION"`
	Comment    *string `db:"COMMENT"`
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
	TableName        string `db:"FK_TABLE_NAME"`
	ColumnName       string `db:"FK_COLUMN_NAME"`
	ReferencedTable  string `db:"PK_TABLE_NAME"`
	ReferencedColumn string `db:"PK_COLUMN_NAME"`
	DeleteRule       string `db:"DELETE_RULE"`
	UpdateRule       string `db:"UPDATE_RULE"`
}

// routineRow holds a stored procedure or function from information_schema.
type routineRow struct {
	ProcedureName string `db:"PROCEDURE_NAME"`
}

// IntrospectSchema returns the full schema for the configured Snowflake
// schema, including all tables and views.
func (c *SnowflakeConnector) IntrospectSchema(ctx context.Context) (*model.Schema, error) {
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
		// Snowflake supports AUTOINCREMENT/IDENTITY via default expressions
		isAuto := col.Default != nil && (strings.Contains(strings.ToUpper(*col.Default), "AUTOINCREMENT") ||
			strings.Contains(strings.ToUpper(*col.Default), "IDENTITY"))

		goType, jsonType := mapSnowflakeType(col.DataType)

		comment := ""
		if col.Comment != nil {
			comment = *col.Comment
		}

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
			Comment:         comment,
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

	// Fetch procedures
	routines, err := c.fetchProcedures(ctx)
	if err != nil {
		return nil, fmt.Errorf("introspect procedures: %w", err)
	}

	for _, r := range routines {
		sp := model.StoredProcedure{
			Name: r.ProcedureName,
			Type: "procedure",
		}
		schema.Procedures = append(schema.Procedures, sp)
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
func (c *SnowflakeConnector) IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error) {
	// Verify the table exists and get its type
	const tableQuery = `SELECT TABLE_NAME, TABLE_TYPE FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`

	var t tableRow
	if err := c.db.GetContext(ctx, &t, tableQuery, c.schemaName, tableName); err != nil {
		return nil, fmt.Errorf("table %q not found in schema %q: %w", tableName, c.schemaName, err)
	}

	// Fetch columns for this specific table
	columns, err := c.fetchColumns(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("introspect columns for %q: %w", tableName, err)
	}

	// Fetch primary keys for this table using SHOW PRIMARY KEYS
	pkQuery := fmt.Sprintf(`SHOW PRIMARY KEYS IN TABLE %s.%s`,
		c.QuoteIdentifier(c.schemaName),
		c.QuoteIdentifier(tableName),
	)

	pkRows, err := c.db.QueryxContext(ctx, pkQuery)
	if err != nil {
		return nil, fmt.Errorf("introspect primary keys for %q: %w", tableName, err)
	}
	defer pkRows.Close()

	pkSet := make(map[string]bool)
	var pkCols []string
	for pkRows.Next() {
		row := make(map[string]interface{})
		if err := pkRows.MapScan(row); err != nil {
			return nil, fmt.Errorf("scan primary key row: %w", err)
		}
		if colName, ok := row["column_name"].(string); ok {
			pkSet[colName] = true
			pkCols = append(pkCols, colName)
		}
	}

	// Fetch foreign keys for this table using SHOW IMPORTED KEYS
	fkQuery := fmt.Sprintf(`SHOW IMPORTED KEYS IN TABLE %s.%s`,
		c.QuoteIdentifier(c.schemaName),
		c.QuoteIdentifier(tableName),
	)

	fkRows, err := c.db.QueryxContext(ctx, fkQuery)
	if err != nil {
		return nil, fmt.Errorf("introspect foreign keys for %q: %w", tableName, err)
	}
	defer fkRows.Close()

	var foreignKeys []model.ForeignKey
	for fkRows.Next() {
		row := make(map[string]interface{})
		if err := fkRows.MapScan(row); err != nil {
			return nil, fmt.Errorf("scan foreign key row: %w", err)
		}
		fkColName, _ := row["fk_column_name"].(string)
		pkTableName, _ := row["pk_table_name"].(string)
		pkColName, _ := row["pk_column_name"].(string)
		deleteRule, _ := row["delete_rule"].(string)
		updateRule, _ := row["update_rule"].(string)

		foreignKeys = append(foreignKeys, model.ForeignKey{
			Name:             fmt.Sprintf("fk_%s_%s", tableName, fkColName),
			ColumnName:       fkColName,
			ReferencedTable:  pkTableName,
			ReferencedColumn: pkColName,
			OnDelete:         deleteRule,
			OnUpdate:         updateRule,
		})
	}

	// Build columns with pk/auto-increment info
	modelColumns := make([]model.Column, 0, len(columns))
	for _, col := range columns {
		isPK := pkSet[col.ColumnName]
		isAuto := col.Default != nil && (strings.Contains(strings.ToUpper(*col.Default), "AUTOINCREMENT") ||
			strings.Contains(strings.ToUpper(*col.Default), "IDENTITY"))
		goType, jsonType := mapSnowflakeType(col.DataType)

		comment := ""
		if col.Comment != nil {
			comment = *col.Comment
		}

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
			Comment:         comment,
		})
	}

	if foreignKeys == nil {
		foreignKeys = []model.ForeignKey{}
	}
	if pkCols == nil {
		pkCols = []string{}
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
func (c *SnowflakeConnector) GetTableNames(ctx context.Context) ([]string, error) {
	const query = `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME`

	var names []string
	if err := c.db.SelectContext(ctx, &names, query, c.schemaName); err != nil {
		return nil, fmt.Errorf("get table names: %w", err)
	}
	return names, nil
}

// GetStoredProcedures returns all stored procedures in the configured schema.
func (c *SnowflakeConnector) GetStoredProcedures(ctx context.Context) ([]model.StoredProcedure, error) {
	routines, err := c.fetchProcedures(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]model.StoredProcedure, 0, len(routines))
	for _, r := range routines {
		result = append(result, model.StoredProcedure{
			Name: r.ProcedureName,
			Type: "procedure",
		})
	}
	return result, nil
}

// --- internal fetch helpers ---

func (c *SnowflakeConnector) fetchTables(ctx context.Context) ([]tableRow, error) {
	const query = `SELECT TABLE_NAME, TABLE_TYPE
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME`

	var rows []tableRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *SnowflakeConnector) fetchColumns(ctx context.Context, tableName string) ([]columnRow, error) {
	query := `SELECT
			c.TABLE_NAME,
			c.COLUMN_NAME,
			c.DATA_TYPE,
			c.IS_NULLABLE,
			c.COLUMN_DEFAULT,
			c.CHARACTER_MAXIMUM_LENGTH,
			c.ORDINAL_POSITION,
			c.COMMENT
		FROM INFORMATION_SCHEMA.COLUMNS c
		WHERE c.TABLE_SCHEMA = ?`

	args := []interface{}{c.schemaName}

	if tableName != "" {
		query += ` AND c.TABLE_NAME = ?`
		args = append(args, tableName)
	}

	query += ` ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION`

	var rows []columnRow
	if err := c.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *SnowflakeConnector) fetchPrimaryKeys(ctx context.Context) ([]pkRow, error) {
	// Snowflake: use SHOW PRIMARY KEYS for the schema, then parse results
	query := fmt.Sprintf(`SHOW PRIMARY KEYS IN SCHEMA %s`, c.QuoteIdentifier(c.schemaName))

	rawRows, err := c.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rawRows.Close()

	var rows []pkRow
	for rawRows.Next() {
		row := make(map[string]interface{})
		if err := rawRows.MapScan(row); err != nil {
			return nil, err
		}
		tableName, _ := row["table_name"].(string)
		columnName, _ := row["column_name"].(string)
		rows = append(rows, pkRow{TableName: tableName, ColumnName: columnName})
	}
	return rows, nil
}

func (c *SnowflakeConnector) fetchForeignKeys(ctx context.Context) ([]fkRow, error) {
	// Snowflake: use SHOW IMPORTED KEYS for the schema
	query := fmt.Sprintf(`SHOW IMPORTED KEYS IN SCHEMA %s`, c.QuoteIdentifier(c.schemaName))

	rawRows, err := c.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rawRows.Close()

	var rows []fkRow
	for rawRows.Next() {
		row := make(map[string]interface{})
		if err := rawRows.MapScan(row); err != nil {
			return nil, err
		}
		fkTableName, _ := row["fk_table_name"].(string)
		fkColumnName, _ := row["fk_column_name"].(string)
		pkTableName, _ := row["pk_table_name"].(string)
		pkColumnName, _ := row["pk_column_name"].(string)
		deleteRule, _ := row["delete_rule"].(string)
		updateRule, _ := row["update_rule"].(string)

		rows = append(rows, fkRow{
			TableName:        fkTableName,
			ColumnName:       fkColumnName,
			ReferencedTable:  pkTableName,
			ReferencedColumn: pkColumnName,
			DeleteRule:        deleteRule,
			UpdateRule:        updateRule,
		})
	}
	return rows, nil
}

func (c *SnowflakeConnector) fetchProcedures(ctx context.Context) ([]routineRow, error) {
	const query = `SELECT PROCEDURE_NAME FROM INFORMATION_SCHEMA.PROCEDURES
		WHERE PROCEDURE_SCHEMA = ?
		ORDER BY PROCEDURE_NAME`

	var rows []routineRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

// mapSnowflakeType maps a Snowflake data type to a Go type string and a JSON
// Schema type string.
func mapSnowflakeType(dataType string) (goType, jsonType string) {
	upper := strings.ToUpper(dataType)

	switch upper {
	case "NUMBER", "DECIMAL", "NUMERIC", "FLOAT", "FLOAT4", "FLOAT8", "DOUBLE", "DOUBLE PRECISION", "REAL":
		return "float64", "number"
	case "INT", "INTEGER", "BIGINT", "SMALLINT", "TINYINT", "BYTEINT":
		return "int64", "integer"
	case "VARCHAR", "STRING", "TEXT", "CHAR", "CHARACTER":
		return "string", "string"
	case "BOOLEAN":
		return "bool", "boolean"
	case "DATE":
		return "time.Time", "string(date)"
	case "DATETIME", "TIMESTAMP", "TIMESTAMP_LTZ", "TIMESTAMP_NTZ", "TIMESTAMP_TZ":
		return "time.Time", "string(date-time)"
	case "TIME":
		return "string", "string(time)"
	case "BINARY", "VARBINARY":
		return "[]byte", "string(byte)"
	case "VARIANT", "OBJECT", "ARRAY":
		return "interface{}", "object"
	case "GEOGRAPHY", "GEOMETRY":
		return "string", "string"
	default:
		return "interface{}", "string"
	}
}
