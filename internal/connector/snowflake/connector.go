package snowflake

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/snowflakedb/gosnowflake"

	"github.com/faucetdb/faucet/internal/connector"
)

// SnowflakeConnector implements connector.Connector for Snowflake databases.
type SnowflakeConnector struct {
	db         *sqlx.DB
	schemaName string
}

// New creates a new SnowflakeConnector with default settings.
func New() connector.Connector {
	return &SnowflakeConnector{schemaName: "PUBLIC"}
}

// Connect establishes a connection to the Snowflake database using the
// provided configuration. It configures connection pool settings and stores
// the schema name for introspection queries.
func (c *SnowflakeConnector) Connect(cfg connector.ConnectionConfig) error {
	db, err := sqlx.Connect("snowflake", cfg.DSN)
	if err != nil {
		return fmt.Errorf("snowflake connect: %w", err)
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
func (c *SnowflakeConnector) Disconnect() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping verifies the database connection is alive.
func (c *SnowflakeConnector) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// DB returns the underlying sqlx.DB connection pool.
func (c *SnowflakeConnector) DB() *sqlx.DB {
	return c.db
}

// DriverName returns the driver identifier for Snowflake.
func (c *SnowflakeConnector) DriverName() string { return "snowflake" }

// QuoteIdentifier wraps a SQL identifier in double quotes for Snowflake.
// Snowflake identifiers are case-sensitive when quoted.
func (c *SnowflakeConnector) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// SupportsReturning indicates that Snowflake does NOT support RETURNING clauses.
func (c *SnowflakeConnector) SupportsReturning() bool { return false }

// SupportsUpsert indicates that Snowflake does NOT support standard upsert.
// Snowflake has MERGE but it requires a staging table pattern.
func (c *SnowflakeConnector) SupportsUpsert() bool { return false }

// ParameterPlaceholder returns a Snowflake-style positional parameter
// placeholder (?). Snowflake ignores the index.
func (c *SnowflakeConnector) ParameterPlaceholder(_ int) string {
	return "?"
}
