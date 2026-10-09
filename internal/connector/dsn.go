package connector

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	go_ora "github.com/sijms/go-ora/v2"
	"github.com/snowflakedb/gosnowflake"

	"github.com/faucetdb/faucet/internal/model"
)

// DefaultPort returns the conventional TCP port for a driver, or 0 when the
// driver has no port (sqlite, snowflake).
func DefaultPort(driver string) int {
	switch driver {
	case "postgres":
		return 5432
	case "mysql":
		return 3306
	case "mssql":
		return 1433
	case "oracle":
		return 1521
	default:
		return 0
	}
}

// BuildDSN turns individual connection fields into the connection string the
// driver expects, escaping credentials as each format requires. Users never
// have to know that MySQL wants tcp(host:port) or that a "@" in a Postgres
// password must be percent-encoded.
func BuildDSN(driver string, p model.ConnectionParams) (string, error) {
	switch driver {
	case "postgres":
		return buildPostgresDSN(p)
	case "mysql":
		return buildMySQLDSN(p)
	case "mssql":
		return buildMSSQLDSN(p)
	case "oracle":
		return buildOracleDSN(p)
	case "sqlite":
		return buildSQLiteDSN(p)
	case "snowflake":
		return buildSnowflakeDSN(p)
	default:
		return "", fmt.Errorf("unsupported driver %q", driver)
	}
}

func hostPort(driver string, p model.ConnectionParams) (string, error) {
	host := strings.TrimSpace(p.Host)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	port := p.Port
	if port == 0 {
		port = DefaultPort(driver)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// sortedQuery encodes options in a stable order so the same input always
// yields the same DSN.
func sortedQuery(opts map[string]string) url.Values {
	q := url.Values{}
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k != "" {
			q.Set(k, opts[k])
		}
	}
	return q
}

func userInfo(p model.ConnectionParams) *url.Userinfo {
	if p.Username == "" {
		return nil
	}
	if p.Password == "" {
		return url.User(p.Username)
	}
	return url.UserPassword(p.Username, p.Password)
}

func buildPostgresDSN(p model.ConnectionParams) (string, error) {
	hp, err := hostPort("postgres", p)
	if err != nil {
		return "", err
	}
	q := sortedQuery(p.Options)
	if p.SSLMode != "" {
		q.Set("sslmode", p.SSLMode)
	}
	u := url.URL{Scheme: "postgres", User: userInfo(p), Host: hp, RawQuery: q.Encode()}
	if p.Database != "" {
		u.Path = "/" + p.Database
	}
	return u.String(), nil
}

func buildMySQLDSN(p model.ConnectionParams) (string, error) {
	hp, err := hostPort("mysql", p)
	if err != nil {
		return "", err
	}
	cfg := mysqldriver.NewConfig()
	cfg.User = p.Username
	cfg.Passwd = p.Password
	cfg.Net = "tcp"
	cfg.Addr = hp
	cfg.DBName = p.Database
	cfg.ParseTime = true
	if p.SSLMode != "" && p.SSLMode != "false" {
		cfg.TLSConfig = p.SSLMode
	}
	if len(p.Options) > 0 {
		cfg.Params = map[string]string{}
		for k, v := range p.Options {
			cfg.Params[k] = v
		}
	}
	return cfg.FormatDSN(), nil
}

func buildMSSQLDSN(p model.ConnectionParams) (string, error) {
	hp, err := hostPort("mssql", p)
	if err != nil {
		return "", err
	}
	q := sortedQuery(p.Options)
	if p.Database != "" {
		q.Set("database", p.Database)
	}
	if p.SSLMode != "" {
		q.Set("encrypt", p.SSLMode)
	}
	u := url.URL{Scheme: "sqlserver", User: userInfo(p), Host: hp, RawQuery: q.Encode()}
	return u.String(), nil
}

func buildOracleDSN(p model.ConnectionParams) (string, error) {
	if strings.TrimSpace(p.Host) == "" {
		return "", fmt.Errorf("host is required")
	}
	if p.Database == "" {
		return "", fmt.Errorf("service name is required")
	}
	port := p.Port
	if port == 0 {
		port = DefaultPort("oracle")
	}
	opts := map[string]string{}
	for k, v := range p.Options {
		opts[k] = v
	}
	if p.SSLMode == "true" {
		opts["SSL"] = "enable"
	}
	return go_ora.BuildUrl(strings.TrimSpace(p.Host), port, p.Database, p.Username, p.Password, opts), nil
}

func buildSQLiteDSN(p model.ConnectionParams) (string, error) {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}
	if len(p.Options) == 0 {
		return path, nil
	}
	return path + "?" + sortedQuery(p.Options).Encode(), nil
}

func buildSnowflakeDSN(p model.ConnectionParams) (string, error) {
	account := strings.TrimSpace(p.Account)
	if account == "" {
		return "", fmt.Errorf("account identifier is required")
	}
	if p.Username == "" {
		return "", fmt.Errorf("username is required")
	}
	q := sortedQuery(p.Options)
	if p.Warehouse != "" {
		q.Set("warehouse", p.Warehouse)
	}
	if p.Role != "" {
		q.Set("role", p.Role)
	}
	// Both password and key-pair auth use the user[:password]@account/db/schema
	// form. Key-pair auth (private_key_path on the service) has no password;
	// the Snowflake connector injects the key at connect time.
	var b strings.Builder
	b.WriteString(url.QueryEscape(p.Username))
	if p.Password != "" {
		b.WriteString(":")
		b.WriteString(url.QueryEscape(p.Password))
	}
	b.WriteString("@")
	b.WriteString(account)
	if p.Database != "" {
		b.WriteString("/" + url.PathEscape(p.Database))
		if p.Schema != "" {
			b.WriteString("/" + url.PathEscape(p.Schema))
		}
	}
	if enc := q.Encode(); enc != "" {
		b.WriteString("?" + enc)
	}
	return b.String(), nil
}

// DescribeDSN extracts the non-secret connection fields from a stored DSN so
// the admin UI can show "db.example.com:5432/app" and pre-fill the edit form.
// The password is never returned. Unparseable DSNs yield empty params.
func DescribeDSN(driver, dsn string) model.ConnectionParams {
	p, _ := parseDSN(driver, dsn)
	p.Password = ""
	return p
}

// MergeStoredPassword fills an empty password in p from an existing DSN, so
// editing a connection without re-typing the password keeps it.
func MergeStoredPassword(driver, existingDSN string, p model.ConnectionParams) model.ConnectionParams {
	if p.Password != "" || existingDSN == "" {
		return p
	}
	if old, err := parseDSN(driver, existingDSN); err == nil {
		p.Password = old.Password
	}
	return p
}

func parseDSN(driver, dsn string) (model.ConnectionParams, error) {
	var p model.ConnectionParams
	switch driver {
	case "postgres":
		if !strings.Contains(dsn, "://") {
			return parsePostgresKeyValue(dsn), nil
		}
		u, err := url.Parse(dsn)
		if err != nil {
			return p, err
		}
		fillFromURL(&p, u)
		p.Database = strings.TrimPrefix(u.Path, "/")
		p.SSLMode = u.Query().Get("sslmode")
	case "mssql":
		u, err := url.Parse(dsn)
		if err != nil {
			return p, err
		}
		fillFromURL(&p, u)
		p.Database = u.Query().Get("database")
		p.SSLMode = u.Query().Get("encrypt")
		if v := u.Query().Get("TrustServerCertificate"); v != "" {
			p.Options = map[string]string{"TrustServerCertificate": v}
		}
	case "oracle":
		u, err := url.Parse(dsn)
		if err != nil {
			return p, err
		}
		fillFromURL(&p, u)
		p.Database = strings.TrimPrefix(u.Path, "/")
		if strings.EqualFold(u.Query().Get("SSL"), "enable") || strings.EqualFold(u.Query().Get("ssl"), "true") {
			p.SSLMode = "true"
		}
	case "mysql":
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			return p, err
		}
		p.Username = cfg.User
		p.Password = cfg.Passwd
		p.Database = cfg.DBName
		p.SSLMode = cfg.TLSConfig
		if host, port, err := net.SplitHostPort(cfg.Addr); err == nil {
			p.Host = host
			p.Port, _ = strconv.Atoi(port)
		} else {
			p.Host = cfg.Addr
		}
	case "sqlite":
		p.Path, _, _ = strings.Cut(dsn, "?")
	case "snowflake":
		parseable := dsn
		if at := strings.LastIndex(dsn, "@"); at > 0 && !strings.Contains(dsn[:at], ":") {
			parseable = dsn[:at] + ":_" + dsn[at:]
		}
		cfg, err := gosnowflake.ParseDSN(parseable)
		if err != nil {
			return p, err
		}
		p.Account = cfg.Account
		p.Username = cfg.User
		if parseable == dsn {
			p.Password = cfg.Password
		}
		p.Database = cfg.Database
		p.Schema = cfg.Schema
		p.Warehouse = cfg.Warehouse
		p.Role = cfg.Role
	default:
		return p, fmt.Errorf("unsupported driver %q", driver)
	}
	return p, nil
}

func fillFromURL(p *model.ConnectionParams, u *url.URL) {
	p.Host = u.Hostname()
	p.Port, _ = strconv.Atoi(u.Port())
	if u.User != nil {
		p.Username = u.User.Username()
		p.Password, _ = u.User.Password()
	}
}

// parsePostgresKeyValue handles the libpq "host=... port=... user=..." form.
// Quoted values are not supported; the URL form is what Faucet writes.
func parsePostgresKeyValue(dsn string) model.ConnectionParams {
	var p model.ConnectionParams
	for _, field := range strings.Fields(dsn) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch k {
		case "host":
			p.Host = v
		case "port":
			p.Port, _ = strconv.Atoi(v)
		case "dbname":
			p.Database = v
		case "user":
			p.Username = v
		case "password":
			p.Password = v
		case "sslmode":
			p.SSLMode = v
		}
	}
	return p
}
