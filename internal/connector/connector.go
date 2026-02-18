package connector

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
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
// MySQL DSNs are normalized to use the tcp() wrapper required by go-sql-driver.
// Snowflake uses its own non-URL DSN format and is returned unchanged.
func SanitizeDSN(driver, dsn string) string {
	switch driver {
	case "postgres", "mssql":
		return sanitizeURLDSN(dsn)
	case "mysql":
		return sanitizeMySQLDSN(dsn)
	default:
		return dsn
	}
}

// mysqlBareHostPort matches "user:pass@host:port/db" (no tcp() wrapper, no ()
// wrapper). We look for the last "@" followed by what looks like host:port/db.
var mysqlBareHostPort = regexp.MustCompile(`^(.+)@([^(@]+:\d+)(/.*)?$`)

// sanitizeMySQLDSN normalizes a MySQL DSN so that go-sql-driver/mysql can
// parse it correctly. The driver requires the format:
//
//	user:pass@tcp(host:port)/dbname
//
// Common mistakes from users:
//
//	user:pass@host:port/db          → missing tcp() wrapper
//	user:pass@(host:port)/db        → missing "tcp" before parens
//	user:pass@tcp(host:port)/db     → already correct
//
// When the password contains "@", the driver's ParseDSN splits on the last
// "@" before "/" — this works ONLY when "tcp(" is present, otherwise the
// parser treats the password fragment as a network name.
func sanitizeMySQLDSN(dsn string) string {
	// If it already parses cleanly and has a known network, trust it.
	if cfg, err := mysqldriver.ParseDSN(dsn); err == nil && (cfg.Net == "tcp" || cfg.Net == "unix") {
		return cfg.FormatDSN()
	}

	// Try to fix common patterns.

	// Pattern: user:pass@(host:port)/db — missing "tcp" keyword.
	// Find the last "@" followed immediately by "(" but NOT preceded by
	// a network name like "tcp" or "unix".
	if idx := strings.LastIndex(dsn, "@("); idx >= 0 {
		// Insert "tcp" between "@" and "("
		fixed := dsn[:idx] + "@tcp" + dsn[idx+1:]
		if cfg, err := mysqldriver.ParseDSN(fixed); err == nil {
			return cfg.FormatDSN()
		}
	}

	// Pattern: user:pass@host:port/db — no parens at all.
	if m := mysqlBareHostPort.FindStringSubmatch(dsn); m != nil {
		userpass := m[1] // everything before the last @host:port
		hostport := m[2]
		dbpart := m[3] // /dbname or empty
		fixed := userpass + "@tcp(" + hostport + ")" + dbpart
		if cfg, err := mysqldriver.ParseDSN(fixed); err == nil {
			return cfg.FormatDSN()
		}
	}

	// Nothing worked — return as-is and let the connect call give a clear error.
	return dsn
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
