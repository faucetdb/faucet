package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/faucetdb/faucet/internal/connector"
)

// SQLiteConnector implements connector.Connector for SQLite databases.
type SQLiteConnector struct {
	db         *sqlx.DB
	schemaName string // always "main" for SQLite
}

// New creates a new SQLiteConnector with default settings.
func New() connector.Connector {
	return &SQLiteConnector{schemaName: "main"}
}

// Connect opens a connection to the SQLite database file specified in the DSN.
// The DSN should be a file path (e.g., "/path/to/db.sqlite") or ":memory:"
// for an in-memory database. Query parameters like ?_journal_mode=WAL are supported.
func (c *SQLiteConnector) Connect(cfg connector.ConnectionConfig) error {
	db, err := sqlx.Connect("sqlite", cfg.DSN)
	if err != nil {
		return fmt.Errorf("sqlite connect: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
	}

	if cfg.SchemaName != "" {
		c.schemaName = cfg.SchemaName
	}

	c.db = db
	return nil
}

// BeginTx starts a new database transaction with the given options.
func (c *SQLiteConnector) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sqlx.Tx, error) {
	return c.db.BeginTxx(ctx, opts)
}

// Disconnect closes the database connection.
func (c *SQLiteConnector) Disconnect() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping verifies the database connection is alive.
func (c *SQLiteConnector) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// DB returns the underlying sqlx.DB connection pool.
func (c *SQLiteConnector) DB() *sqlx.DB {
	return c.db
}

// DriverName returns the driver identifier for SQLite.
func (c *SQLiteConnector) DriverName() string { return "sqlite" }

// QuoteIdentifier wraps a SQL identifier in double quotes, escaping any
// embedded double quotes to prevent SQL injection.
func (c *SQLiteConnector) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// SupportsReturning indicates that SQLite supports RETURNING clauses (3.35+).
func (c *SQLiteConnector) SupportsReturning() bool { return true }

// SupportsUpsert indicates that SQLite supports ON CONFLICT (upsert).
func (c *SQLiteConnector) SupportsUpsert() bool { return true }

// ParameterPlaceholder returns a SQLite-style positional parameter
// placeholder (?). SQLite ignores the index.
func (c *SQLiteConnector) ParameterPlaceholder(_ int) string {
	return "?"
}
