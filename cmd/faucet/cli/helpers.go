package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/mssql"
	"github.com/faucetdb/faucet/internal/connector/mysql"
	"github.com/faucetdb/faucet/internal/connector/oracle"
	"github.com/faucetdb/faucet/internal/connector/postgres"
	"github.com/faucetdb/faucet/internal/connector/snowflake"
	"github.com/faucetdb/faucet/internal/connector/sqlite"
	"github.com/faucetdb/faucet/internal/model"
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

// jwtSecretSettingKey is the config-store settings key under which a
// generated JWT signing secret is persisted.
const jwtSecretSettingKey = "auth.jwt_secret"

// jwtSecretBytes is the size of a generated JWT signing secret before hex
// encoding (256 bits).
const jwtSecretBytes = 32

// resolveJWTSecret returns the secret used to sign admin JWTs.
//
// Precedence:
//  1. The viper key "auth.jwt_secret" (FAUCET_AUTH_JWT_SECRET, its alias
//     FAUCET_JWT_SECRET, or auth.jwt_secret in faucet.yaml).
//  2. The value persisted under the "auth.jwt_secret" settings key in the
//     config store.
//  3. A freshly generated 256-bit random secret, which is persisted in the
//     config store so that later processes (and the standalone MCP HTTP
//     listener sharing the same data directory) sign and verify with the
//     same key.
//
// There is deliberately no hard-coded fallback: a predictable signing secret
// lets anyone mint an admin token, and admin tokens bypass RBAC.
func resolveJWTSecret(ctx context.Context, store *config.Store, logger *slog.Logger) (string, error) {
	if secret := strings.TrimSpace(viper.GetString(jwtSecretSettingKey)); secret != "" {
		return secret, nil
	}
	if store == nil {
		return "", errors.New("resolve jwt secret: no config store")
	}

	secret, err := store.GetSetting(ctx, jwtSecretSettingKey)
	switch {
	case err == nil:
		if strings.TrimSpace(secret) != "" {
			return secret, nil
		}
	case !errors.Is(err, config.ErrNotFound):
		return "", fmt.Errorf("read stored jwt secret: %w", err)
	}

	raw := make([]byte, jwtSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate jwt secret: %w", err)
	}
	secret = hex.EncodeToString(raw)
	if err := store.SetSetting(ctx, jwtSecretSettingKey, secret); err != nil {
		return "", fmt.Errorf("persist jwt secret: %w", err)
	}
	if logger != nil {
		logger.Info("generated a new JWT signing secret and stored it in the data directory; set FAUCET_AUTH_JWT_SECRET to pin it",
			"setting", jwtSecretSettingKey)
	}
	return secret, nil
}

// warnUnusableRoles logs a WARN line for every role that will deny all
// requests from the API keys bound to it: roles with no access rules, roles
// whose rules all have verb_mask 0, and inactive roles. Roles with no active
// API keys are skipped since nothing can be locked out by them.
//
// This is an upgrade diagnostic: before verb masks were enforced such roles
// silently allowed everything, so an operator upgrading needs to know which
// keys are about to start receiving 403s and how to fix it.
func warnUnusableRoles(ctx context.Context, store *config.Store, logger *slog.Logger) {
	if store == nil || logger == nil {
		return
	}
	roles, err := store.ListRoles(ctx)
	if err != nil {
		logger.Warn("failed to list roles for upgrade diagnostics", "error", err)
		return
	}
	keys, err := store.ListAPIKeys(ctx)
	if err != nil {
		logger.Warn("failed to list API keys for upgrade diagnostics", "error", err)
		return
	}

	activeKeys := make(map[int64]int, len(roles))
	for _, k := range keys {
		if !k.IsActive {
			continue
		}
		if k.ExpiresAt != nil && !k.ExpiresAt.After(timeNow()) {
			continue
		}
		activeKeys[k.RoleID]++
	}

	for _, role := range roles {
		n := activeKeys[role.ID]
		if n == 0 {
			continue
		}
		reason, fix := unusableRoleReason(role)
		if reason == "" {
			continue
		}
		logger.Warn(fmt.Sprintf("role %q (id=%d) %s; %d active API %s will be denied (403) — run: %s",
			role.Name, role.ID, reason, n, pluralKeys(n), fix))
	}
}

// unusableRoleReason returns a human-readable reason a role denies every
// request plus the CLI command that fixes it, or empty strings when the role
// is usable.
func unusableRoleReason(role model.Role) (reason, fix string) {
	if !role.IsActive {
		return "is inactive", fmt.Sprintf("faucet role list   (activate role %q in the admin UI or via PUT /api/v1/system/role/%d)", role.Name, role.ID)
	}
	if len(role.Access) == 0 {
		return "has no access rules", fmt.Sprintf("faucet role grant --role %s --verbs GET", role.Name)
	}
	for _, a := range role.Access {
		if a.VerbMask != 0 {
			return "", ""
		}
	}
	return "has only rules with verb_mask=0", fmt.Sprintf("faucet role grant --role %s --verbs GET", role.Name)
}

func pluralKeys(n int) string {
	if n == 1 {
		return "key"
	}
	return "keys"
}

// timeNow is a hook for tests that need to control API key expiry checks.
var timeNow = time.Now

// newRegistry creates a connector registry with all supported database drivers registered.
func newRegistry() *connector.Registry {
	registry := connector.NewRegistry()
	registry.RegisterDriver("postgres", func() connector.Connector { return postgres.New() })
	registry.RegisterDriver("mysql", func() connector.Connector { return mysql.New() })
	registry.RegisterDriver("mssql", func() connector.Connector { return mssql.New() })
	registry.RegisterDriver("snowflake", func() connector.Connector { return snowflake.New() })
	registry.RegisterDriver("oracle", func() connector.Connector { return oracle.New() })
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
	_ = os.Remove(pidFilePath())
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
