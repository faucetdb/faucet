package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/faucetdb/faucet/internal/config"
)

func TestDBAdd_ConnectionFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FAUCET_DATA_DIR", dir)

	cmd := newDBAddCmd()
	cmd.SetArgs([]string{
		"--name", "shop", "--driver", "mysql",
		"--host", "db.internal", "--port", "3307",
		"--user", "root", "--password", "p@ss:w/rd#1",
		"--database", "shop", "--param", "parseTime=true",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("db add: %v", err)
	}

	store, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc, err := store.GetServiceByName(context.Background(), "shop")
	if err != nil {
		t.Fatal(err)
	}
	if want := "root:p@ss:w/rd#1@tcp(db.internal:3307)/shop?parseTime=true"; svc.DSN != want {
		t.Errorf("DSN = %q, want %q", svc.DSN, want)
	}
}

func TestDBAdd_ConnectionFieldErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"dsn and host", []string{"--name", "a", "--driver", "postgres", "--dsn", "postgres://h/d", "--host", "h"}, "none of the others"},
		{"dsn and user", []string{"--name", "a", "--driver", "postgres", "--dsn", "postgres://h/d", "--user", "u"}, "not both"},
		{"fields without host", []string{"--name", "a", "--driver", "postgres", "--user", "u"}, "host is required"},
		{"password and prompt", []string{"--name", "a", "--driver", "postgres", "--host", "h", "--password", "x", "--password-prompt"}, "none of the others"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FAUCET_DATA_DIR", t.TempDir())
			cmd := newDBAddCmd()
			cmd.SetArgs(tt.args)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
