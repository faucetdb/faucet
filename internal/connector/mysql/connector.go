package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"

	"github.com/faucetdb/faucet/internal/connector"
)

// MySQLConnector implements connector.Connector for MySQL databases.
type MySQLConnector struct {
	db         *sqlx.DB
	schemaName string
}

// New creates a new MySQLConnector with default settings.
func New() connector.Connector {
	return &MySQLConnector{}
}

// Connect establishes a connection to the MySQL database using the provided
// configuration. It configures connection pool settings and stores the schema
// name for introspection queries.
func (c *MySQLConnector) Connect(cfg connector.ConnectionConfig) error {
	db, err := sqlx.Connect("mysql", cfg.DSN)
	if err != nil {
		return fmt.Errorf("mysql connect: %w", err)
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

	// If no schema name provided, query the current database name
	if c.schemaName == "" {
		var dbName string
		if err := db.Get(&dbName, "SELECT DATABASE()"); err == nil && dbName != "" {
			c.schemaName = dbName
		}
	}

	c.db = db
	return nil
}

// Disconnect closes the database connection pool.
func (c *MySQLConnector) Disconnect() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping verifies the database connection is alive.
func (c *MySQLConnector) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// DB returns the underlying sqlx.DB connection pool.
func (c *MySQLConnector) DB() *sqlx.DB {
	return c.db
}

// DriverName returns the driver identifier for MySQL.
func (c *MySQLConnector) DriverName() string { return "mysql" }

// QuoteIdentifier wraps a SQL identifier in backticks, escaping any
// embedded backticks to prevent SQL injection.
func (c *MySQLConnector) QuoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// SupportsReturning indicates that MySQL does NOT support RETURNING clauses.
func (c *MySQLConnector) SupportsReturning() bool { return false }

// SupportsUpsert indicates that MySQL supports ON DUPLICATE KEY UPDATE (upsert).
func (c *MySQLConnector) SupportsUpsert() bool { return true }

// ParameterPlaceholder returns a MySQL-style positional parameter
// placeholder (?). MySQL ignores the index.
func (c *MySQLConnector) ParameterPlaceholder(_ int) string {
	return "?"
}
