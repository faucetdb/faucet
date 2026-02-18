package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/faucetdb/faucet/internal/model"
)

// tableInfoRow holds a row from PRAGMA table_info().
type tableInfoRow struct {
	CID       int     `db:"cid"`
	Name      string  `db:"name"`
	Type      string  `db:"type"`
	NotNull   int     `db:"notnull"`
	Default   *string `db:"dflt_value"`
	PK        int     `db:"pk"`
}

// foreignKeyRow holds a row from PRAGMA foreign_key_list().
type foreignKeyRow struct {
	ID        int    `db:"id"`
	Seq       int    `db:"seq"`
	Table     string `db:"table"`
	From      string `db:"from"`
	To        string `db:"to"`
	OnUpdate  string `db:"on_update"`
	OnDelete  string `db:"on_delete"`
	Match     string `db:"match"`
}

// indexListRow holds a row from PRAGMA index_list().
type indexListRow struct {
	Seq     int    `db:"seq"`
	Name    string `db:"name"`
	Unique  int    `db:"unique"`
	Origin  string `db:"origin"`
	Partial int    `db:"partial"`
}

// indexInfoRow holds a row from PRAGMA index_info().
type indexInfoRow struct {
	SeqNo int     `db:"seqno"`
	CID   int     `db:"cid"`
	Name  *string `db:"name"`
}

// IntrospectSchema returns the full schema for the SQLite database,
// including all tables and views.
func (c *SQLiteConnector) IntrospectSchema(ctx context.Context) (*model.Schema, error) {
	// Fetch table and view names
	const query = `SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name`

	type masterRow struct {
		Name string `db:"name"`
		Type string `db:"type"`
	}

	var rows []masterRow
	if err := c.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("introspect schema: %w", err)
	}

	schema := &model.Schema{
		Tables:     []model.TableSchema{},
		Views:      []model.TableSchema{},
		Procedures: []model.StoredProcedure{},
		Functions:  []model.StoredProcedure{},
	}

	for _, row := range rows {
		ts, err := c.IntrospectTable(ctx, row.Name)
		if err != nil {
			return nil, fmt.Errorf("introspect table %q: %w", row.Name, err)
		}

		switch row.Type {
		case "view":
			ts.Type = "view"
			schema.Views = append(schema.Views, *ts)
		default:
			ts.Type = "table"
			schema.Tables = append(schema.Tables, *ts)
		}
	}

	return schema, nil
}

// IntrospectTable returns the schema for a single table or view.
func (c *SQLiteConnector) IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error) {
	// Fetch column info via PRAGMA
	pragmaQuery := fmt.Sprintf("PRAGMA table_info(%s)", c.QuoteIdentifier(tableName))
	var columns []tableInfoRow
	if err := c.db.SelectContext(ctx, &columns, pragmaQuery); err != nil {
		return nil, fmt.Errorf("table_info for %q: %w", tableName, err)
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("table %q not found", tableName)
	}

	// Build primary key list and check for autoincrement
	pkCols := []string{}
	for _, col := range columns {
		if col.PK > 0 {
			pkCols = append(pkCols, col.Name)
		}
	}

	// Check if the table uses AUTOINCREMENT by inspecting sqlite_master SQL
	autoIncrCols := c.detectAutoIncrement(ctx, tableName, pkCols)

	// Build model columns
	modelColumns := make([]model.Column, 0, len(columns))
	for _, col := range columns {
		goType, jsonType := mapSQLiteType(col.Type)
		isPK := col.PK > 0
		isAuto := autoIncrCols[col.Name]

		modelColumns = append(modelColumns, model.Column{
			Name:            col.Name,
			Position:        col.CID + 1,
			Type:            col.Type,
			GoType:          goType,
			JsonType:        jsonType,
			Nullable:        col.NotNull == 0 && !isPK,
			Default:         col.Default,
			IsPrimaryKey:    isPK,
			IsAutoIncrement: isAuto,
		})
	}

	// Fetch foreign keys
	fkQuery := fmt.Sprintf("PRAGMA foreign_key_list(%s)", c.QuoteIdentifier(tableName))
	var fkRows []foreignKeyRow
	if err := c.db.SelectContext(ctx, &fkRows, fkQuery); err != nil {
		return nil, fmt.Errorf("foreign_key_list for %q: %w", tableName, err)
	}

	foreignKeys := make([]model.ForeignKey, 0, len(fkRows))
	for _, fk := range fkRows {
		foreignKeys = append(foreignKeys, model.ForeignKey{
			Name:             fmt.Sprintf("fk_%s_%s", tableName, fk.From),
			ColumnName:       fk.From,
			ReferencedTable:  fk.Table,
			ReferencedColumn: fk.To,
			OnDelete:         fk.OnDelete,
			OnUpdate:         fk.OnUpdate,
		})
	}

	// Fetch indexes
	idxQuery := fmt.Sprintf("PRAGMA index_list(%s)", c.QuoteIdentifier(tableName))
	var idxRows []indexListRow
	if err := c.db.SelectContext(ctx, &idxRows, idxQuery); err != nil {
		return nil, fmt.Errorf("index_list for %q: %w", tableName, err)
	}

	indexes := make([]model.Index, 0, len(idxRows))
	for _, idx := range idxRows {
		// Skip auto-generated indexes for primary keys
		if idx.Origin == "pk" {
			continue
		}

		infoQuery := fmt.Sprintf("PRAGMA index_info(%s)", c.QuoteIdentifier(idx.Name))
		var infoRows []indexInfoRow
		if err := c.db.SelectContext(ctx, &infoRows, infoQuery); err != nil {
			continue
		}

		idxCols := make([]string, 0, len(infoRows))
		for _, info := range infoRows {
			if info.Name != nil {
				idxCols = append(idxCols, *info.Name)
			}
		}

		indexes = append(indexes, model.Index{
			Name:     idx.Name,
			Columns:  idxCols,
			IsUnique: idx.Unique == 1,
		})
	}

	// Determine table type
	tableType := "table"
	var objType string
	typeQuery := `SELECT type FROM sqlite_master WHERE name = ?`
	if err := c.db.GetContext(ctx, &objType, typeQuery, tableName); err == nil {
		if objType == "view" {
			tableType = "view"
		}
	}

	return &model.TableSchema{
		Name:        tableName,
		Type:        tableType,
		Columns:     modelColumns,
		PrimaryKey:  pkCols,
		ForeignKeys: foreignKeys,
		Indexes:     indexes,
	}, nil
}

// GetTableNames returns a list of all table names in the database.
func (c *SQLiteConnector) GetTableNames(ctx context.Context) ([]string, error) {
	const query = `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name`

	var names []string
	if err := c.db.SelectContext(ctx, &names, query); err != nil {
		return nil, fmt.Errorf("get table names: %w", err)
	}
	return names, nil
}

// GetStoredProcedures returns an empty list since SQLite does not support
// stored procedures.
func (c *SQLiteConnector) GetStoredProcedures(_ context.Context) ([]model.StoredProcedure, error) {
	return []model.StoredProcedure{}, nil
}

// detectAutoIncrement checks if the primary key columns use AUTOINCREMENT
// by inspecting the CREATE TABLE SQL in sqlite_master.
func (c *SQLiteConnector) detectAutoIncrement(ctx context.Context, tableName string, pkCols []string) map[string]bool {
	result := make(map[string]bool)

	if len(pkCols) != 1 {
		return result
	}

	var createSQL string
	query := `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`
	if err := c.db.GetContext(ctx, &createSQL, query, tableName); err != nil {
		return result
	}

	upper := strings.ToUpper(createSQL)
	// INTEGER PRIMARY KEY is implicitly an alias for rowid (auto-increment behavior)
	if strings.Contains(upper, "INTEGER PRIMARY KEY") {
		result[pkCols[0]] = true
	}

	return result
}

// mapSQLiteType maps a SQLite type affinity to Go and JSON Schema types.
// SQLite uses type affinity rather than strict types.
func mapSQLiteType(typeName string) (goType, jsonType string) {
	upper := strings.ToUpper(strings.TrimSpace(typeName))

	// Strip parenthesized length/precision (e.g., VARCHAR(255) -> VARCHAR)
	if idx := strings.IndexByte(upper, '('); idx >= 0 {
		upper = strings.TrimSpace(upper[:idx])
	}

	// SQLite type affinity rules (https://sqlite.org/datatype3.html)
	switch {
	case strings.Contains(upper, "INT"):
		return "int64", "integer"
	case strings.Contains(upper, "CHAR"),
		strings.Contains(upper, "CLOB"),
		strings.Contains(upper, "TEXT"):
		return "string", "string"
	case strings.Contains(upper, "BLOB") || upper == "":
		return "[]byte", "string(byte)"
	case strings.Contains(upper, "REAL"),
		strings.Contains(upper, "FLOA"),
		strings.Contains(upper, "DOUB"):
		return "float64", "number"
	case strings.Contains(upper, "BOOL"):
		return "bool", "boolean"
	case strings.Contains(upper, "DATE"),
		strings.Contains(upper, "TIME"):
		return "time.Time", "string(date-time)"
	case strings.Contains(upper, "NUMERIC"),
		strings.Contains(upper, "DECIMAL"):
		return "float64", "number"
	case strings.Contains(upper, "JSON"):
		return "interface{}", "object"
	default:
		// Default to NUMERIC affinity per SQLite rules
		return "interface{}", "string"
	}
}
