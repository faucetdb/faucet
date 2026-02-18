package connector

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/faucetdb/faucet/internal/model"
)

// SelectRequest represents a query for records.
type SelectRequest struct {
	Table      string
	Fields     []string
	Filter     string
	FilterArgs []interface{}
	Order      string
	Limit      int
	Offset     int
	Cursor     string
}

// InsertRequest represents an insert operation.
type InsertRequest struct {
	Table   string
	Records []map[string]interface{}
}

// UpdateRequest represents an update operation.
type UpdateRequest struct {
	Table      string
	Filter     string
	FilterArgs []interface{}
	Record     map[string]interface{}
	IDs        []interface{} // for updating by primary key
}

// DeleteRequest represents a delete operation.
type DeleteRequest struct {
	Table      string
	Filter     string
	FilterArgs []interface{}
	IDs        []interface{}
}

// CountRequest represents a count query.
type CountRequest struct {
	Table      string
	Filter     string
	FilterArgs []interface{}
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
	PrivateKeyPath  string // Path to PEM-encoded private key file (Snowflake JWT auth)
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

// SanitizeDSN ensures that URL-style DSNs (postgres://, sqlserver://) have
// their userinfo (especially the password) properly percent-encoded. Raw
// passwords containing @, #, %, or other URL-special characters cause the
// Go URL parser to mis-split the authority component, leading to connection
// failures that surface as "Service not found" because the connector never
// registers in the live registry.
//
// MySQL and Snowflake use non-URL DSN formats and are returned unchanged.
func SanitizeDSN(driver, dsn string) string {
	switch driver {
	case "postgres", "mssql":
		return sanitizeURLDSN(dsn)
	default:
		return dsn
	}
}

// sanitizeURLDSN parses a DSN that begins with a scheme (e.g.
// postgres://user:p@ss#word@host/db) and re-encodes the password so the
// URL library can parse it unambiguously.
func sanitizeURLDSN(dsn string) string {
	// Find the scheme separator.
	schemeEnd := strings.Index(dsn, "://")
	if schemeEnd < 0 {
		return dsn // not a URL-style DSN, return as-is
	}

	scheme := dsn[:schemeEnd]
	rest := dsn[schemeEnd+3:] // everything after "://"

	// Split off query/fragment from the authority+path portion.
	query := ""
	if qi := strings.IndexByte(rest, '?'); qi >= 0 {
		query = rest[qi:]
		rest = rest[:qi]
	}

	// Find the LAST '@' — everything before it is userinfo, everything after is host+path.
	atIdx := strings.LastIndex(rest, "@")
	if atIdx < 0 {
		return dsn // no credentials in the DSN
	}

	userinfo := rest[:atIdx]
	hostpath := rest[atIdx+1:]

	// Split userinfo into user and password at the FIRST ':'.
	user := userinfo
	pass := ""
	if ci := strings.IndexByte(userinfo, ':'); ci >= 0 {
		user = userinfo[:ci]
		pass = userinfo[ci+1:]
	}

	// Re-encode. url.PathEscape is too aggressive; url.QueryEscape encodes
	// spaces as '+' which isn't great for passwords. Use a manual approach:
	// percent-encode only the characters that break URL parsing.
	encodedUser := url.PathEscape(user)
	encodedPass := url.PathEscape(pass)

	return scheme + "://" + encodedUser + ":" + encodedPass + "@" + hostpath + query
}

// SchemaChange represents a table alteration.
type SchemaChange struct {
	Type       string        // "add_column", "drop_column", "rename_column", "modify_column"
	Column     string
	NewName    string        // for rename
	Definition *model.Column // for add/modify
}
