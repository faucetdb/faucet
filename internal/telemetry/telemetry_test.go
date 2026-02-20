package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockStore implements SettingsStore for testing.
type mockStore struct {
	data map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{data: make(map[string]string)}
}

func (m *mockStore) GetSetting(_ context.Context, key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return v, nil
}

func (m *mockStore) SetSetting(_ context.Context, key, value string) error {
	m.data[key] = value
	return nil
}

func TestResolveInstanceID_GeneratesAndPersists(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()

	id := resolveInstanceID(ctx, store)
	if id == "" {
		t.Fatal("expected non-empty instance ID")
	}

	// Should be persisted
	stored, err := store.GetSetting(ctx, "instance_id")
	if err != nil {
		t.Fatalf("expected instance_id in store: %v", err)
	}
	if stored != id {
		t.Errorf("stored ID %q != returned ID %q", stored, id)
	}

	// Second call should return same ID
	id2 := resolveInstanceID(ctx, store)
	if id2 != id {
		t.Errorf("expected same ID on second call, got %q vs %q", id2, id)
	}
}

func TestResolveInstanceID_NilStore(t *testing.T) {
	id := resolveInstanceID(context.Background(), nil)
	if id == "" {
		t.Fatal("expected non-empty instance ID even with nil store")
	}
}

// setTestKey sets a fake PostHog API key for testing and restores it on cleanup.
func setTestKey(t *testing.T) {
	t.Helper()
	old := posthogAPIKey
	posthogAPIKey = "phc_test_key"
	t.Cleanup(func() { posthogAPIKey = old })
}

func TestNew_DisabledWhenNoKey(t *testing.T) {
	old := posthogAPIKey
	posthogAPIKey = ""
	defer func() { posthogAPIKey = old }()

	store := newMockStore()
	tracker := New(context.Background(), store, func() Properties { return Properties{} })
	if tracker != nil {
		t.Fatal("expected nil tracker when no API key is set")
	}
}

func TestNew_DisabledViaSetting(t *testing.T) {
	setTestKey(t)
	store := newMockStore()
	store.data["telemetry.enabled"] = "false"

	tracker := New(context.Background(), store, func() Properties { return Properties{} })
	if tracker != nil {
		t.Fatal("expected nil tracker when telemetry is disabled via setting")
	}
}

func TestNew_DisabledViaEnv(t *testing.T) {
	setTestKey(t)
	t.Setenv("FAUCET_TELEMETRY", "0")

	store := newMockStore()
	tracker := New(context.Background(), store, func() Properties { return Properties{} })
	if tracker != nil {
		t.Fatal("expected nil tracker when FAUCET_TELEMETRY=0")
	}
}

func TestNew_DisabledViaEnvCaseInsensitive(t *testing.T) {
	setTestKey(t)

	for _, val := range []string{"False", "FALSE", "Off", "NO", "no"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv("FAUCET_TELEMETRY", val)
			store := newMockStore()
			tracker := New(context.Background(), store, func() Properties { return Properties{} })
			if tracker != nil {
				t.Fatalf("expected nil tracker when FAUCET_TELEMETRY=%s", val)
			}
		})
	}
}

func TestNew_EnabledByDefault(t *testing.T) {
	setTestKey(t)
	store := newMockStore()
	tracker := New(context.Background(), store, func() Properties { return Properties{} })
	if tracker == nil {
		t.Fatal("expected non-nil tracker when telemetry is enabled by default")
	}
}

func TestTracker_InstanceIDPersisted(t *testing.T) {
	setTestKey(t)
	store := newMockStore()
	tracker := New(context.Background(), store, func() Properties {
		return Properties{
			Version:   "0.1.2",
			GoVersion: "go1.25.0",
			OS:        "linux",
			Arch:      "amd64",
			DBTypes:   []string{"postgres"},
			Services:  1,
			Tables:    10,
		}
	})

	if tracker.instanceID == "" {
		t.Fatal("expected non-empty instance ID")
	}

	// Verify the instance ID was persisted
	id, err := store.GetSetting(context.Background(), "instance_id")
	if err != nil {
		t.Fatalf("instance_id not persisted: %v", err)
	}
	if id != tracker.instanceID {
		t.Errorf("persisted ID %q != tracker ID %q", id, tracker.instanceID)
	}
}

func TestTracker_StartShutdown(t *testing.T) {
	setTestKey(t)
	store := newMockStore()
	tracker := New(context.Background(), store, func() Properties {
		return Properties{Version: "test"}
	})

	// Test that Start/Shutdown complete without hanging or panicking.
	// The flush will attempt to POST to PostHog which will fail silently
	// (3s timeout), but the goroutine lifecycle should be clean.
	tracker.Start()
	time.Sleep(100 * time.Millisecond)
	tracker.Shutdown()
}

func TestStartShutdown_NilTracker(t *testing.T) {
	// Ensure nil tracker doesn't panic
	var tracker *Tracker
	tracker.Start()
	tracker.Shutdown()
}
