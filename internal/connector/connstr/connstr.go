// Package connstr builds driver-specific connection strings (DSNs) from
// individual connection fields such as host, port, user, and password.
package connstr

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/faucetdb/faucet/internal/model"
)

// defaultPorts holds the port used when ConnectionFields.Port is zero.
var defaultPorts = map[string]int{
	"postgres": 5432,
	"mysql":    3306,
	"mssql":    1433,
	"oracle":   1521,
}

// Build assembles a driver-specific connection string from individual
// connection fields (host, port, user, password, database, params). It lets
// callers configure a service without hand-writing a DSN. User, password,
// and database values are escaped for the target format, so any character
// is safe in a password.
func Build(driver string, f model.ConnectionFields) (string, error) {
	host := strings.TrimSpace(f.Host)
	if host == "" {
		if driver == "snowflake" {
			return "", fmt.Errorf("account is required")
		}
		return "", fmt.Errorf("host is required")
	}
	if f.Port < 0 || f.Port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}

	switch driver {
	case "postgres":
		return buildURLDSN("postgres", host, portOrDefault(driver, f.Port), f, "/"+f.Database, nil), nil
	case "oracle":
		if f.Database == "" {
			return "", fmt.Errorf("database (service name) is required for oracle")
		}
		return buildURLDSN("oracle", host, portOrDefault(driver, f.Port), f, "/"+f.Database, nil), nil
	case "mssql":
		var extra url.Values
		if f.Database != "" {
			extra = url.Values{"database": {f.Database}}
		}
		return buildURLDSN("sqlserver", host, portOrDefault(driver, f.Port), f, "", extra), nil
	case "mysql":
		return buildMySQLDSN(host, portOrDefault(driver, f.Port), f)
	case "snowflake":
		return buildSnowflakeDSN(host, f), nil
	case "sqlite":
		return "", fmt.Errorf("connection fields are not supported for sqlite; set dsn to the database file path")
	default:
		return "", fmt.Errorf("unsupported driver %q", driver)
	}
}

// Resolve fills svc.DSN from svc.Connection when the caller gave
// individual fields instead of a connection string, then clears
// svc.Connection so the plaintext fields are not kept around. Giving both a
// DSN and connection fields is an error, because it is not clear which one
// should win. When neither is set, svc is left unchanged.
func Resolve(svc *model.ServiceConfig) error {
	if svc.Connection == nil {
		return nil
	}
	defer func() { svc.Connection = nil }()
	if strings.TrimSpace(svc.DSN) != "" {
		return fmt.Errorf("provide either dsn or connection fields, not both")
	}
	dsn, err := Build(svc.Driver, *svc.Connection)
	if err != nil {
		return err
	}
	svc.DSN = dsn
	return nil
}

func portOrDefault(driver string, port int) int {
	if port == 0 {
		return defaultPorts[driver]
	}
	return port
}

// buildURLDSN builds scheme://user:pass@host:port/path?params. url.URL does
// the percent-encoding of the userinfo, path, and query.
func buildURLDSN(scheme, host string, port int, f model.ConnectionFields, path string, extra url.Values) string {
	u := url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}
	if path != "/" {
		u.Path = path
	}
	if f.User != "" {
		if f.Password != "" {
			u.User = url.UserPassword(f.User, f.Password)
		} else {
			u.User = url.User(f.User)
		}
	}
	q := url.Values{}
	for k, v := range extra {
		q[k] = v
	}
	for k, v := range f.Params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// buildMySQLDSN builds user:pass@tcp(host:port)/db?params and validates the
// result with the driver's own parser so bad params fail early.
func buildMySQLDSN(host string, port int, f model.ConnectionFields) (string, error) {
	cfg := mysqldriver.NewConfig()
	cfg.User = f.User
	cfg.Passwd = f.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	cfg.DBName = f.Database
	dsn := cfg.FormatDSN()

	if len(f.Params) > 0 {
		q := url.Values{}
		for k, v := range f.Params {
			q.Set(k, v)
		}
		sep := "?"
		if strings.Contains(dsn[strings.LastIndex(dsn, "/"):], "?") {
			sep = "&"
		}
		dsn += sep + q.Encode()
	}

	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid mysql connection fields: %w", err)
	}
	return parsed.FormatDSN(), nil
}

// buildSnowflakeDSN builds user:pass@account[:port]/database?params. Host
// holds the Snowflake account identifier. An empty password produces
// user@account/..., which the Snowflake connector accepts for key-pair auth.
func buildSnowflakeDSN(account string, f model.ConnectionFields) string {
	var b strings.Builder
	if f.User != "" {
		b.WriteString(url.QueryEscape(f.User))
		if f.Password != "" {
			b.WriteByte(':')
			b.WriteString(url.QueryEscape(f.Password))
		}
		b.WriteByte('@')
	}
	b.WriteString(account)
	if f.Port != 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(f.Port))
	}
	if f.Database != "" {
		// Snowflake reads "db/schema" from the path; escape each segment
		// but keep the separator.
		for _, seg := range strings.Split(f.Database, "/") {
			b.WriteByte('/')
			b.WriteString(url.QueryEscape(seg))
		}
	}
	if len(f.Params) > 0 {
		keys := make([]string, 0, len(f.Params))
		for k := range f.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('?')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(f.Params[k]))
		}
	}
	return b.String()
}
