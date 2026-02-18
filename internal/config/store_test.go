package config

import (
	"context"
	"testing"
	"time"

	"github.com/faucetdb/faucet/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore("") // in-memory
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestServiceCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create
	svc := &model.ServiceConfig{
		Name:     "testdb",
		Label:    "Test Database",
		Driver:   "postgres",
		DSN:      "postgres://localhost/test",
		Schema:   "public",
		IsActive: true,
		Pool:     model.DefaultPoolConfig(),
	}
	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	// GetService
	got, err := s.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.Name != "testdb" {
		t.Errorf("got name %q, want %q", got.Name, "testdb")
	}
	if got.Driver != "postgres" {
		t.Errorf("got driver %q, want %q", got.Driver, "postgres")
	}

	// GetServiceByName
	got2, err := s.GetServiceByName(ctx, "testdb")
	if err != nil {
		t.Fatalf("GetServiceByName: %v", err)
	}
	if got2.ID != svc.ID {
		t.Errorf("got ID %d, want %d", got2.ID, svc.ID)
	}

	// ListServices
	list, err := s.ListServices(ctx)
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("got %d services, want 1", len(list))
	}

	// Update
	svc.Label = "Updated Label"
	if err := s.UpdateService(ctx, svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
	got3, _ := s.GetService(ctx, svc.ID)
	if got3.Label != "Updated Label" {
		t.Errorf("got label %q, want %q", got3.Label, "Updated Label")
	}

	// Delete
	if err := s.DeleteService(ctx, svc.ID); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	_, err = s.GetService(ctx, svc.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRoleCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	role := &model.Role{
		Name:        "readonly",
		Description: "Read-only access",
		IsActive:    true,
	}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if role.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Set access rules
	access := []model.RoleAccess{
		{
			ServiceName:   "*",
			Component:     "_table/*",
			VerbMask:      model.VerbGet,
			RequestorMask: model.RequestorAPI,
			Filters:       []model.Filter{},
			FilterOp:      "AND",
		},
	}
	if err := s.SetRoleAccess(ctx, role.ID, access); err != nil {
		t.Fatalf("SetRoleAccess: %v", err)
	}

	// Get role with access
	got, err := s.GetRole(ctx, role.ID)
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got.Name != "readonly" {
		t.Errorf("got name %q, want %q", got.Name, "readonly")
	}
	if len(got.Access) != 1 {
		t.Fatalf("got %d access rules, want 1", len(got.Access))
	}
	if got.Access[0].VerbMask != model.VerbGet {
		t.Errorf("got verb mask %d, want %d", got.Access[0].VerbMask, model.VerbGet)
	}

	// List roles
	roles, err := s.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) != 1 {
		t.Errorf("got %d roles, want 1", len(roles))
	}

	// Delete
	if err := s.DeleteRole(ctx, role.ID); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
}

func TestAdminCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// HasAnyAdmin - should be false initially
	has, err := s.HasAnyAdmin(ctx)
	if err != nil {
		t.Fatalf("HasAnyAdmin: %v", err)
	}
	if has {
		t.Error("expected no admins initially")
	}

	admin := &model.Admin{
		Email:        "admin@example.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Test Admin",
		IsActive:     true,
		IsSuperAdmin: true,
	}
	if err := s.CreateAdmin(ctx, admin); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	// HasAnyAdmin - should be true now
	has, err = s.HasAnyAdmin(ctx)
	if err != nil {
		t.Fatalf("HasAnyAdmin: %v", err)
	}
	if !has {
		t.Error("expected admin to exist")
	}

	// GetAdminByEmail
	got, err := s.GetAdminByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("GetAdminByEmail: %v", err)
	}
	if got.Name != "Test Admin" {
		t.Errorf("got name %q, want %q", got.Name, "Test Admin")
	}

	// UpdateAdminLastLogin
	if err := s.UpdateAdminLastLogin(ctx, admin.ID); err != nil {
		t.Fatalf("UpdateAdminLastLogin: %v", err)
	}

	// ListAdmins
	admins, err := s.ListAdmins(ctx)
	if err != nil {
		t.Fatalf("ListAdmins: %v", err)
	}
	if len(admins) != 1 {
		t.Errorf("got %d admins, want 1", len(admins))
	}
}

func TestAPIKeyCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Need a role first
	role := &model.Role{Name: "testrole", IsActive: true}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	rawKey := "faucet_key_abc123def456"
	hash := HashAPIKey(rawKey)

	key := &model.APIKey{
		KeyHash:  hash,
		KeyPrefix: rawKey[:8],
		Label:    "Test Key",
		RoleID:   role.ID,
		IsActive: true,
	}
	if err := s.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// GetAPIKeyByHash
	got, err := s.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if got.Label != "Test Key" {
		t.Errorf("got label %q, want %q", got.Label, "Test Key")
	}
	if got.RoleID != role.ID {
		t.Errorf("got role ID %d, want %d", got.RoleID, role.ID)
	}

	// ListAPIKeys
	keys, err := s.ListAPIKeys(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("got %d keys, want 1", len(keys))
	}

	// UpdateAPIKeyLastUsed
	if err := s.UpdateAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("UpdateAPIKeyLastUsed: %v", err)
	}

	// Revoke
	if err := s.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	got2, _ := s.GetAPIKeyByHash(ctx, hash)
	if got2.IsActive {
		t.Error("expected key to be revoked (inactive)")
	}
}

func TestGetRoleByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a role with access rules.
	role := &model.Role{
		Name:        "editor",
		Description: "Can edit things",
		IsActive:    true,
	}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Set access rules so we can verify they're returned.
	access := []model.RoleAccess{
		{
			ServiceName:   "mydb",
			Component:     "_table/*",
			VerbMask:      model.VerbGet | model.VerbPost,
			RequestorMask: model.RequestorAPI,
			Filters:       []model.Filter{},
			FilterOp:      "AND",
		},
	}
	if err := s.SetRoleAccess(ctx, role.ID, access); err != nil {
		t.Fatalf("SetRoleAccess: %v", err)
	}

	// Get by name - should succeed.
	got, err := s.GetRoleByName(ctx, "editor")
	if err != nil {
		t.Fatalf("GetRoleByName: %v", err)
	}
	if got.ID != role.ID {
		t.Errorf("got ID %d, want %d", got.ID, role.ID)
	}
	if got.Name != "editor" {
		t.Errorf("got name %q, want %q", got.Name, "editor")
	}
	if got.Description != "Can edit things" {
		t.Errorf("got description %q, want %q", got.Description, "Can edit things")
	}
	if !got.IsActive {
		t.Error("expected is_active = true")
	}

	// Verify access rules are loaded.
	if len(got.Access) != 1 {
		t.Fatalf("got %d access rules, want 1", len(got.Access))
	}
	if got.Access[0].ServiceName != "mydb" {
		t.Errorf("access[0].ServiceName = %q, want %q", got.Access[0].ServiceName, "mydb")
	}
	if got.Access[0].VerbMask != model.VerbGet|model.VerbPost {
		t.Errorf("access[0].VerbMask = %d, want %d", got.Access[0].VerbMask, model.VerbGet|model.VerbPost)
	}

	// Get by name - not found.
	_, err = s.GetRoleByName(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRevokeAPIKeyByPrefix(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Need a role first.
	role := &model.Role{Name: "prefixtest", IsActive: true}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	rawKey := "faucet_prefixtest_key_abcdef1234"
	hash := HashAPIKey(rawKey)
	prefix := rawKey[:15]

	key := &model.APIKey{
		KeyHash:   hash,
		KeyPrefix: prefix,
		Label:     "Prefix Test Key",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := s.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Revoke by prefix - should succeed.
	if err := s.RevokeAPIKeyByPrefix(ctx, prefix); err != nil {
		t.Fatalf("RevokeAPIKeyByPrefix: %v", err)
	}

	// Verify the key is now inactive.
	got, err := s.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if got.IsActive {
		t.Error("expected key to be revoked (inactive)")
	}

	// Revoking again should return ErrNotFound (already inactive).
	err = s.RevokeAPIKeyByPrefix(ctx, prefix)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound on second revoke, got %v", err)
	}

	// Revoking a nonexistent prefix should return ErrNotFound.
	err = s.RevokeAPIKeyByPrefix(ctx, "nonexistent_pfx")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for unknown prefix, got %v", err)
	}
}

func TestRevokeAPIKeyByPrefix_MultipleKeys(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	role := &model.Role{Name: "multipfx", IsActive: true}
	if err := s.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Create two keys with different prefixes.
	key1 := &model.APIKey{
		KeyHash:   HashAPIKey("faucet_key1_xxxxxxxxxx"),
		KeyPrefix: "faucet_key1_xxx",
		Label:     "Key 1",
		RoleID:    role.ID,
		IsActive:  true,
	}
	key2 := &model.APIKey{
		KeyHash:   HashAPIKey("faucet_key2_yyyyyyyyyy"),
		KeyPrefix: "faucet_key2_yyy",
		Label:     "Key 2",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := s.CreateAPIKey(ctx, key1); err != nil {
		t.Fatalf("CreateAPIKey key1: %v", err)
	}
	if err := s.CreateAPIKey(ctx, key2); err != nil {
		t.Fatalf("CreateAPIKey key2: %v", err)
	}

	// Revoke key1 by prefix - key2 should remain active.
	if err := s.RevokeAPIKeyByPrefix(ctx, "faucet_key1_xxx"); err != nil {
		t.Fatalf("RevokeAPIKeyByPrefix: %v", err)
	}

	got1, _ := s.GetAPIKeyByHash(ctx, key1.KeyHash)
	if got1.IsActive {
		t.Error("key1 should be inactive")
	}

	got2, _ := s.GetAPIKeyByHash(ctx, key2.KeyHash)
	if !got2.IsActive {
		t.Error("key2 should still be active")
	}
}

func TestHashAPIKey(t *testing.T) {
	hash1 := HashAPIKey("test-key-123")
	hash2 := HashAPIKey("test-key-123")
	hash3 := HashAPIKey("different-key")

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different input should produce different hash")
	}
	if len(hash1) != 64 { // SHA-256 hex = 64 chars
		t.Errorf("hash length %d, want 64", len(hash1))
	}
}

func TestPoolConfigRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	pool := model.PoolConfig{
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: 10 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	svc := &model.ServiceConfig{
		Name:     "pooltest",
		Driver:   "postgres",
		DSN:      "postgres://localhost/test",
		Schema:   "public",
		IsActive: true,
		Pool:     pool,
	}
	if err := s.CreateService(ctx, svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	got, err := s.GetService(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}

	if got.Pool.MaxOpenConns != 50 {
		t.Errorf("MaxOpenConns: got %d, want 50", got.Pool.MaxOpenConns)
	}
	if got.Pool.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns: got %d, want 10", got.Pool.MaxIdleConns)
	}
	if got.Pool.ConnMaxLifetime != 10*time.Minute {
		t.Errorf("ConnMaxLifetime: got %v, want 10m", got.Pool.ConnMaxLifetime)
	}
}
