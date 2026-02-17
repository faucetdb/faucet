package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// YAMLConfig represents the top-level faucet configuration file.
type YAMLConfig struct {
	Server   ServerConfig  `yaml:"server"`
	Auth     AuthConfig    `yaml:"auth"`
	Services []ServiceYAML `yaml:"services"`
	MCP      MCPConfig     `yaml:"mcp"`
	Logging  LoggingConfig `yaml:"logging"`
}

// ServerConfig controls the HTTP server behavior.
type ServerConfig struct {
	Host            string     `yaml:"host"`
	Port            int        `yaml:"port"`
	MaxBodySize     string     `yaml:"max_body_size"`
	MaxBatchSize    int        `yaml:"max_batch_size"`
	ShutdownTimeout string     `yaml:"shutdown_timeout"`
	CORS            CORSConfig `yaml:"cors"`
	TLS             TLSConfig  `yaml:"tls"`
}

// CORSConfig controls cross-origin resource sharing settings.
type CORSConfig struct {
	Origins []string `yaml:"origins"`
	Methods []string `yaml:"methods"`
}

// TLSConfig controls TLS termination at the server level.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// AuthConfig controls authentication settings.
type AuthConfig struct {
	JWTSecret    string `yaml:"jwt_secret"`
	JWTExpiry    string `yaml:"jwt_expiry"`
	APIKeyHeader string `yaml:"api_key_header"`
}

// ServiceYAML defines a database service in the YAML configuration file.
type ServiceYAML struct {
	Name     string          `yaml:"name"`
	Driver   string          `yaml:"driver"`
	DSN      string          `yaml:"dsn"`
	Schema   string          `yaml:"schema"`
	ReadOnly bool            `yaml:"read_only"`
	RawSQL   bool            `yaml:"raw_sql_allowed"`
	Pool     *PoolYAMLConfig `yaml:"pool,omitempty"`
}

// PoolYAMLConfig controls the connection pool for a service in YAML config.
type PoolYAMLConfig struct {
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime string `yaml:"conn_max_lifetime"`
}

// MCPConfig controls the MCP (Model Context Protocol) server.
type MCPConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Transport     string `yaml:"transport"`
	RawSQLAllowed bool   `yaml:"raw_sql_allowed"`
	RawSQLTimeout string `yaml:"raw_sql_timeout"`
	RawSQLMaxRows int    `yaml:"raw_sql_max_rows"`
}

// LoggingConfig controls log output.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LoadYAMLConfig reads and parses a YAML configuration file. Environment
// variables referenced as ${VAR_NAME} in the file are expanded before parsing.
func LoadYAMLConfig(path string) (*YAMLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables: ${VAR_NAME}
	content := os.ExpandEnv(string(data))

	var cfg YAMLConfig
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	return &cfg, nil
}

// DefaultYAMLConfig returns a YAMLConfig pre-filled with sensible defaults.
func DefaultYAMLConfig() *YAMLConfig {
	return &YAMLConfig{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			MaxBodySize:     "10MB",
			MaxBatchSize:    1000,
			ShutdownTimeout: "30s",
			CORS: CORSConfig{
				Origins: []string{"*"},
				Methods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			},
		},
		Auth: AuthConfig{
			JWTExpiry:    "1h",
			APIKeyHeader: "X-API-Key",
		},
		MCP: MCPConfig{
			Enabled:       true,
			Transport:     "stdio",
			RawSQLAllowed: false,
			RawSQLTimeout: "30s",
			RawSQLMaxRows: 1000,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// WriteDefaultConfig writes the default configuration to a YAML file.
func WriteDefaultConfig(path string) error {
	cfg := DefaultYAMLConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
