package connstr

import (
	"net/url"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	mssql "github.com/microsoft/go-mssqldb/msdsn"
	"github.com/snowflakedb/gosnowflake"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
)

// A password that breaks naive string concatenation in every DSN format.
const hardPassword = `p@ss:w/rd#1?&=%(x)`

func TestBuild_Formats(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		fields model.ConnectionFields
		want   string
	}{
		{
			name:   "postgres full",
			driver: "postgres",
			fields: model.ConnectionFields{Host: "db.local", Port: 6543, User: "app", Password: "secret", Database: "orders", Params: map[string]string{"sslmode": "disable"}},
			want:   "postgres://app:secret@db.local:6543/orders?sslmode=disable",
		},
		{
			name:   "postgres default port, no db",
			driver: "postgres",
			fields: model.ConnectionFields{Host: "localhost", User: "app"},
			want:   "postgres://app@localhost:5432",
		},
		{
			name:   "postgres ipv6 host",
			driver: "postgres",
			fields: model.ConnectionFields{Host: "::1", User: "app", Database: "d"},
			want:   "postgres://app@[::1]:5432/d",
		},
		{
			name:   "mysql",
			driver: "mysql",
			fields: model.ConnectionFields{Host: "localhost", User: "root", Password: "pw", Database: "shop"},
			want:   "root:pw@tcp(localhost:3306)/shop",
		},
		{
			name:   "mssql with database",
			driver: "mssql",
			fields: model.ConnectionFields{Host: "sql1", User: "sa", Password: "pw", Database: "crm"},
			want:   "sqlserver://sa:pw@sql1:1433?database=crm",
		},
		{
			name:   "oracle",
			driver: "oracle",
			fields: model.ConnectionFields{Host: "ora", User: "scott", Password: "tiger", Database: "XEPDB1"},
			want:   "oracle://scott:tiger@ora:1521/XEPDB1",
		},
		{
			name:   "snowflake password auth",
			driver: "snowflake",
			fields: model.ConnectionFields{Host: "org-acct", User: "bob", Password: "pw", Database: "DB/PUBLIC", Params: map[string]string{"warehouse": "WH"}},
			want:   "bob:pw@org-acct/DB/PUBLIC?warehouse=WH",
		},
		{
			name:   "snowflake key-pair auth, no password",
			driver: "snowflake",
			fields: model.ConnectionFields{Host: "org-acct", User: "bob", Database: "DB"},
			want:   "bob@org-acct/DB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Build(tt.driver, tt.fields)
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Build = %q, want %q", got, tt.want)
			}
		})
	}
}

// Round-trip each built DSN through the real driver parser and check the
// password and database come back byte-for-byte. This is the property that
// matters: special characters must survive.
func TestBuild_SpecialCharactersRoundTrip(t *testing.T) {
	f := model.ConnectionFields{Host: "db.local", User: "us@er", Password: hardPassword, Database: "my db"}

	t.Run("postgres", func(t *testing.T) {
		dsn, err := Build("postgres", f)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatalf("pgx.ParseConfig(%q): %v", dsn, err)
		}
		if cfg.User != f.User || cfg.Password != f.Password || cfg.Database != f.Database || cfg.Host != "db.local" || cfg.Port != 5432 {
			t.Errorf("round trip mismatch: user=%q pass=%q db=%q host=%q port=%d", cfg.User, cfg.Password, cfg.Database, cfg.Host, cfg.Port)
		}
		// SanitizeDSN runs on every stored DSN; it must not corrupt ours.
		if again := connector.SanitizeDSN("postgres", dsn); again != dsn {
			t.Errorf("SanitizeDSN changed built DSN:\n got %q\nwant %q", again, dsn)
		}
	})

	t.Run("mysql", func(t *testing.T) {
		dsn, err := Build("mysql", f)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			t.Fatalf("mysql.ParseDSN(%q): %v", dsn, err)
		}
		if cfg.User != f.User || cfg.Passwd != f.Password || cfg.DBName != f.Database || cfg.Addr != "db.local:3306" || cfg.Net != "tcp" {
			t.Errorf("round trip mismatch: user=%q pass=%q db=%q addr=%q net=%q", cfg.User, cfg.Passwd, cfg.DBName, cfg.Addr, cfg.Net)
		}
		if again := connector.SanitizeDSN("mysql", dsn); again != dsn {
			t.Errorf("SanitizeDSN changed built DSN:\n got %q\nwant %q", again, dsn)
		}
	})

	t.Run("mssql", func(t *testing.T) {
		dsn, err := Build("mssql", f)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := mssql.Parse(dsn)
		if err != nil {
			t.Fatalf("msdsn.Parse(%q): %v", dsn, err)
		}
		if cfg.User != f.User || cfg.Password != f.Password || cfg.Database != f.Database || cfg.Host != "db.local" || cfg.Port != 1433 {
			t.Errorf("round trip mismatch: user=%q pass=%q db=%q host=%q port=%d", cfg.User, cfg.Password, cfg.Database, cfg.Host, cfg.Port)
		}
		if again := connector.SanitizeDSN("mssql", dsn); again != dsn {
			t.Errorf("SanitizeDSN changed built DSN:\n got %q\nwant %q", again, dsn)
		}
	})

	t.Run("oracle", func(t *testing.T) {
		dsn, err := Build("oracle", f)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", dsn, err)
		}
		pw, _ := u.User.Password()
		if u.User.Username() != f.User || pw != f.Password || strings.TrimPrefix(u.Path, "/") != f.Database {
			t.Errorf("round trip mismatch: user=%q pass=%q path=%q", u.User.Username(), pw, u.Path)
		}
		if again := connector.SanitizeDSN("oracle", dsn); again != dsn {
			t.Errorf("SanitizeDSN changed built DSN:\n got %q\nwant %q", again, dsn)
		}
	})

	t.Run("snowflake", func(t *testing.T) {
		sf := model.ConnectionFields{Host: "org-acct", User: "us@er", Password: hardPassword, Database: "DB/PUBLIC", Params: map[string]string{"warehouse": "W H"}}
		dsn, err := Build("snowflake", sf)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := gosnowflake.ParseDSN(dsn)
		if err != nil {
			t.Fatalf("gosnowflake.ParseDSN(%q): %v", dsn, err)
		}
		if cfg.User != sf.User || cfg.Password != sf.Password || cfg.Database != "DB" || cfg.Schema != "PUBLIC" || cfg.Warehouse != "W H" || cfg.Account != "org-acct" {
			t.Errorf("round trip mismatch: user=%q pass=%q db=%q schema=%q wh=%q acct=%q", cfg.User, cfg.Password, cfg.Database, cfg.Schema, cfg.Warehouse, cfg.Account)
		}
	})
}

func TestBuild_Errors(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		fields  model.ConnectionFields
		wantErr string
	}{
		{"missing host", "postgres", model.ConnectionFields{User: "u"}, "host is required"},
		{"missing account", "snowflake", model.ConnectionFields{User: "u"}, "account is required"},
		{"bad port", "mysql", model.ConnectionFields{Host: "h", Port: 70000}, "port must be"},
		{"oracle needs service", "oracle", model.ConnectionFields{Host: "h"}, "service name"},
		{"sqlite unsupported", "sqlite", model.ConnectionFields{Host: "h"}, "not supported for sqlite"},
		{"unknown driver", "db2", model.ConnectionFields{Host: "h"}, "unsupported driver"},
		{"bad mysql param", "mysql", model.ConnectionFields{Host: "h", Params: map[string]string{"parseTime": "maybe"}}, "invalid mysql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Build(tt.driver, tt.fields)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Build error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Run("dsn only is unchanged", func(t *testing.T) {
		svc := &model.ServiceConfig{Driver: "postgres", DSN: "postgres://x@h/db"}
		if err := Resolve(svc); err != nil {
			t.Fatal(err)
		}
		if svc.DSN != "postgres://x@h/db" {
			t.Errorf("DSN = %q", svc.DSN)
		}
	})
	t.Run("fields build dsn and are cleared", func(t *testing.T) {
		svc := &model.ServiceConfig{Driver: "postgres", Connection: &model.ConnectionFields{Host: "h", User: "u", Password: "p", Database: "d"}}
		if err := Resolve(svc); err != nil {
			t.Fatal(err)
		}
		if svc.DSN != "postgres://u:p@h:5432/d" {
			t.Errorf("DSN = %q", svc.DSN)
		}
		if svc.Connection != nil {
			t.Error("Connection should be cleared after resolve")
		}
	})
	t.Run("both is an error", func(t *testing.T) {
		svc := &model.ServiceConfig{Driver: "postgres", DSN: "postgres://x@h/db", Connection: &model.ConnectionFields{Host: "h"}}
		if err := Resolve(svc); err == nil || !strings.Contains(err.Error(), "not both") {
			t.Errorf("err = %v, want 'not both'", err)
		}
	})
}

// SanitizeDSN runs when a service is created and again on the stored DSN at
// every server start. It must be idempotent, or a password with special
// characters works until the first restart and then fails authentication.
func TestSanitizeDSN_Idempotent(t *testing.T) {
	tests := []struct {
		driver string
		raw    string // as a user might type it
		pass   string // the password the database must receive
	}{
		{"postgres", "postgres://app:p@ss#1@db:5432/app", "p@ss#1"},
		{"postgres", "postgres://app:p%40ss%231@db:5432/app", "p@ss#1"}, // already encoded
		{"postgres", "postgres://app:100%@db:5432/app", "100%"},         // raw %, not an escape
		{"mssql", "sqlserver://sa:a/b#c@db:1433?database=x", "a/b#c"},
		{"oracle", "oracle://u:p%3Aw@db:1521/XE", "p:w"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			once := connector.SanitizeDSN(tt.driver, tt.raw)
			twice := connector.SanitizeDSN(tt.driver, once)
			if once != twice {
				t.Fatalf("not idempotent:\n once  %q\n twice %q", once, twice)
			}
			u, err := url.Parse(twice)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", twice, err)
			}
			if pw, _ := u.User.Password(); pw != tt.pass {
				t.Errorf("password = %q, want %q (dsn %q)", pw, tt.pass, twice)
			}
		})
	}
}
