package config

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/faucetdb/faucet/internal/model"
)

// Store manages Faucet's internal configuration state backed by SQLite.
// It persists services, roles, API keys, and admin accounts.
type Store struct {
	db *sqlx.DB
}

// NewStore creates a new config store. Pass empty string for in-memory.
func NewStore(dataDir string) (*Store, error) {
	var dsn string
	if dataDir == "" {
		dsn = ":memory:?_journal_mode=WAL"
	} else {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
		dsn = filepath.Join(dataDir, "faucet.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	}

	db, err := sqlx.Connect("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open config database: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes

	// Enable foreign keys (off by default in SQLite).
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate config database: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Service CRUD
// ---------------------------------------------------------------------------

// serviceRow is a flat struct that maps 1:1 to the services table columns.
// We use it for sqlx scanning because model.ServiceConfig has a nested Pool
// struct that doesn't map directly to columns.
type serviceRow struct {
	ID                int64     `db:"id"`
	Name              string    `db:"name"`
	Label             string    `db:"label"`
	Driver            string    `db:"driver"`
	DSN               string    `db:"dsn"`
	PrivateKeyPath    string    `db:"private_key_path"`
	SchemaName        string    `db:"schema_name"`
	ReadOnly          bool      `db:"read_only"`
	RawSQLAllowed     bool      `db:"raw_sql_allowed"`
	IsActive          bool      `db:"is_active"`
	MaxOpenConns      int       `db:"max_open_conns"`
	MaxIdleConns      int       `db:"max_idle_conns"`
	ConnMaxLifetimeMs int64     `db:"conn_max_lifetime_ms"`
	ConnMaxIdleTimeMs int64     `db:"conn_max_idle_time_ms"`
	CreatedAt         time.Time `db:"created_at"`
	UpdatedAt         time.Time `db:"updated_at"`
}

func serviceRowFromModel(svc *model.ServiceConfig) serviceRow {
	return serviceRow{
		ID:                svc.ID,
		Name:              svc.Name,
		Label:             svc.Label,
		Driver:            svc.Driver,
		DSN:               svc.DSN,
		PrivateKeyPath:    svc.PrivateKeyPath,
		SchemaName:        svc.Schema,
		ReadOnly:          svc.ReadOnly,
		RawSQLAllowed:     svc.RawSQL,
		IsActive:          svc.IsActive,
		MaxOpenConns:      svc.Pool.MaxOpenConns,
		MaxIdleConns:      svc.Pool.MaxIdleConns,
		ConnMaxLifetimeMs: svc.Pool.ConnMaxLifetime.Milliseconds(),
		ConnMaxIdleTimeMs: svc.Pool.ConnMaxIdleTime.Milliseconds(),
		CreatedAt:         svc.CreatedAt,
		UpdatedAt:         svc.UpdatedAt,
	}
}

func (r serviceRow) toModel() model.ServiceConfig {
	return model.ServiceConfig{
		ID:             r.ID,
		Name:           r.Name,
		Label:          r.Label,
		Driver:         r.Driver,
		DSN:            r.DSN,
		PrivateKeyPath: r.PrivateKeyPath,
		Schema:         r.SchemaName,
		ReadOnly:       r.ReadOnly,
		RawSQL:         r.RawSQLAllowed,
		IsActive:       r.IsActive,
		Pool: model.PoolConfig{
			MaxOpenConns:    r.MaxOpenConns,
			MaxIdleConns:    r.MaxIdleConns,
			ConnMaxLifetime: time.Duration(r.ConnMaxLifetimeMs) * time.Millisecond,
			ConnMaxIdleTime: time.Duration(r.ConnMaxIdleTimeMs) * time.Millisecond,
		},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// CreateService inserts a new service configuration. The ID, CreatedAt, and
// UpdatedAt fields on svc are populated after a successful insert.
func (s *Store) CreateService(ctx context.Context, svc *model.ServiceConfig) error {
	now := time.Now().UTC()
	svc.CreatedAt = now
	svc.UpdatedAt = now

	row := serviceRowFromModel(svc)

	const q = `INSERT INTO services
		(name, label, driver, dsn, private_key_path, schema_name, read_only, raw_sql_allowed, is_active,
		 max_open_conns, max_idle_conns, conn_max_lifetime_ms, conn_max_idle_time_ms,
		 created_at, updated_at)
		VALUES
		(:name, :label, :driver, :dsn, :private_key_path, :schema_name, :read_only, :raw_sql_allowed, :is_active,
		 :max_open_conns, :max_idle_conns, :conn_max_lifetime_ms, :conn_max_idle_time_ms,
		 :created_at, :updated_at)`

	result, err := s.db.NamedExecContext(ctx, q, row)
	if err != nil {
		return fmt.Errorf("insert service: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get service id: %w", err)
	}
	svc.ID = id
	return nil
}

// GetService returns a service by ID.
func (s *Store) GetService(ctx context.Context, id int64) (*model.ServiceConfig, error) {
	var row serviceRow
	if err := s.db.GetContext(ctx, &row, "SELECT * FROM services WHERE id = ?", id); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get service: %w", err)
	}
	svc := row.toModel()
	return &svc, nil
}

// GetServiceByName returns a service by its unique name.
func (s *Store) GetServiceByName(ctx context.Context, name string) (*model.ServiceConfig, error) {
	var row serviceRow
	if err := s.db.GetContext(ctx, &row, "SELECT * FROM services WHERE name = ?", name); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get service by name: %w", err)
	}
	svc := row.toModel()
	return &svc, nil
}

// ListServices returns all configured service definitions.
func (s *Store) ListServices(ctx context.Context) ([]model.ServiceConfig, error) {
	var rows []serviceRow
	if err := s.db.SelectContext(ctx, &rows, "SELECT * FROM services ORDER BY name"); err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	services := make([]model.ServiceConfig, len(rows))
	for i, r := range rows {
		services[i] = r.toModel()
	}
	return services, nil
}

// UpdateService updates an existing service configuration. The UpdatedAt field
// on svc is refreshed automatically.
func (s *Store) UpdateService(ctx context.Context, svc *model.ServiceConfig) error {
	svc.UpdatedAt = time.Now().UTC()
	row := serviceRowFromModel(svc)

	const q = `UPDATE services SET
		name = :name, label = :label, driver = :driver, dsn = :dsn, private_key_path = :private_key_path,
		schema_name = :schema_name, read_only = :read_only, raw_sql_allowed = :raw_sql_allowed,
		is_active = :is_active, max_open_conns = :max_open_conns, max_idle_conns = :max_idle_conns,
		conn_max_lifetime_ms = :conn_max_lifetime_ms, conn_max_idle_time_ms = :conn_max_idle_time_ms,
		updated_at = :updated_at
		WHERE id = :id`

	result, err := s.db.NamedExecContext(ctx, q, row)
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update service rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteService removes a service configuration by ID.
func (s *Store) DeleteService(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM services WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete service rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Role CRUD
// ---------------------------------------------------------------------------

// CreateRole inserts a new role. The ID, CreatedAt, and UpdatedAt fields are
// populated after a successful insert.
func (s *Store) CreateRole(ctx context.Context, role *model.Role) error {
	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now

	const q = `INSERT INTO roles (name, description, is_active, created_at, updated_at)
		VALUES (:name, :description, :is_active, :created_at, :updated_at)`

	result, err := s.db.NamedExecContext(ctx, q, role)
	if err != nil {
		return fmt.Errorf("insert role: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get role id: %w", err)
	}
	role.ID = id
	return nil
}

// GetRole returns a role by ID, including its access rules.
func (s *Store) GetRole(ctx context.Context, id int64) (*model.Role, error) {
	var role model.Role
	if err := s.db.GetContext(ctx, &role, "SELECT * FROM roles WHERE id = ?", id); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get role: %w", err)
	}

	access, err := s.GetRoleAccess(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get role access: %w", err)
	}
	role.Access = access
	return &role, nil
}

// GetRoleByName returns a role by its unique name.
func (s *Store) GetRoleByName(ctx context.Context, name string) (*model.Role, error) {
	var role model.Role
	if err := s.db.GetContext(ctx, &role, "SELECT * FROM roles WHERE name = ?", name); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get role by name: %w", err)
	}
	access, err := s.GetRoleAccess(ctx, role.ID)
	if err != nil {
		return nil, fmt.Errorf("get role access: %w", err)
	}
	role.Access = access
	return &role, nil
}

// ListRoles returns all configured roles with their access rules.
func (s *Store) ListRoles(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	if err := s.db.SelectContext(ctx, &roles, "SELECT * FROM roles ORDER BY name"); err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	for i := range roles {
		access, err := s.GetRoleAccess(ctx, roles[i].ID)
		if err != nil {
			return nil, fmt.Errorf("get role access for role %d: %w", roles[i].ID, err)
		}
		roles[i].Access = access
	}
	return roles, nil
}

// UpdateRole updates an existing role. The UpdatedAt field is refreshed
// automatically.
func (s *Store) UpdateRole(ctx context.Context, role *model.Role) error {
	role.UpdatedAt = time.Now().UTC()

	const q = `UPDATE roles SET
		name = :name, description = :description, is_active = :is_active, updated_at = :updated_at
		WHERE id = :id`

	result, err := s.db.NamedExecContext(ctx, q, role)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update role rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRole removes a role by ID. Associated role_access rows are cascade
// deleted by the foreign key constraint.
func (s *Store) DeleteRole(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM roles WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete role rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// RoleAccess
// ---------------------------------------------------------------------------

// roleAccessRow is a flat struct that maps 1:1 to the role_access table.
// The filters_json column stores the JSON-encoded []model.Filter.
type roleAccessRow struct {
	ID            int64  `db:"id"`
	RoleID        int64  `db:"role_id"`
	ServiceName   string `db:"service_name"`
	Component     string `db:"component"`
	VerbMask      int    `db:"verb_mask"`
	RequestorMask int    `db:"requestor_mask"`
	FiltersJSON   string `db:"filters_json"`
	FilterOp      string `db:"filter_op"`
}

func roleAccessRowFromModel(a model.RoleAccess) (roleAccessRow, error) {
	filtersJSON, err := json.Marshal(a.Filters)
	if err != nil {
		return roleAccessRow{}, fmt.Errorf("marshal filters: %w", err)
	}
	return roleAccessRow{
		ID:            a.ID,
		RoleID:        a.RoleID,
		ServiceName:   a.ServiceName,
		Component:     a.Component,
		VerbMask:      a.VerbMask,
		RequestorMask: a.RequestorMask,
		FiltersJSON:   string(filtersJSON),
		FilterOp:      a.FilterOp,
	}, nil
}

func (r roleAccessRow) toModel() (model.RoleAccess, error) {
	var filters []model.Filter
	if r.FiltersJSON != "" && r.FiltersJSON != "[]" {
		if err := json.Unmarshal([]byte(r.FiltersJSON), &filters); err != nil {
			return model.RoleAccess{}, fmt.Errorf("unmarshal filters: %w", err)
		}
	}
	if filters == nil {
		filters = []model.Filter{}
	}
	return model.RoleAccess{
		ID:            r.ID,
		RoleID:        r.RoleID,
		ServiceName:   r.ServiceName,
		Component:     r.Component,
		VerbMask:      r.VerbMask,
		RequestorMask: r.RequestorMask,
		Filters:       filters,
		FilterOp:      r.FilterOp,
	}, nil
}

// SetRoleAccess replaces all access rules for a role within a transaction.
func (s *Store) SetRoleAccess(ctx context.Context, roleID int64, access []model.RoleAccess) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete existing access rules for this role.
	if _, err := tx.ExecContext(ctx, "DELETE FROM role_access WHERE role_id = ?", roleID); err != nil {
		return fmt.Errorf("delete existing role access: %w", err)
	}

	const insertQ = `INSERT INTO role_access
		(role_id, service_name, component, verb_mask, requestor_mask, filters_json, filter_op)
		VALUES (:role_id, :service_name, :component, :verb_mask, :requestor_mask, :filters_json, :filter_op)`

	for _, a := range access {
		a.RoleID = roleID
		row, err := roleAccessRowFromModel(a)
		if err != nil {
			return err
		}
		if _, err := tx.NamedExecContext(ctx, insertQ, row); err != nil {
			return fmt.Errorf("insert role access: %w", err)
		}
	}

	return tx.Commit()
}

// GetRoleAccess returns all access rules for a role.
func (s *Store) GetRoleAccess(ctx context.Context, roleID int64) ([]model.RoleAccess, error) {
	var rows []roleAccessRow
	if err := s.db.SelectContext(ctx, &rows, "SELECT * FROM role_access WHERE role_id = ? ORDER BY id", roleID); err != nil {
		return nil, fmt.Errorf("get role access: %w", err)
	}

	access := make([]model.RoleAccess, 0, len(rows))
	for _, r := range rows {
		a, err := r.toModel()
		if err != nil {
			return nil, err
		}
		access = append(access, a)
	}
	return access, nil
}

// ---------------------------------------------------------------------------
// Admin CRUD
// ---------------------------------------------------------------------------

// CreateAdmin inserts a new admin account. The ID, CreatedAt, and UpdatedAt
// fields are populated after a successful insert.
func (s *Store) CreateAdmin(ctx context.Context, admin *model.Admin) error {
	now := time.Now().UTC()
	admin.CreatedAt = now
	admin.UpdatedAt = now

	const q = `INSERT INTO admins
		(email, password_hash, name, is_active, is_super_admin, created_at, updated_at)
		VALUES
		(:email, :password_hash, :name, :is_active, :is_super_admin, :created_at, :updated_at)`

	result, err := s.db.NamedExecContext(ctx, q, admin)
	if err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get admin id: %w", err)
	}
	admin.ID = id
	return nil
}

// GetAdminByEmail returns an admin by email address.
func (s *Store) GetAdminByEmail(ctx context.Context, email string) (*model.Admin, error) {
	var admin model.Admin
	if err := s.db.GetContext(ctx, &admin, "SELECT * FROM admins WHERE email = ?", email); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get admin by email: %w", err)
	}
	return &admin, nil
}

// ListAdmins returns all admin accounts.
func (s *Store) ListAdmins(ctx context.Context) ([]model.Admin, error) {
	var admins []model.Admin
	if err := s.db.SelectContext(ctx, &admins, "SELECT * FROM admins ORDER BY email"); err != nil {
		return nil, fmt.Errorf("list admins: %w", err)
	}
	return admins, nil
}

// HasAnyAdmin reports whether at least one admin account exists. This is used
// for first-run detection to trigger the initial setup flow.
func (s *Store) HasAnyAdmin(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM admins"); err != nil {
		return false, fmt.Errorf("count admins: %w", err)
	}
	return count > 0, nil
}

// UpdateAdminLastLogin sets the last_login_at timestamp for an admin.
func (s *Store) UpdateAdminLastLogin(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		"UPDATE admins SET last_login_at = ?, updated_at = ? WHERE id = ?", now, now, id)
	if err != nil {
		return fmt.Errorf("update admin last login: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update admin last login rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// API Key management
// ---------------------------------------------------------------------------

// CreateAPIKey inserts a new API key record. The key_hash must already be set
// (use HashAPIKey). The ID and CreatedAt fields are populated after insert.
func (s *Store) CreateAPIKey(ctx context.Context, key *model.APIKey) error {
	key.CreatedAt = time.Now().UTC()

	const q = `INSERT INTO api_keys
		(key_hash, key_prefix, label, role_id, is_active, expires_at, created_at)
		VALUES
		(:key_hash, :key_prefix, :label, :role_id, :is_active, :expires_at, :created_at)`

	result, err := s.db.NamedExecContext(ctx, q, key)
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get api key id: %w", err)
	}
	key.ID = id
	return nil
}

// GetAPIKeyByHash looks up an API key by its SHA-256 hash.
func (s *Store) GetAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	var key model.APIKey
	if err := s.db.GetContext(ctx, &key, "SELECT * FROM api_keys WHERE key_hash = ?", hash); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return &key, nil
}

// ListAPIKeys returns all API keys.
func (s *Store) ListAPIKeys(ctx context.Context) ([]model.APIKey, error) {
	var keys []model.APIKey
	if err := s.db.SelectContext(ctx, &keys, "SELECT * FROM api_keys ORDER BY created_at DESC"); err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	return keys, nil
}

// RevokeAPIKey marks an API key as inactive by ID.
func (s *Store) RevokeAPIKey(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE api_keys SET is_active = 0 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke api key rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAPIKeyByPrefix marks an API key as inactive by its prefix.
func (s *Store) RevokeAPIKeyByPrefix(ctx context.Context, prefix string) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE api_keys SET is_active = 0 WHERE key_prefix = ? AND is_active = 1", prefix)
	if err != nil {
		return fmt.Errorf("revoke api key by prefix: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke api key rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAPIKeyLastUsed sets the last_used timestamp for an API key.
func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		"UPDATE api_keys SET last_used = ? WHERE id = ?", now, id)
	if err != nil {
		return fmt.Errorf("update api key last used: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update api key last used rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// HashAPIKey returns the hex-encoded SHA-256 hash of a raw API key string.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
