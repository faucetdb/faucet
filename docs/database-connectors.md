# Database Connectors

Faucet supports five SQL databases. Each database is connected as a **service** -- a named reference to a single database that becomes an API namespace.

All connectors implement the same `Connector` interface, which provides:

- Connection management (connect, disconnect, ping)
- Schema introspection (tables, columns, views, stored procedures)
- Query building (SELECT, INSERT, UPDATE, DELETE with dialect-specific SQL)
- Schema modification (CREATE TABLE, ALTER TABLE, DROP TABLE)
- Stored procedure execution

## PostgreSQL

**Driver name:** `postgres`

### DSN Format

```
postgres://username:password@host:port/database?sslmode=disable
```

Or the keyword/value format:

```
host=localhost port=5432 user=myuser password=mypass dbname=mydb sslmode=disable
```

### Common DSN Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `sslmode` | `require` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `search_path` | `public` | Default schema |
| `connect_timeout` | -- | Connection timeout in seconds |
| `application_name` | -- | Application name for `pg_stat_activity` |

### DSN Examples

```bash
# Local development
postgres://localhost/mydb?sslmode=disable

# Remote with SSL
postgres://user:pass@db.example.com:5432/production?sslmode=require

# With search path
postgres://user:pass@localhost/mydb?sslmode=disable&search_path=myschema

# Amazon RDS
postgres://admin:secret@mydb.abc123.us-east-1.rds.amazonaws.com:5432/mydb?sslmode=require
```

### Supported Features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | Yes |
| Upsert (ON CONFLICT) | Yes |
| Stored procedures | Yes |
| Views | Yes |
| Parameter style | `$1, $2, $3` |
| Default schema | `public` |

### Adding via CLI

```bash
faucet db add \
  --name mypostgres \
  --driver postgres \
  --dsn "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

---

## MySQL

**Driver name:** `mysql`

### DSN Format

```
username:password@tcp(host:port)/database?parseTime=true
```

### Common DSN Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `parseTime` | `false` | Parse `DATE`/`DATETIME` to `time.Time` (recommended: `true`) |
| `charset` | `utf8mb4` | Character set |
| `collation` | `utf8mb4_general_ci` | Collation |
| `tls` | `false` | TLS mode: `true`, `false`, `skip-verify`, or custom name |
| `timeout` | -- | Connection timeout (e.g., `5s`) |
| `readTimeout` | -- | I/O read timeout |
| `writeTimeout` | -- | I/O write timeout |
| `multiStatements` | `false` | Allow multiple statements in one query |

### DSN Examples

```bash
# Local development
root:password@tcp(localhost:3306)/mydb?parseTime=true

# Remote with TLS
user:pass@tcp(db.example.com:3306)/production?parseTime=true&tls=true

# With charset
user:pass@tcp(localhost:3306)/mydb?parseTime=true&charset=utf8mb4

# Amazon RDS
admin:secret@tcp(mydb.abc123.us-east-1.rds.amazonaws.com:3306)/mydb?parseTime=true&tls=true
```

### Supported Features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | No (MySQL < 8.0.21) |
| Upsert (ON DUPLICATE KEY) | Yes |
| Stored procedures | Yes |
| Views | Yes |
| Parameter style | `?` |
| Default schema | database name |

Note: Without RETURNING support, INSERT operations return the submitted records rather than the server-generated values. Use a subsequent GET to retrieve auto-incremented IDs.

### Adding via CLI

```bash
faucet db add \
  --name mymysql \
  --driver mysql \
  --dsn "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
```

---

## SQL Server

**Driver name:** `mssql`

### DSN Format

```
sqlserver://username:password@host:port?database=dbname
```

Or using ADO-style connection string:

```
server=host;port=1433;user id=username;password=password;database=dbname
```

### Common DSN Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `database` | -- | Database name |
| `encrypt` | `false` | Enable encryption: `true`, `false`, `disable` |
| `TrustServerCertificate` | `false` | Trust self-signed certificates |
| `app name` | -- | Application name |
| `connection timeout` | `0` | Connection timeout in seconds |

### DSN Examples

```bash
# Local development
sqlserver://sa:YourStrong!Passw0rd@localhost:1433?database=mydb

# Remote with encryption
sqlserver://user:pass@db.example.com:1433?database=production&encrypt=true

# Trust self-signed cert (development only)
sqlserver://sa:password@localhost:1433?database=mydb&encrypt=true&TrustServerCertificate=true

# Azure SQL
sqlserver://admin:secret@myserver.database.windows.net:1433?database=mydb&encrypt=true
```

### Supported Features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | Via OUTPUT clause |
| Upsert (MERGE) | Yes |
| Stored procedures | Yes |
| Views | Yes |
| Parameter style | `@p1, @p2, @p3` |
| Default schema | `dbo` |

### Adding via CLI

```bash
faucet db add \
  --name mymssql \
  --driver mssql \
  --dsn "sqlserver://sa:YourStrong!Passw0rd@localhost:1433?database=mydb"
```

---

## Snowflake

**Driver name:** `snowflake`

### DSN Format

```
account_identifier/database/schema?user=username&password=password&warehouse=wh
```

Or using the Snowflake Go driver format:

```
username:password@account_identifier/database/schema?warehouse=wh&role=role
```

### Common DSN Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `warehouse` | -- | Snowflake warehouse |
| `role` | -- | Snowflake role |
| `schema` | `PUBLIC` | Default schema |
| `authenticator` | -- | Auth type: `snowflake` (default), `externalbrowser`, `oauth`, `SNOWFLAKE_JWT` |
| `token` | -- | OAuth access token (when authenticator=oauth) |
| `loginTimeout` | `60` | Login timeout in seconds |

### DSN Examples

```bash
# Basic connection (username/password)
user:pass@myaccount/mydb/public?warehouse=COMPUTE_WH

# With role
user:pass@myaccount/mydb/public?warehouse=COMPUTE_WH&role=ANALYST

# Full URL format
user:pass@myorg-myaccount/mydb/PUBLIC?warehouse=COMPUTE_WH&role=DATA_READER
```

### Key Pair (JWT) Authentication

For environments where username/password is not permitted, Faucet supports Snowflake key pair authentication using RSA private keys.

**1. Generate an RSA key pair:**

```bash
# Generate PKCS#8 private key (recommended)
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out snowflake_key.p8

# Extract the public key
openssl pkey -in snowflake_key.p8 -pubout -out snowflake_key.pub
```

**2. Register the public key in Snowflake:**

```sql
ALTER USER MY_USER SET RSA_PUBLIC_KEY='MIIBIjANBg...';
```

(Paste the key contents without the `-----BEGIN/END PUBLIC KEY-----` headers.)

**3. Configure the service with `private_key_path`:**

```bash
# Via CLI — no password needed in the DSN
faucet db add \
  --name analytics \
  --driver snowflake \
  --dsn "MY_USER@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH" \
  --private-key-path /path/to/snowflake_key.p8
```

Via API:

```json
{
  "name": "analytics",
  "driver": "snowflake",
  "dsn": "MY_USER@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH",
  "private_key_path": "/path/to/snowflake_key.p8"
}
```

Via YAML config:

```yaml
services:
  - name: analytics
    driver: snowflake
    dsn: "MY_USER@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH"
    private_key_path: /path/to/snowflake_key.p8
```

Supported PEM formats: `PKCS#8` (`PRIVATE KEY`) and `PKCS#1` (`RSA PRIVATE KEY`).

### Supported Features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | No |
| Upsert | No |
| Stored procedures | Yes |
| Views | Yes |
| Key pair (JWT) auth | Yes |
| Parameter style | `?` |
| Default schema | `PUBLIC` |

### Adding via CLI

```bash
# Username/password
faucet db add \
  --name mysnowflake \
  --driver snowflake \
  --dsn "user:pass@myaccount/mydb/public?warehouse=COMPUTE_WH"

# Key pair authentication
faucet db add \
  --name mysnowflake \
  --driver snowflake \
  --dsn "user@myaccount/mydb/public?warehouse=COMPUTE_WH" \
  --private-key-path /path/to/key.p8
```

---

## SQLite

**Driver name:** `sqlite`

### DSN Format

```
/path/to/database.db?param=value
```

Or for an in-memory database:

```
:memory:
```

### Common DSN Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `_journal_mode` | `delete` | Set to `WAL` for better concurrent read performance |
| `_busy_timeout` | `5000` | Milliseconds to wait when the database is locked |
| `_foreign_keys` | `on` | Enable foreign key enforcement |
| `_cache_size` | `-2000` | Page cache size (negative = KB, positive = pages) |

### DSN Examples

```bash
# Local file
/var/lib/faucet/mydata.db

# With WAL mode (recommended)
/var/lib/faucet/mydata.db?_journal_mode=WAL

# With WAL and busy timeout
/var/lib/faucet/mydata.db?_journal_mode=WAL&_busy_timeout=10000

# In-memory (ephemeral, for testing)
:memory:
```

### Supported Features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | Yes (SQLite 3.35+) |
| Upsert (ON CONFLICT) | Yes |
| Stored procedures | No |
| Views | Yes |
| Parameter style | `?` |
| Default schema | `main` |

Note: SQLite does not support stored procedures. The `_proc` endpoint returns an error for SQLite services.

### Adding via CLI

```bash
faucet db add \
  --name mydata \
  --driver sqlite \
  --dsn "/path/to/mydata.db?_journal_mode=WAL"
```

---

## Connection Pool Configuration

Each service has its own connection pool. Configure it when creating the service via the API:

```json
{
  "name": "mydb",
  "driver": "postgres",
  "dsn": "postgres://...",
  "pool": {
    "max_open_conns": 25,
    "max_idle_conns": 5,
    "conn_max_lifetime": "5m",
    "conn_max_idle_time": "1m"
  }
}
```

### Pool Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `max_open_conns` | 25 | Maximum number of open connections to the database |
| `max_idle_conns` | 5 | Maximum number of idle connections in the pool |
| `conn_max_lifetime` | 5 minutes | Maximum time a connection can be reused |
| `conn_max_idle_time` | 1 minute | Maximum time a connection can sit idle |

### Tuning Guidelines

- **Small/dev:** `max_open_conns=10, max_idle_conns=2`
- **Medium production:** `max_open_conns=25, max_idle_conns=5` (defaults)
- **High traffic:** `max_open_conns=50-100, max_idle_conns=10-25`
- **Serverless databases (Snowflake, Aurora Serverless):** Lower `max_open_conns` to respect provider limits
- **SQLite:** `max_open_conns=1, max_idle_conns=1` (SQLite supports only one writer at a time)

Keep `conn_max_lifetime` shorter than the database server's connection timeout to avoid stale connections.

---

## Schema Exposure

By default, Faucet exposes the database's default schema:

| Database | Default Schema |
|----------|---------------|
| PostgreSQL | `public` |
| MySQL | (the database itself) |
| SQL Server | `dbo` |
| Snowflake | `PUBLIC` |
| SQLite | `main` |

Override with the `--schema` flag when adding a service or the `schema` field in the API:

```bash
faucet db add \
  --name mydb \
  --driver postgres \
  --dsn "postgres://..." \
  --schema custom_schema
```

---

## Read-Only and Raw SQL Modes

Each service supports two security flags:

| Flag | Default | Description |
|------|---------|-------------|
| `read_only` | `false` | Only allow GET requests (no INSERT, UPDATE, DELETE, DDL) |
| `raw_sql_allowed` | `false` | Allow raw SQL execution via the MCP `faucet_raw_sql` tool |

Set via the system API:

```bash
curl -X PUT http://localhost:8080/api/v1/system/service/mydb \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"read_only": true, "raw_sql_allowed": false}'
```

---

## Testing a Connection

Use the CLI to verify connectivity:

```bash
faucet db test mydb
```

Or via the API -- a successful GET to `/_table` confirms the connection is working:

```bash
curl http://localhost:8080/api/v1/mydb/_table \
  -H "Authorization: Bearer $TOKEN"
```
