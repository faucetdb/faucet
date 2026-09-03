package cli

import (
	"bytes"
	"context"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/faucetdb/faucet/internal/config"
)

func resetViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
}

func TestResolveJWTSecret_ViperValueWins(t *testing.T) {
	resetViper(t)
	viper.Set("auth.jwt_secret", "configured-secret")

	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	// A different stored value must be ignored when config/env is set.
	if err := store.SetSetting(context.Background(), jwtSecretSettingKey, "stored-secret"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	got, err := resolveJWTSecret(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("resolveJWTSecret: %v", err)
	}
	if got != "configured-secret" {
		t.Errorf("got %q, want configured value", got)
	}
}

func TestResolveJWTSecret_EnvAliases(t *testing.T) {
	for _, name := range []string{"FAUCET_AUTH_JWT_SECRET", "FAUCET_JWT_SECRET"} {
		t.Run(name, func(t *testing.T) {
			resetViper(t)
			t.Setenv("FAUCET_AUTH_JWT_SECRET", "")
			t.Setenv("FAUCET_JWT_SECRET", "")
			t.Setenv(name, "from-"+name)
			initConfig()

			store, err := config.NewStore("")
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
			defer store.Close()

			got, err := resolveJWTSecret(context.Background(), store, nil)
			if err != nil {
				t.Fatalf("resolveJWTSecret: %v", err)
			}
			if got != "from-"+name {
				t.Errorf("got %q, want value from %s", got, name)
			}
		})
	}
}

func TestResolveJWTSecret_GeneratedAndPersisted(t *testing.T) {
	resetViper(t)
	t.Setenv("FAUCET_AUTH_JWT_SECRET", "")
	t.Setenv("FAUCET_JWT_SECRET", "")

	store, err := config.NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	first, err := resolveJWTSecret(context.Background(), store, logger)
	if err != nil {
		t.Fatalf("first resolveJWTSecret: %v", err)
	}
	if first == "" || first == "faucet-dev-secret-change-me" {
		t.Fatalf("expected a generated secret, got %q", first)
	}
	raw, err := hex.DecodeString(first)
	if err != nil || len(raw) != jwtSecretBytes {
		t.Errorf("secret should be %d hex-encoded random bytes, got %q (err=%v)", jwtSecretBytes, first, err)
	}
	if !strings.Contains(buf.String(), "generated a new JWT signing secret") {
		t.Errorf("expected INFO log about generation, got %q", buf.String())
	}

	stored, err := store.GetSetting(context.Background(), jwtSecretSettingKey)
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if stored != first {
		t.Errorf("persisted secret %q != returned %q", stored, first)
	}

	buf.Reset()
	second, err := resolveJWTSecret(context.Background(), store, logger)
	if err != nil {
		t.Fatalf("second resolveJWTSecret: %v", err)
	}
	if second != first {
		t.Errorf("secret not stable across calls: %q vs %q", first, second)
	}
	if strings.Contains(buf.String(), "generated") {
		t.Errorf("second call should not generate again, got log %q", buf.String())
	}

	// Two independent stores generate different secrets.
	other, err := config.NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer other.Close()
	third, err := resolveJWTSecret(context.Background(), other, nil)
	if err != nil {
		t.Fatalf("resolveJWTSecret on other store: %v", err)
	}
	if third == first {
		t.Error("two stores produced the same generated secret")
	}
}

func TestResolveJWTSecret_NilStore(t *testing.T) {
	resetViper(t)
	t.Setenv("FAUCET_AUTH_JWT_SECRET", "")
	t.Setenv("FAUCET_JWT_SECRET", "")
	if _, err := resolveJWTSecret(context.Background(), nil, nil); err == nil {
		t.Error("expected error with nil store and no configured secret")
	}
}
