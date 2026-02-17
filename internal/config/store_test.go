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
