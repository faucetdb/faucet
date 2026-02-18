# CLI Reference

Faucet is a single binary with subcommands for managing the server, databases, API keys, roles, and more.

```
faucet <command> [subcommand] [flags]
```

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `./faucet.yaml` | Path to config file |

## faucet serve

Start the Faucet API server.

```bash
faucet serve [flags]
```

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | `8080` | HTTP listen port |
| `--host` | | `0.0.0.0` | HTTP listen host |
| `--no-ui` | | `false` | Disable the embedded admin UI |
| `--dev` | | `false` | Enable development mode (verbose logging, CORS *) |
| `--data-dir` | | `~/.faucet` | Data directory for SQLite config database |

**Examples:**

```bash
# Start with defaults
faucet serve

# Custom port
faucet serve --port 9090

# Development mode with verbose logging
faucet serve --dev

# Without admin UI
faucet serve --no-ui

# Custom data directory
faucet serve --data-dir /var/lib/faucet

# Bind to localhost only
faucet serve --host 127.0.0.1 --port 8080
```

**On startup, the server:**
1. Initializes the SQLite config store at the data directory
2. Registers database drivers (postgres, mysql, mssql, snowflake, sqlite)
3. Connects all active database services from the config
4. Initializes the authentication service
5. Starts the HTTP server with all routes and middleware

**Endpoints served:**

| URL | Description |
|-----|-------------|
| `http://host:port/` | Admin UI (SPA) |
| `http://host:port/admin` | Admin dashboard |
| `http://host:port/setup` | First-run setup wizard |
| `http://host:port/healthz` | Liveness probe |
| `http://host:port/readyz` | Readiness probe |
| `http://host:port/openapi.json` | Combined OpenAPI spec |
| `http://host:port/api/v1/...` | REST API |

---

## faucet db

Manage database connections. Aliases: `service`, `database`.

### faucet db add

Add a new database connection. Supports interactive mode (prompts for missing fields) or non-interactive mode (all fields via flags).

```bash
faucet db add [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--name` | Service name (unique identifier) |
| `--driver` | Database driver: `postgres`, `mysql`, `mssql`, `snowflake` |
| `--dsn` | Data source name / connection string |
| `--label` | Human-readable label (defaults to name) |
| `--schema` | Database schema to expose (default depends on driver) |

**Examples:**

```bash
# Non-interactive
faucet db add --name mydb --driver postgres --dsn "postgres://user:pass@localhost/mydb"

# Interactive mode (prompts for each field)
faucet db add

# With schema override
faucet db add --name mydb --driver postgres \
  --dsn "postgres://user:pass@localhost/mydb" \
  --schema custom_schema
```

### faucet db list

List all registered database services. Alias: `ls`.

```bash
faucet db list [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

**Example output:**

```
NAME                 DRIVER       ACTIVE
----                 ------       ------
mydb                 postgres     yes
analytics            snowflake    yes
legacy               mysql        no
```

### faucet db remove

Remove a database service. Aliases: `rm`, `delete`.

```bash
faucet db remove <name>
```

**Example:**

```bash
faucet db remove legacy
```

### faucet db test

Test a database connection by attempting to connect and ping.

```bash
faucet db test <name>
```

**Example:**

```bash
faucet db test mydb
# Testing connection "mydb"... ok
```

### faucet db schema

Print the database schema as JSON. Optionally filter to a single table.

```bash
faucet db schema <name> [flags]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--table` | Show schema for a single table only |

**Examples:**

```bash
# Full schema
faucet db schema mydb

# Single table
faucet db schema mydb --table users
```

---

## faucet key

Manage API keys. Alias: `apikey`.

### faucet key create

Create a new API key. The raw key is shown once and cannot be retrieved again.

```bash
faucet key create [flags]
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--role` | Yes | Role name to bind the key to |
| `--label` | No | Human-readable label for the key |

**Examples:**

```bash
faucet key create --role readonly --label "CI pipeline"
faucet key create --role admin
```

**Output:**

```
API Key created:

  Key:   faucet_a1b2c3d4e5f6...
  Role:  readonly
  Label: CI pipeline

  Save this key now - it cannot be retrieved again.
```

### faucet key list

List all API keys (without exposing the actual key values). Alias: `ls`.

```bash
faucet key list [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

**Example output:**

```
PREFIX       ROLE             LABEL                    ACTIVE
------       ----             -----                    ------
faucet_a1   readonly         CI pipeline              yes
faucet_b2   admin            Admin UI                 yes
```

### faucet key revoke

Revoke an API key by its prefix, preventing further authenticated requests.

```bash
faucet key revoke <prefix>
```

**Example:**

```bash
faucet key revoke faucet_a1
```

---

## faucet role

Manage RBAC roles.

### faucet role create

Create a new role.

```bash
faucet role create [flags]
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--name` | Yes | Role name |
| `--description` | No | Role description |

**Examples:**

```bash
faucet role create --name readonly --description "Read-only access to all services"
faucet role create --name admin --description "Full access"
```

### faucet role list

List all roles. Alias: `ls`.

```bash
faucet role list [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

**Example output:**

```
NAME                 DESCRIPTION                              ACTIVE
----                 -----------                              ------
readonly             Read-only access to all services         yes
admin                Full access                              yes
```

---

## faucet admin

Manage admin users.

### faucet admin create

Create a new admin user. If `--password` is omitted, you will be prompted interactively (with confirmation).

```bash
faucet admin create [flags]
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--email` | Yes | Admin email address |
| `--password` | No | Admin password (prompted if omitted) |
| `--name` | No | Admin display name |

**Examples:**

```bash
# With password flag
faucet admin create --email admin@example.com --password changeme123

# Interactive password prompt
faucet admin create --email admin@example.com

# With display name
faucet admin create --email admin@example.com --name "Admin User"
```

Password must be at least 8 characters.

### faucet admin list

List all admin accounts. Alias: `ls`.

```bash
faucet admin list [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

---

## faucet mcp

Start the MCP (Model Context Protocol) server for AI agent integration.

```bash
faucet mcp [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--transport` | `stdio` | Transport mode: `stdio` or `http` |
| `--port` | `3001` | HTTP port (only used with `--transport http`) |
| `--data-dir` | `~/.faucet` | Data directory for SQLite config |

**Examples:**

```bash
# stdio mode for Claude Desktop
faucet mcp

# HTTP mode for remote access
faucet mcp --transport http --port 3001

# Custom data directory
faucet mcp --data-dir /var/lib/faucet
```

See [MCP Server](mcp-server.md) for complete documentation.

---

## faucet openapi

Generate an OpenAPI 3.1 specification for one or all database services.

```bash
faucet openapi [service] [flags]
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--all` | | Generate combined spec for all services |
| `--output` | `-o` | Write spec to file instead of stdout |

**Examples:**

```bash
# Single service
faucet openapi mydb

# All services
faucet openapi --all

# Write to file
faucet openapi mydb -o spec.json

# Pipe to other tools
faucet openapi --all | jq '.paths | keys'
```

---

## faucet benchmark

Run a load test against a database to measure query throughput and latency.

```bash
faucet benchmark [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--driver` | `postgres` | Database driver |
| `--dsn` | (required) | Connection string |
| `--duration` | `30s` | Test duration |
| `--concurrency` | `10` | Number of concurrent workers |
| `--table` | (auto) | Table to query (auto-detected if omitted) |

**Examples:**

```bash
# Basic benchmark
faucet benchmark --dsn "postgres://localhost/mydb"

# With custom settings
faucet benchmark \
  --driver postgres \
  --dsn "postgres://localhost/mydb" \
  --duration 60s \
  --concurrency 50 \
  --table users
```

**Output:**

```
Faucet Benchmark
================
  driver:      postgres
  duration:    30s
  concurrency: 10

Connecting... ok
Detecting tables... using "users"
  query: SELECT * FROM "users" LIMIT 10

Running benchmark...

Results
-------
  Total queries:  45230
  Errors:         0
  QPS:            1507.7
  Latency p50:    0.65ms
  Latency p95:    1.2ms
  Latency p99:    3.1ms
  Latency max:    15.4ms
```

---

## faucet version

Print version, build, and runtime information.

```bash
faucet version [flags]
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

**Example output:**

```
faucet v0.1.0
  commit:  abc1234
  built:   2025-01-15T10:30:00Z
  go:      go1.24.0
  os/arch: darwin/arm64
```

**JSON output:**

```json
{
  "version": "v0.1.0",
  "commit": "abc1234",
  "built": "2025-01-15T10:30:00Z",
  "go_version": "go1.24.0",
  "os": "darwin",
  "arch": "arm64"
}
```
