package mssql

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/microsoft/go-mssqldb"

	"github.com/faucetdb/faucet/internal/connector"
)

// MSSQLConnector implements connector.Connector for SQL Server databases.
type MSSQLConnector struct {
	db         *sqlx.DB
	schemaName string
}

// New creates a new MSSQLConnector with default settings.
func New() connector.Connector {
	return &MSSQLConnector{schemaName: "dbo"}
}

// Connect establishes a connection to the SQL Server database using the
// provided configuration. It configures connection pool settings and stores
// the schema name for introspection queries.
func (c *MSSQLConnector) Connect(cfg connector.ConnectionConfig) error {
	db, err := sqlx.Connect("sqlserver", cfg.DSN)
	if err != nil {
		return fmt.Errorf("mssql connect: %w", err)
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

// Disconnect closes the database connection pool.
func (c *MSSQLConnector) Disconnect() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping verifies the database connection is alive.
func (c *MSSQLConnector) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// DB returns the underlying sqlx.DB connection pool.
func (c *MSSQLConnector) DB() *sqlx.DB {
	return c.db
}

// DriverName returns the driver identifier for SQL Server.
func (c *MSSQLConnector) DriverName() string { return "mssql" }

// QuoteIdentifier wraps a SQL identifier in brackets, escaping any
// embedded closing brackets to prevent SQL injection.
func (c *MSSQLConnector) QuoteIdentifier(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// SupportsReturning indicates that SQL Server does NOT support the RETURNING
// clause. Use OUTPUT INSERTED.* instead.
func (c *MSSQLConnector) SupportsReturning() bool { return false }

// SupportsUpsert indicates that SQL Server supports upsert via MERGE.
func (c *MSSQLConnector) SupportsUpsert() bool { return true }

// ParameterPlaceholder returns a SQL Server-style numbered parameter
// placeholder (e.g., @p1, @p2, @p3).
func (c *MSSQLConnector) ParameterPlaceholder(index int) string {
	return fmt.Sprintf("@p%d", index)
}
