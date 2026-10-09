package connector

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSanitizeDSN_URLStyle(t *testing.T) {
	cases := []struct {
		name, in, wantPass string
	}{
		{"raw special characters", "postgres://u:p@ss#1@db:5432/app", "p@ss#1"},
		{"already encoded is not double-encoded", "postgres://u:p%40ss%231@db:5432/app", "p@ss#1"},
		{"stray percent kept literally", "postgres://u:100%zz@db:5432/app", "100%zz"},
		{"plain", "postgres://u:secret@db:5432/app?sslmode=disable", "secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := SanitizeDSN("postgres", c.in)
			cfg, err := pgx.ParseConfig(out)
			if err != nil {
				t.Fatalf("pgx rejected %q: %v", out, err)
			}
			if cfg.Password != c.wantPass || cfg.Host != "db" || cfg.Database != "app" {
				t.Errorf("SanitizeDSN(%q) = %q -> password %q host %q db %q", c.in, out, cfg.Password, cfg.Host, cfg.Database)
			}
		})
	}
}

func TestSanitizeDSN_UserOnly(t *testing.T) {
	if got := SanitizeDSN("postgres", "postgres://alice@db/app"); got != "postgres://alice@db/app" {
		t.Errorf("got %q", got)
	}
	if got := SanitizeDSN("postgres", "postgres://db/app"); got != "postgres://db/app" {
		t.Errorf("got %q", got)
	}
}
