package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/sijms/go-ora/v2"

	"github.com/faucetdb/faucet/internal/connector"
)

// OracleConnector implements connector.Connector for Oracle databases.
type OracleConnector struct {
	db         *sqlx.DB
	schemaName string
}

// New creates a new OracleConnector with default settings.
func New() connector.Connector {
	return &OracleConnector{}
}

// Connect establishes a connection to the Oracle database using the provided
// configuration. It configures connection pool settings and stores the schema
// name for introspection queries.
func (c *OracleConnector) Connect(cfg connector.ConnectionConfig) error {
	db, err := sqlx.Connect("oracle", cfg.DSN)
	if err != nil {
		return fmt.Errorf("oracle connect: %w", err)
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
		c.schemaName = strings.ToUpper(cfg.SchemaName)
	}

	// If no schema name provided, query the current user (Oracle's default schema)
	if c.schemaName == "" {
		var user string
		if err := db.Get(&user, "SELECT USER FROM DUAL"); err == nil && user != "" {
			c.schemaName = strings.ToUpper(user)
		}
	}

	c.db = db
	return nil
}

// BeginTx starts a new database transaction with the given options.
func (c *OracleConnector) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sqlx.Tx, error) {
	return c.db.BeginTxx(ctx, opts)
}

// Disconnect closes the database connection pool.
func (c *OracleConnector) Disconnect() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping verifies the database connection is alive.
func (c *OracleConnector) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// DB returns the underlying sqlx.DB connection pool.
func (c *OracleConnector) DB() *sqlx.DB {
	return c.db
}

// DriverName returns the driver identifier for Oracle.
func (c *OracleConnector) DriverName() string { return "oracle" }

// QuoteIdentifier wraps a SQL identifier in double quotes, escaping any
// embedded double quotes to prevent SQL injection.
func (c *OracleConnector) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// SupportsReturning indicates that Oracle does NOT support PostgreSQL-style
// RETURNING clauses. Oracle has RETURNING INTO which requires bound OUT
// variables, incompatible with the standard query-rows pattern.
func (c *OracleConnector) SupportsReturning() bool { return false }

// SupportsUpsert indicates that Oracle does NOT support simple upsert syntax.
// Oracle uses MERGE statements which have a substantially different structure.
func (c *OracleConnector) SupportsUpsert() bool { return false }

// ParameterPlaceholder returns an Oracle-style numbered parameter
// placeholder (e.g., :1, :2, :3).
func (c *OracleConnector) ParameterPlaceholder(index int) string {
	return fmt.Sprintf(":%d", index)
}
