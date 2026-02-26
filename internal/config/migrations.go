package config

import (
	"fmt"
	"strings"
)

func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			driver TEXT NOT NULL,
			dsn TEXT NOT NULL,
			schema_name TEXT NOT NULL DEFAULT 'public',
			read_only INTEGER NOT NULL DEFAULT 0,
			raw_sql_allowed INTEGER NOT NULL DEFAULT 0,
			is_active INTEGER NOT NULL DEFAULT 1,
			max_open_conns INTEGER NOT NULL DEFAULT 25,
			max_idle_conns INTEGER NOT NULL DEFAULT 5,
			conn_max_lifetime_ms INTEGER NOT NULL DEFAULT 300000,
			conn_max_idle_time_ms INTEGER NOT NULL DEFAULT 60000,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS admins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			is_super_admin INTEGER NOT NULL DEFAULT 0,
			last_login_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS roles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS role_access (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
			service_name TEXT NOT NULL DEFAULT '*',
			component TEXT NOT NULL DEFAULT '*',
			verb_mask INTEGER NOT NULL DEFAULT 31,
			requestor_mask INTEGER NOT NULL DEFAULT 1,
			filters_json TEXT NOT NULL DEFAULT '[]',
			filter_op TEXT NOT NULL DEFAULT 'AND'
		)`,

		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key_hash TEXT UNIQUE NOT NULL,
			key_prefix TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			role_id INTEGER NOT NULL REFERENCES roles(id),
			is_active INTEGER NOT NULL DEFAULT 1,
			expires_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME
		)`,

		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_role_access_role_id ON role_access(role_id)`,

		// v2: Add private_key_path for Snowflake JWT / key-pair auth
		`ALTER TABLE services ADD COLUMN private_key_path TEXT NOT NULL DEFAULT ''`,

		// v3: Key-value settings table (telemetry, instance ID, etc.)
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,

		// v4: Schema contract locking — snapshots of locked table schemas.
		`CREATE TABLE IF NOT EXISTS schema_contracts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			service_name TEXT NOT NULL,
			table_name TEXT NOT NULL,
			schema_json TEXT NOT NULL,
			locked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			promoted_at DATETIME,
			UNIQUE(service_name, table_name)
		)`,

		// v5: Schema lock mode per service (none, auto, strict).
		`ALTER TABLE services ADD COLUMN schema_lock TEXT NOT NULL DEFAULT 'none'`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			// SQLite ALTER TABLE ADD COLUMN fails if column already exists;
			// treat "duplicate column" as a no-op for idempotent migrations.
			if strings.Contains(err.Error(), "duplicate column") {
				continue
			}
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}
	return nil
}
