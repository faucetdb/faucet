package service

import (
	"context"
	"testing"
	"time"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
)

func newTestAuth(t *testing.T) (*AuthService, *config.Store) {
	t.Helper()
	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	auth := NewAuthService(store, "test-secret-key-for-jwt")
	return auth, store
}

func TestJWTRoundTrip(t *testing.T) {
	auth, _ := newTestAuth(t)
	ctx := context.Background()

	// Issue a token
	token, err := auth.IssueJWT(ctx, 42, "admin@example.com", 1*time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Validate the token
	principal, err := auth.ValidateJWT(ctx, token)
	if err != nil {
		t.Fatalf("ValidateJWT: %v", err)
	}
	if principal.AdminID != 42 {
		t.Errorf("AdminID: got %d, want 42", principal.AdminID)
	}
	if principal.Email != "admin@example.com" {
		t.Errorf("Email: got %q, want %q", principal.Email, "admin@example.com")
	}
}

func TestJWTExpired(t *testing.T) {
	auth, _ := newTestAuth(t)
	ctx := context.Background()

	// Issue a token with negative TTL (already expired)
	token, err := auth.IssueJWT(ctx, 1, "test@test.com", -1*time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}

	_, err = auth.ValidateJWT(ctx, token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestJWTInvalidToken(t *testing.T) {
	auth, _ := newTestAuth(t)
	ctx := context.Background()

	_, err := auth.ValidateJWT(ctx, "garbage.token.here")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestAPIKeyValidation(t *testing.T) {
	auth, store := newTestAuth(t)
	ctx := context.Background()

	// Create a role
	role := &model.Role{Name: "testrole", IsActive: true}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}

	// Create an API key
	rawKey := "faucet_test_key_abcdef123456"
	hash := config.HashAPIKey(rawKey)
	key := &model.APIKey{
		KeyHash:   hash,
		KeyPrefix: rawKey[:8],
		Label:     "test",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := store.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Validate the key
	principal, err := auth.ValidateAPIKey(ctx, rawKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if principal.RoleID != role.ID {
		t.Errorf("RoleID: got %d, want %d", principal.RoleID, role.ID)
	}

	// Invalid key
	_, err = auth.ValidateAPIKey(ctx, "wrong_key")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAPIKeyRevoked(t *testing.T) {
	auth, store := newTestAuth(t)
	ctx := context.Background()

	role := &model.Role{Name: "testrole", IsActive: true}
	store.CreateRole(ctx, role)

	rawKey := "faucet_revoke_test_key"
	hash := config.HashAPIKey(rawKey)
	key := &model.APIKey{
		KeyHash:   hash,
		KeyPrefix: rawKey[:8],
		Label:     "revoke-test",
		RoleID:    role.ID,
		IsActive:  true,
	}
	store.CreateAPIKey(ctx, key)

	// Revoke
	store.RevokeAPIKey(ctx, key.ID)

	// Should fail
	_, err := auth.ValidateAPIKey(ctx, rawKey)
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
	}
}
