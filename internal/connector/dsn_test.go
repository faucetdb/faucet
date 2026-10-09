package connector

import (
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/microsoft/go-mssqldb/msdsn"
	"github.com/snowflakedb/gosnowflake"

	"github.com/faucetdb/faucet/internal/model"
)

// A password that breaks naive string formatting in every DSN dialect.
const nastyPassword = `p@ss:w/rd#?%&=+ é`

func TestBuildDSN_PostgresRoundTripsThroughPgx(t *testing.T) {
	dsn, err := BuildDSN("postgres", model.ConnectionParams{
		Host: "db.example.com", Port: 6543, Database: "app", Username: "api user",
		Password: nastyPassword, SSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx rejected %q: %v", dsn, err)
	}
	if cfg.Host != "db.example.com" || cfg.Port != 6543 || cfg.Database != "app" ||
		cfg.User != "api user" || cfg.Password != nastyPassword {
		t.Errorf("unexpected parse of %q: %+v", dsn, cfg.Config)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("sslmode missing from %q", dsn)
	}
}

func TestBuildDSN_PostgresDefaultsPort(t *testing.T) {
	dsn, err := BuildDSN("postgres", model.ConnectionParams{Host: "localhost", Database: "app", Username: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "postgres://u@localhost:5432/app" {
		t.Errorf("got %q", dsn)
	}
}

func TestBuildDSN_MySQLRoundTripsThroughDriver(t *testing.T) {
	dsn, err := BuildDSN("mysql", model.ConnectionParams{
		Host: "10.0.0.5", Database: "shop", Username: "root", Password: nastyPassword, SSLMode: "skip-verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("mysql driver rejected %q: %v", dsn, err)
	}
	if cfg.Addr != "10.0.0.5:3306" || cfg.DBName != "shop" || cfg.User != "root" ||
		cfg.Passwd != nastyPassword || !cfg.ParseTime || cfg.TLSConfig != "skip-verify" {
		t.Errorf("unexpected parse of %q: %+v", dsn, cfg)
	}
}

func TestBuildDSN_MSSQLRoundTripsThroughDriver(t *testing.T) {
	dsn, err := BuildDSN("mssql", model.ConnectionParams{
		Host: "sql.local", Database: "Sales", Username: "sa", Password: nastyPassword, SSLMode: "true",
		Options: map[string]string{"TrustServerCertificate": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := msdsn.Parse(dsn)
	if err != nil {
		t.Fatalf("mssql driver rejected %q: %v", dsn, err)
	}
	if cfg.Host != "sql.local" || cfg.Port != 1433 || cfg.Database != "Sales" ||
		cfg.User != "sa" || cfg.Password != nastyPassword {
		t.Errorf("unexpected parse of %q: %+v", dsn, cfg)
	}
}

func TestBuildDSN_OracleRoundTrips(t *testing.T) {
	dsn, err := BuildDSN("oracle", model.ConnectionParams{
		Host: "ora.local", Database: "XEPDB1", Username: "app", Password: nastyPassword, SSLMode: "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dsn, "oracle://") {
		t.Fatalf("got %q", dsn)
	}
	p, err := parseDSN("oracle", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if p.Host != "ora.local" || p.Port != 1521 || p.Database != "XEPDB1" || p.Username != "app" ||
		p.Password != nastyPassword || p.SSLMode != "true" {
		t.Errorf("unexpected parse of %q: %+v", dsn, p)
	}
}

func TestBuildDSN_SnowflakeRoundTripsThroughDriver(t *testing.T) {
	dsn, err := BuildDSN("snowflake", model.ConnectionParams{
		Account: "myorg-acct", Username: "ANALYST", Password: nastyPassword,
		Database: "ANALYTICS", Schema: "PUBLIC", Warehouse: "COMPUTE_WH", Role: "READER",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gosnowflake.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("snowflake driver rejected %q: %v", dsn, err)
	}
	if cfg.Account != "myorg-acct" || cfg.User != "ANALYST" || cfg.Password != nastyPassword ||
		cfg.Database != "ANALYTICS" || cfg.Schema != "PUBLIC" || cfg.Warehouse != "COMPUTE_WH" || cfg.Role != "READER" {
		t.Errorf("unexpected parse of %q: %+v", dsn, cfg)
	}
}

func TestBuildDSN_SnowflakeKeyPairHasNoPassword(t *testing.T) {
	dsn, err := BuildDSN("snowflake", model.ConnectionParams{
		Account: "myorg-acct", Username: "SVC", Database: "DB", Schema: "S", Warehouse: "WH",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "SVC@myorg-acct/DB/S?warehouse=WH" {
		t.Errorf("got %q", dsn)
	}
	// The JWT path in the snowflake connector must still be able to parse it.
	if _, err := gosnowflake.ParseDSN(strings.Replace(dsn, "@", ":_@", 1)); err != nil {
		t.Errorf("snowflake driver rejected %q: %v", dsn, err)
	}
}

func TestBuildDSN_SQLite(t *testing.T) {
	dsn, err := BuildDSN("sqlite", model.ConnectionParams{Path: " /data/app.db "})
	if err != nil || dsn != "/data/app.db" {
		t.Errorf("got %q, %v", dsn, err)
	}
}

func TestBuildDSN_Validation(t *testing.T) {
	cases := []struct {
		driver string
		p      model.ConnectionParams
		want   string
	}{
		{"postgres", model.ConnectionParams{}, "host is required"},
		{"mysql", model.ConnectionParams{Host: "h", Port: 70000}, "port must be"},
		{"oracle", model.ConnectionParams{Host: "h"}, "service name is required"},
		{"sqlite", model.ConnectionParams{}, "file path is required"},
		{"snowflake", model.ConnectionParams{Username: "u"}, "account identifier is required"},
		{"snowflake", model.ConnectionParams{Account: "a"}, "username is required"},
		{"db2", model.ConnectionParams{}, "unsupported driver"},
	}
	for _, c := range cases {
		_, err := BuildDSN(c.driver, c.p)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %+v: got %v, want %q", c.driver, c.p, err, c.want)
		}
	}
}

func TestDescribeDSN_NeverReturnsPassword(t *testing.T) {
	cases := map[string]string{
		"postgres":  "postgres://alice:secret@db:5432/app?sslmode=disable",
		"mysql":     "alice:secret@tcp(db:3306)/app?parseTime=true",
		"mssql":     "sqlserver://alice:secret@db:1433?database=app",
		"oracle":    "oracle://alice:secret@db:1521/app",
		"snowflake": "alice:secret@acct/app/PUBLIC?warehouse=WH",
	}
	for driver, dsn := range cases {
		p := DescribeDSN(driver, dsn)
		if p.Password != "" {
			t.Errorf("%s: password leaked", driver)
		}
		if p.Username != "alice" || p.Database != "app" {
			t.Errorf("%s: got %+v", driver, p)
		}
		if driver != "snowflake" && (p.Host != "db" || p.Port == 0) {
			t.Errorf("%s: host/port not parsed: %+v", driver, p)
		}
	}
	if p := DescribeDSN("postgres", "host=db port=5433 user=alice password=secret dbname=app"); p.Host != "db" || p.Port != 5433 || p.Password != "" {
		t.Errorf("key/value form: got %+v", p)
	}
	if p := DescribeDSN("sqlite", "/data/app.db?_pragma=foreign_keys(1)"); p.Path != "/data/app.db" {
		t.Errorf("sqlite: got %+v", p)
	}
}

func TestMergeStoredPassword(t *testing.T) {
	existing, _ := BuildDSN("mysql", model.ConnectionParams{Host: "db", Username: "u", Password: nastyPassword})
	merged := MergeStoredPassword("mysql", existing, model.ConnectionParams{Host: "db2", Username: "u"})
	if merged.Password != nastyPassword || merged.Host != "db2" {
		t.Errorf("got %+v", merged)
	}
	kept := MergeStoredPassword("mysql", existing, model.ConnectionParams{Password: "new"})
	if kept.Password != "new" {
		t.Errorf("explicit password overwritten: %+v", kept)
	}
}
