package connector

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/faucetdb/faucet/internal/model"
)

// SelectRequest represents a query for records.
type SelectRequest struct {
	Table  string
	Fields []string
	Filter string
	Order  string
	Limit  int
	Offset int
	Cursor string
}

// InsertRequest represents an insert operation.
type InsertRequest struct {
	Table   string
	Records []map[string]interface{}
}

// UpdateRequest represents an update operation.
type UpdateRequest struct {
	Table  string
	Filter string
	Record map[string]interface{}
	IDs    []interface{} // for updating by primary key
}

// DeleteRequest represents a delete operation.
type DeleteRequest struct {
	Table  string
	Filter string
	IDs    []interface{}
}

// CountRequest represents a count query.
type CountRequest struct {
	Table  string
	Filter string
}

// ConnectionConfig holds database connection parameters.
type ConnectionConfig struct {
	Driver          string
	DSN             string
	SchemaName      string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Connector is the interface that all database connectors must implement.
type Connector interface {
	// Connection management
	Connect(cfg ConnectionConfig) error
	Disconnect() error
	Ping(ctx context.Context) error
	DB() *sqlx.DB

	// Schema introspection
	IntrospectSchema(ctx context.Context) (*model.Schema, error)
	IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error)
	GetTableNames(ctx context.Context) ([]string, error)
	GetStoredProcedures(ctx context.Context) ([]model.StoredProcedure, error)

	// Query building (database-specific SQL dialect)
	BuildSelect(ctx context.Context, req SelectRequest) (string, []interface{}, error)
	BuildInsert(ctx context.Context, req InsertRequest) (string, []interface{}, error)
	BuildUpdate(ctx context.Context, req UpdateRequest) (string, []interface{}, error)
	BuildDelete(ctx context.Context, req DeleteRequest) (string, []interface{}, error)
	BuildCount(ctx context.Context, req CountRequest) (string, []interface{}, error)

	// Schema modification
	CreateTable(ctx context.Context, def model.TableSchema) error
	AlterTable(ctx context.Context, tableName string, changes []SchemaChange) error
	DropTable(ctx context.Context, tableName string) error

	// Stored procedures
	CallProcedure(ctx context.Context, name string, params map[string]interface{}) ([]map[string]interface{}, error)

	// Metadata
	DriverName() string
	QuoteIdentifier(name string) string
	SupportsReturning() bool
	SupportsUpsert() bool
	ParameterPlaceholder(index int) string
}

// SchemaChange represents a table alteration.
type SchemaChange struct {
	Type       string        // "add_column", "drop_column", "rename_column", "modify_column"
	Column     string
	NewName    string        // for rename
	Definition *model.Column // for add/modify
}
