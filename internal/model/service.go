package model

import "time"

// ServiceConfig holds the configuration for a database service connection.
// Each service maps to one database and is exposed as an API namespace.
type ServiceConfig struct {
	ID        int64      `json:"id" db:"id"`
	Name      string     `json:"name" db:"name"`
	Label     string     `json:"label" db:"label"`
	Driver    string     `json:"driver" db:"driver"` // postgres, mysql, mssql, snowflake, sqlite
	DSN            string `json:"dsn,omitempty" db:"dsn"` // Accepted on input; omitted in list responses via serviceToMap
	PrivateKeyPath string `json:"private_key_path,omitempty" db:"private_key_path"`
	Schema         string `json:"schema" db:"schema_name"`
	ReadOnly   bool   `json:"read_only" db:"read_only"`
	RawSQL     bool   `json:"raw_sql_allowed" db:"raw_sql_allowed"`
	IsActive   bool   `json:"is_active" db:"is_active"`
	SchemaLock string `json:"schema_lock" db:"schema_lock"`
	Pool      PoolConfig `json:"pool"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}

// PoolConfig controls the database connection pool behavior for a service.
type PoolConfig struct {
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`
	PingInterval    time.Duration `yaml:"ping_interval" json:"ping_interval"`
}

// DefaultPoolConfig returns sensible defaults for a database connection pool.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
		PingInterval:    30 * time.Second,
	}
}
