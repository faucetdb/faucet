package oracle

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/model"
)

// columnRow holds the result of querying ALL_TAB_COLUMNS.
type columnRow struct {
	TableName  string  `db:"TABLE_NAME"`
	ColumnName string  `db:"COLUMN_NAME"`
	DataType   string  `db:"DATA_TYPE"`
	IsNullable string  `db:"NULLABLE"`
	Default    *string `db:"DATA_DEFAULT"`
	MaxLength  *int64  `db:"CHAR_LENGTH"`
	Position   int     `db:"COLUMN_ID"`
	DataScale  *int    `db:"DATA_SCALE"`
}

// tableRow holds the result of querying ALL_TABLES / ALL_VIEWS.
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
	ReferencedTable  string `db:"REFERENCED_TABLE"`
	ReferencedColumn string `db:"REFERENCED_COLUMN"`
	DeleteRule       string `db:"DELETE_RULE"`
}

// routineRow holds a stored procedure or function from ALL_PROCEDURES.
type routineRow struct {
	ObjectName string `db:"OBJECT_NAME"`
	ObjectType string `db:"OBJECT_TYPE"`
}

// IntrospectSchema returns the full schema for the configured Oracle schema,
// including all tables, views, procedures, and functions.
func (c *OracleConnector) IntrospectSchema(ctx context.Context) (*model.Schema, error) {
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
		})
	}

	// Build column lookup: table_name -> []Column
	colMap := make(map[string][]model.Column)
	for _, col := range columns {
		isPK := pkMap[col.TableName] != nil && pkMap[col.TableName][col.ColumnName]

		goType, jsonType := mapOracleType(col.DataType, col.DataScale)

		colMap[col.TableName] = append(colMap[col.TableName], model.Column{
			Name:         col.ColumnName,
			Position:     col.Position,
			Type:         col.DataType,
			GoType:       goType,
			JsonType:     jsonType,
			Nullable:     col.IsNullable == "Y",
			Default:      col.Default,
			MaxLength:    col.MaxLength,
			IsPrimaryKey: isPK,
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
			Name: r.ObjectName,
		}
		switch strings.ToUpper(r.ObjectType) {
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
func (c *OracleConnector) IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error) {
	upperName := strings.ToUpper(tableName)

	// Verify the table exists and get its type
	const tableQuery = `SELECT TABLE_NAME, 'TABLE' AS TABLE_TYPE FROM ALL_TABLES
		WHERE OWNER = :1 AND TABLE_NAME = :2
		UNION ALL
		SELECT VIEW_NAME AS TABLE_NAME, 'VIEW' AS TABLE_TYPE FROM ALL_VIEWS
		WHERE OWNER = :3 AND VIEW_NAME = :4`

	var t tableRow
	if err := c.db.GetContext(ctx, &t, tableQuery, c.schemaName, upperName, c.schemaName, upperName); err != nil {
		return nil, fmt.Errorf("table %q not found in schema %q: %w", tableName, c.schemaName, err)
	}

	// Fetch columns for this specific table
	columns, err := c.fetchColumns(ctx, upperName)
	if err != nil {
		return nil, fmt.Errorf("introspect columns for %q: %w", tableName, err)
	}

	// Fetch primary keys for this table
	const pkQuery = `SELECT acc.TABLE_NAME, acc.COLUMN_NAME
		FROM ALL_CONSTRAINTS ac
		JOIN ALL_CONS_COLUMNS acc ON ac.CONSTRAINT_NAME = acc.CONSTRAINT_NAME
			AND ac.OWNER = acc.OWNER
		WHERE ac.CONSTRAINT_TYPE = 'P'
			AND ac.OWNER = :1
			AND ac.TABLE_NAME = :2
		ORDER BY acc.POSITION`

	var pks []pkRow
	if err := c.db.SelectContext(ctx, &pks, pkQuery, c.schemaName, upperName); err != nil {
		return nil, fmt.Errorf("introspect primary keys for %q: %w", tableName, err)
	}

	pkSet := make(map[string]bool, len(pks))
	pkCols := make([]string, 0, len(pks))
	for _, pk := range pks {
		pkSet[pk.ColumnName] = true
		pkCols = append(pkCols, pk.ColumnName)
	}

	// Fetch foreign keys for this table
	const fkQuery = `SELECT ac.TABLE_NAME,
			acc.COLUMN_NAME,
			rac.TABLE_NAME AS REFERENCED_TABLE,
			racc.COLUMN_NAME AS REFERENCED_COLUMN,
			NVL(ac.DELETE_RULE, 'NO ACTION') AS DELETE_RULE
		FROM ALL_CONSTRAINTS ac
		JOIN ALL_CONS_COLUMNS acc ON ac.CONSTRAINT_NAME = acc.CONSTRAINT_NAME
			AND ac.OWNER = acc.OWNER
		JOIN ALL_CONSTRAINTS rac ON ac.R_CONSTRAINT_NAME = rac.CONSTRAINT_NAME
			AND ac.R_OWNER = rac.OWNER
		JOIN ALL_CONS_COLUMNS racc ON rac.CONSTRAINT_NAME = racc.CONSTRAINT_NAME
			AND rac.OWNER = racc.OWNER
			AND acc.POSITION = racc.POSITION
		WHERE ac.CONSTRAINT_TYPE = 'R'
			AND ac.OWNER = :1
			AND ac.TABLE_NAME = :2`

	var fks []fkRow
	if err := c.db.SelectContext(ctx, &fks, fkQuery, c.schemaName, upperName); err != nil {
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
		})
	}

	// Build columns with pk info
	modelColumns := make([]model.Column, 0, len(columns))
	for _, col := range columns {
		isPK := pkSet[col.ColumnName]
		goType, jsonType := mapOracleType(col.DataType, col.DataScale)

		modelColumns = append(modelColumns, model.Column{
			Name:         col.ColumnName,
			Position:     col.Position,
			Type:         col.DataType,
			GoType:       goType,
			JsonType:     jsonType,
			Nullable:     col.IsNullable == "Y",
			Default:      col.Default,
			MaxLength:    col.MaxLength,
			IsPrimaryKey: isPK,
		})
	}

	tableType := "table"
	if t.TableType == "VIEW" {
		tableType = "view"
	}

	return &model.TableSchema{
		Name:        t.TableName,
		Type:        tableType,
		Columns:     modelColumns,
		PrimaryKey:  pkCols,
		ForeignKeys: foreignKeys,
		Indexes:     []model.Index{},
	}, nil
}

// GetTableNames returns a list of all table names in the configured schema.
func (c *OracleConnector) GetTableNames(ctx context.Context) ([]string, error) {
	const query = `SELECT TABLE_NAME FROM ALL_TABLES
		WHERE OWNER = :1
		ORDER BY TABLE_NAME`

	var names []string
	if err := c.db.SelectContext(ctx, &names, query, c.schemaName); err != nil {
		return nil, fmt.Errorf("get table names: %w", err)
	}
	return names, nil
}

// GetStoredProcedures returns all stored procedures and functions in the
// configured schema.
func (c *OracleConnector) GetStoredProcedures(ctx context.Context) ([]model.StoredProcedure, error) {
	routines, err := c.fetchRoutines(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]model.StoredProcedure, 0, len(routines))
	for _, r := range routines {
		sp := model.StoredProcedure{
			Name: r.ObjectName,
		}
		switch strings.ToUpper(r.ObjectType) {
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

func (c *OracleConnector) fetchTables(ctx context.Context) ([]tableRow, error) {
	const query = `SELECT TABLE_NAME, 'TABLE' AS TABLE_TYPE FROM ALL_TABLES WHERE OWNER = :1
		UNION ALL
		SELECT VIEW_NAME AS TABLE_NAME, 'VIEW' AS TABLE_TYPE FROM ALL_VIEWS WHERE OWNER = :1
		ORDER BY TABLE_NAME`

	var rows []tableRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *OracleConnector) fetchColumns(ctx context.Context, tableName string) ([]columnRow, error) {
	query := `SELECT
			TABLE_NAME,
			COLUMN_NAME,
			DATA_TYPE,
			NULLABLE,
			DATA_DEFAULT,
			CHAR_LENGTH,
			COLUMN_ID,
			DATA_SCALE
		FROM ALL_TAB_COLUMNS
		WHERE OWNER = :1`

	args := []interface{}{c.schemaName}

	if tableName != "" {
		query += ` AND TABLE_NAME = :2`
		args = append(args, tableName)
	}

	query += ` ORDER BY TABLE_NAME, COLUMN_ID`

	var rows []columnRow
	if err := c.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *OracleConnector) fetchPrimaryKeys(ctx context.Context) ([]pkRow, error) {
	const query = `SELECT acc.TABLE_NAME, acc.COLUMN_NAME
		FROM ALL_CONSTRAINTS ac
		JOIN ALL_CONS_COLUMNS acc ON ac.CONSTRAINT_NAME = acc.CONSTRAINT_NAME
			AND ac.OWNER = acc.OWNER
		WHERE ac.CONSTRAINT_TYPE = 'P'
			AND ac.OWNER = :1
		ORDER BY acc.TABLE_NAME, acc.POSITION`

	var rows []pkRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *OracleConnector) fetchForeignKeys(ctx context.Context) ([]fkRow, error) {
	const query = `SELECT ac.TABLE_NAME,
			acc.COLUMN_NAME,
			rac.TABLE_NAME AS REFERENCED_TABLE,
			racc.COLUMN_NAME AS REFERENCED_COLUMN,
			NVL(ac.DELETE_RULE, 'NO ACTION') AS DELETE_RULE
		FROM ALL_CONSTRAINTS ac
		JOIN ALL_CONS_COLUMNS acc ON ac.CONSTRAINT_NAME = acc.CONSTRAINT_NAME
			AND ac.OWNER = acc.OWNER
		JOIN ALL_CONSTRAINTS rac ON ac.R_CONSTRAINT_NAME = rac.CONSTRAINT_NAME
			AND ac.R_OWNER = rac.OWNER
		JOIN ALL_CONS_COLUMNS racc ON rac.CONSTRAINT_NAME = racc.CONSTRAINT_NAME
			AND rac.OWNER = racc.OWNER
			AND acc.POSITION = racc.POSITION
		WHERE ac.CONSTRAINT_TYPE = 'R'
			AND ac.OWNER = :1`

	var rows []fkRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *OracleConnector) fetchRoutines(ctx context.Context) ([]routineRow, error) {
	const query = `SELECT OBJECT_NAME, OBJECT_TYPE
		FROM ALL_PROCEDURES
		WHERE OWNER = :1
			AND OBJECT_TYPE IN ('PROCEDURE', 'FUNCTION')
			AND PROCEDURE_NAME IS NULL
		ORDER BY OBJECT_NAME`

	var rows []routineRow
	if err := c.db.SelectContext(ctx, &rows, query, c.schemaName); err != nil {
		return nil, err
	}
	return rows, nil
}

// mapOracleType maps an Oracle data type to a Go type string and a JSON Schema type string.
func mapOracleType(dataType string, dataScale *int) (goType, jsonType string) {
	upper := strings.ToUpper(strings.TrimSpace(dataType))
	switch {
	case upper == "NUMBER" || upper == "NUMERIC" || upper == "DECIMAL":
		// If scale is 0 or nil, treat as integer
		if dataScale == nil || *dataScale == 0 {
			return "int64", "integer"
		}
		return "float64", "number"
	case upper == "BINARY_FLOAT" || upper == "FLOAT":
		return "float32", "number"
	case upper == "BINARY_DOUBLE":
		return "float64", "number"
	case upper == "INTEGER" || upper == "INT" || upper == "SMALLINT":
		return "int64", "integer"
	case upper == "VARCHAR2" || upper == "NVARCHAR2" || upper == "CHAR" || upper == "NCHAR":
		return "string", "string"
	case upper == "CLOB" || upper == "NCLOB" || upper == "LONG":
		return "string", "string"
	case upper == "BLOB" || upper == "RAW" || upper == "LONG RAW":
		return "[]byte", "string(byte)"
	case upper == "DATE":
		return "time.Time", "string(date-time)"
	case strings.HasPrefix(upper, "TIMESTAMP"):
		return "time.Time", "string(date-time)"
	case strings.HasPrefix(upper, "INTERVAL"):
		return "string", "string"
	case upper == "XMLTYPE":
		return "string", "string"
	case upper == "JSON":
		return "interface{}", "object"
	case upper == "BOOLEAN":
		return "bool", "boolean"
	case upper == "ROWID" || upper == "UROWID":
		return "string", "string"
	case upper == "BFILE":
		return "string", "string"
	default:
		return "interface{}", "string"
	}
}
