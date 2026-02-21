package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/mssql"
	"github.com/faucetdb/faucet/internal/connector/mysql"
	"github.com/faucetdb/faucet/internal/connector/postgres"
	"github.com/faucetdb/faucet/internal/connector/snowflake"
	"github.com/faucetdb/faucet/internal/connector/sqlite"
)

// dataDir holds the --data-dir persistent flag value (set on root command).
var dataDir string

// resolveDataDir returns the data directory from --data-dir flag,
// FAUCET_DATA_DIR env var, or ~/.faucet as fallback.
func resolveDataDir() string {
	if dataDir != "" {
		return dataDir
	}
	if envDir := os.Getenv("FAUCET_DATA_DIR"); envDir != "" {
		return envDir
	}
	home, _ := os.UserHomeDir()
	return home + "/.faucet"
}

// openConfigStore opens the SQLite config store, defaulting to ~/.faucet
// if no data dir was specified.
func openConfigStore() (*config.Store, error) {
	return config.NewStore(resolveDataDir())
}

// newRegistry creates a connector registry with all supported database drivers registered.
func newRegistry() *connector.Registry {
	registry := connector.NewRegistry()
	registry.RegisterDriver("postgres", func() connector.Connector { return postgres.New() })
	registry.RegisterDriver("mysql", func() connector.Connector { return mysql.New() })
	registry.RegisterDriver("mssql", func() connector.Connector { return mssql.New() })
	registry.RegisterDriver("snowflake", func() connector.Connector { return snowflake.New() })
	registry.RegisterDriver("sqlite", func() connector.Connector { return sqlite.New() })
	return registry
}

// --- PID file management ---

func pidFilePath() string {
	return filepath.Join(resolveDataDir(), "faucet.pid")
}

func writePID(pid int) error {
	dir := resolveDataDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(pidFilePath(), []byte(strconv.Itoa(pid)), 0644)
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func removePID() {
	os.Remove(pidFilePath())
}

func logFilePath() string {
	return filepath.Join(resolveDataDir(), "faucet.log")
}

// versionString returns a display version string.
func versionString() string {
	if appVersion == "" || appVersion == "dev" {
		return "dev"
	}
	if strings.HasPrefix(appVersion, "v") {
		return appVersion
	}
	return "v" + appVersion
}
