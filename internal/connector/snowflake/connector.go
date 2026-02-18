package snowflake

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	gosnowflake "github.com/snowflakedb/gosnowflake"

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
//
// If PrivateKeyPath is set, the connector uses JWT (key pair) authentication
// instead of username/password. The private key file must be PEM-encoded
// (PKCS#1 or PKCS#8 format).
func (c *SnowflakeConnector) Connect(cfg connector.ConnectionConfig) error {
	dsn := cfg.DSN

	if cfg.PrivateKeyPath != "" {
		var err error
		dsn, err = buildJWTDSN(cfg.DSN, cfg.PrivateKeyPath)
		if err != nil {
			return fmt.Errorf("snowflake jwt auth: %w", err)
		}
	}

	db, err := sqlx.Connect("snowflake", dsn)
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

// buildJWTDSN parses the given DSN, loads the private key from keyPath,
// sets JWT authenticator fields, and re-serializes the DSN.
func buildJWTDSN(dsn, keyPath string) (string, error) {
	// gosnowflake.ParseDSN requires a password even for JWT auth.
	// If the DSN has no password (user@account/db format), inject a
	// placeholder so parsing succeeds — JWT auth ignores it.
	sfConfig, err := gosnowflake.ParseDSN(dsn)
	if err != nil && strings.Contains(err.Error(), "password is empty") {
		if idx := strings.Index(dsn, "@"); idx > 0 && !strings.Contains(dsn[:idx], ":") {
			dsn = dsn[:idx] + ":_" + dsn[idx:]
		}
		sfConfig, err = gosnowflake.ParseDSN(dsn)
	}
	if err != nil {
		return "", fmt.Errorf("parse DSN: %w", err)
	}
	sfConfig.Password = ""

	privKey, err := loadPrivateKey(keyPath)
	if err != nil {
		return "", err
	}

	sfConfig.Authenticator = gosnowflake.AuthTypeJwt
	sfConfig.PrivateKey = privKey

	newDSN, err := gosnowflake.DSN(sfConfig)
	if err != nil {
		return "", fmt.Errorf("rebuild DSN: %w", err)
	}
	return newDSN, nil
}

// loadPrivateKey reads a PEM-encoded private key file and returns an
// *rsa.PrivateKey. Supports both PKCS#1 (RSA PRIVATE KEY) and PKCS#8
// (PRIVATE KEY) formats, with or without passphrase-less encryption.
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file %q: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %q", path)
	}

	var key interface{}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q (expected RSA PRIVATE KEY or PRIVATE KEY)", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA (got %T)", key)
	}
	return rsaKey, nil
}
