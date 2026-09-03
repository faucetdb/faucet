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

// seedAPIKey creates an active role and API key and returns the raw key.
func seedAPIKey(t *testing.T, store *config.Store, rawKey string) *model.APIKey {
	t.Helper()
	ctx := context.Background()
	role := &model.Role{Name: "throttle-" + rawKey[len(rawKey)-4:], IsActive: true}
	if err := store.CreateRole(ctx, role); err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	key := &model.APIKey{
		KeyHash:   config.HashAPIKey(rawKey),
		KeyPrefix: rawKey[:8],
		Label:     "throttle",
		RoleID:    role.ID,
		IsActive:  true,
	}
	if err := store.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	return key
}

// waitLastUsed polls until the key's last_used is set and returns it.
func waitLastUsed(t *testing.T, store *config.Store, rawKey string) time.Time {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		k, err := store.GetAPIKeyByHash(context.Background(), config.HashAPIKey(rawKey))
		if err != nil {
			t.Fatalf("GetAPIKeyByHash: %v", err)
		}
		if k.LastUsed != nil {
			return *k.LastUsed
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("last_used was never written")
	return time.Time{}
}

// TestValidateAPIKey_ThrottlesLastUsed verifies that validating the same
// key twice within LastUsedInterval issues a single last_used write.
func TestValidateAPIKey_ThrottlesLastUsed(t *testing.T) {
	auth, store := newTestAuth(t)
	ctx := context.Background()

	orig := LastUsedInterval
	LastUsedInterval = time.Hour
	t.Cleanup(func() { LastUsedInterval = orig })

	rawKey := "faucet_throttle_key_0001"
	key := seedAPIKey(t, store, rawKey)

	if _, err := auth.ValidateAPIKey(ctx, rawKey); err != nil {
		t.Fatalf("ValidateAPIKey #1: %v", err)
	}
	first := waitLastUsed(t, store, rawKey)

	auth.lastUsedMu.Lock()
	stamp, ok := auth.lastUsed[key.ID]
	auth.lastUsedMu.Unlock()
	if !ok {
		t.Fatal("expected throttle entry after first validation")
	}

	// Second validation within the interval must not schedule a write:
	// the throttle stamp is untouched (checked synchronously) ...
	if _, err := auth.ValidateAPIKey(ctx, rawKey); err != nil {
		t.Fatalf("ValidateAPIKey #2: %v", err)
	}
	auth.lastUsedMu.Lock()
	stamp2 := auth.lastUsed[key.ID]
	auth.lastUsedMu.Unlock()
	if !stamp2.Equal(stamp) {
		t.Fatalf("throttle stamp changed on second validation: %v -> %v", stamp, stamp2)
	}

	// ... and the stored last_used stays what the first write set.
	time.Sleep(20 * time.Millisecond)
	k, err := store.GetAPIKeyByHash(ctx, config.HashAPIKey(rawKey))
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if k.LastUsed == nil || !k.LastUsed.Equal(first) {
		t.Fatalf("last_used changed on second validation: %v -> %v", first, k.LastUsed)
	}
}

// TestValidateAPIKey_WritesLastUsedWhenIntervalElapsed verifies that the
// throttle lets a write through once the interval has passed, and that
// keys are throttled independently.
func TestValidateAPIKey_WritesLastUsedWhenIntervalElapsed(t *testing.T) {
	auth, store := newTestAuth(t)
	ctx := context.Background()

	orig := LastUsedInterval
	LastUsedInterval = time.Hour
	t.Cleanup(func() { LastUsedInterval = orig })

	rawKey := "faucet_throttle_key_0002"
	key := seedAPIKey(t, store, rawKey)
	otherKey := "faucet_throttle_key_0003"
	other := seedAPIKey(t, store, otherKey)

	if _, err := auth.ValidateAPIKey(ctx, rawKey); err != nil {
		t.Fatalf("ValidateAPIKey #1: %v", err)
	}
	first := waitLastUsed(t, store, rawKey)

	// A different key is not affected by the first key's throttle entry.
	if _, err := auth.ValidateAPIKey(ctx, otherKey); err != nil {
		t.Fatalf("ValidateAPIKey(other): %v", err)
	}
	waitLastUsed(t, store, otherKey)
	auth.lastUsedMu.Lock()
	_, otherOK := auth.lastUsed[other.ID]
	auth.lastUsedMu.Unlock()
	if !otherOK {
		t.Fatal("expected throttle entry for the second key")
	}

	// Backdate the throttle entry past the interval; the next validation
	// must write again.
	auth.lastUsedMu.Lock()
	auth.lastUsed[key.ID] = time.Now().Add(-2 * time.Hour)
	auth.lastUsedMu.Unlock()

	if _, err := auth.ValidateAPIKey(ctx, rawKey); err != nil {
		t.Fatalf("ValidateAPIKey #2: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		k, err := store.GetAPIKeyByHash(ctx, config.HashAPIKey(rawKey))
		if err != nil {
			t.Fatalf("GetAPIKeyByHash: %v", err)
		}
		if k.LastUsed != nil && k.LastUsed.After(first) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last_used not rewritten after interval elapsed: still %v", k.LastUsed)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
