<p align="center">
  <img src="ui/public/faucet-logo.svg" alt="Faucet — open-source database to REST API server" height="60">
</p>

<h3 align="center">Turn any SQL database into a secure REST API.<br>One binary. One command.</h3>

<p align="center">
  Faucet is an open-source, single-binary API server that auto-generates REST endpoints, OpenAPI specs, and MCP tools from your database schema at runtime. No code generation, no ORM, no boilerplate.
</p>

<p align="center">
  <a href="https://github.com/faucetdb/faucet/releases"><img src="https://img.shields.io/github/v/release/faucetdb/faucet?style=flat-square&color=blue" alt="GitHub Release"></a>
  <a href="https://github.com/faucetdb/faucet/blob/main/LICENSE"><img src="https://img.shields.io/github/license/faucetdb/faucet?style=flat-square" alt="License: MIT"></a>
  <a href="https://hub.docker.com/r/faucetdb/faucet"><img src="https://img.shields.io/docker/pulls/faucetdb/faucet?style=flat-square" alt="Docker Pulls"></a>
  <a href="https://github.com/faucetdb/faucet/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/faucetdb/faucet/ci.yml?branch=main&style=flat-square&label=CI" alt="CI Status"></a>
  <a href="https://goreportcard.com/report/github.com/faucetdb/faucet"><img src="https://goreportcard.com/badge/github.com/faucetdb/faucet?style=flat-square" alt="Go Report Card"></a>
</p>

<p align="center">
  <a href="https://wiki.faucetdb.ai">Docs</a> &middot;
  <a href="#60-second-quickstart">Getting Started</a> &middot;
  <a href="https://hub.docker.com/r/faucetdb/faucet">Docker</a> &middot;
  <a href="#mcp-server-for-ai-agents">MCP Server</a> &middot;
  <a href="https://github.com/faucetdb/faucet/issues">Issues</a>
</p>

---

## What is Faucet?

Faucet is a **database-to-REST-API gateway** — a lightweight, self-hosted server that connects to your SQL databases, introspects the schema, and generates a full CRUD REST API with authentication, role-based access control (RBAC), and OpenAPI documentation. It also exposes an **MCP server** so AI agents (Claude, GPT, etc.) can query your data directly.

Think of it as an open-source alternative to [DreamFactory](https://www.dreamfactory.com/), [PostgREST](https://postgrest.org/), or [Hasura](https://hasura.io/) — but with multi-database support, a built-in admin UI, and native AI agent integration, all in a single ~22MB binary.

![demo](https://github.com/faucetdb/faucet/blob/main/screenshots/faucet-demo.gif)

### Use Cases

- **Instant backend for apps** — Skip writing CRUD APIs by hand. Point Faucet at your database and start building your frontend.
- **AI agent data access** — Give Claude, GPT, or any MCP-compatible agent governed, read/write access to your databases.
- **Internal tools & dashboards** — Generate APIs for internal databases without modifying existing infrastructure.
- **Legacy database modernization** — Put a REST API in front of SQL Server 2008, MySQL 5.7, or PostgreSQL 9.6 without code changes.
- **Multi-database aggregation** — Connect PostgreSQL, MySQL, and SQL Server to a single Faucet instance and query them all through one API.
- **Rapid prototyping** — Go from empty database to working API in under 60 seconds.

---

## How It Works

```
┌─────────────┐       ┌───────────────────────────────────────┐       ┌──────────────┐
│ PostgreSQL   │       │              F A U C E T              │       │   REST API   │
│ MySQL        │──────▶│                                       │──────▶│   /api/v1/*  │
│ MariaDB      │  SQL  │  ┌─────────┐ ┌──────┐ ┌───────────┐  │       ├──────────────┤
│ SQL Server   │◀──────│  │ Schema  │ │ RBAC │ │  OpenAPI   │  │──────▶│  OpenAPI 3.1 │
│ Snowflake    │       │  │ Intro-  │ │ Auth │ │ Generator  │  │       │  /openapi.json│
│ SQLite       │       │  │ spection│ │      │ │            │  │       ├──────────────┤
└─────────────┘       │  └─────────┘ └──────┘ └───────────┘  │──────▶│  MCP Server  │
                       │                                       │       │  (AI Agents) │
                       │  ┌──────────────────────────────────┐ │       ├──────────────┤
                       │  │   Embedded Admin UI (Preact)     │ │──────▶│   Admin UI   │
                       │  └──────────────────────────────────┘ │       │  :8080/admin │
                       └───────────────────────────────────────┘       └──────────────┘
```

**Connect** any SQL database → Faucet **introspects** the schema → Instantly generates **REST endpoints**, **OpenAPI docs**, **MCP tools**, and an **Admin UI** — all secured with API keys, JWT auth, and role-based permissions.

---

## Screenshots

<p align="center">
  <img src="screenshots/faucet-services-all-connected.png" alt="Faucet Services — Multiple database connections" width="800">
  <br><em>Manage multiple database connections — PostgreSQL, MySQL, SQL Server, and more</em>
</p>

<p align="center">
  <img src="screenshots/faucet-schema-explorer-working.png" alt="Faucet Schema Explorer — Browse tables, columns, and types" width="800">
  <br><em>Schema Explorer — Browse tables, columns, types, and constraints across all connected databases</em>
</p>

<p align="center">
  <img src="screenshots/faucet-api-explorer-working.png" alt="Faucet API Explorer — Test REST endpoints interactively" width="800">
  <br><em>API Explorer — Test REST endpoints interactively with live response data</em>
</p>

---

## Key Features

### API Generation
- **Full CRUD REST API** — GET, POST, PUT, PATCH, DELETE with filtering, ordering, and pagination
- **Schema introspection** — Discovers tables, columns, types, and constraints at runtime
- **Schema DDL** — Create, alter, and drop tables via API
- **Stored procedure calls** — Execute stored procedures with typed parameters
- **DreamFactory-compatible filters** — `(age > 21) AND (status = 'active')`
- **OpenAPI 3.1 spec** — Auto-generated from live database schema at `/openapi.json`

### Security & Access Control
- **API key authentication** — SHA-256 hashed keys with per-key role assignment
- **JWT authentication** — HMAC-SHA256 signed tokens for admin sessions
- **Role-based access control (RBAC)** — Per-table verb permissions (GET, POST, PUT, DELETE)
- **Row-level security filters** — Restrict data access per role with SQL filter expressions

### AI Agent Integration (MCP)
- **Built-in MCP server** — 8 tools + 2 resources for Model Context Protocol
- **Claude Desktop ready** — Drop-in config for Claude Desktop and Claude Code
- **stdio + HTTP transport** — Works locally or over the network
- **Governed AI queries** — AI agents respect the same RBAC rules as API clients

### Developer Experience
- **Single binary** — Zero external dependencies, ~22MB, cross-platform (Linux, macOS, Windows)
- **Embedded admin UI** — Preact + Tailwind dashboard with setup wizard, schema explorer, API tester
- **SQLite config store** — All configuration stored locally, no external database required
- **Homebrew + Docker** — Install in seconds on any platform
- **Health endpoints** — `/healthz` and `/readyz` for Kubernetes-style probes

---

## Supported Databases

| Database | Versions | Cloud Variants |
|----------|----------|----------------|
| **PostgreSQL** | 9.6 – 17 | Amazon RDS, Aurora, Supabase, Neon, Azure Database |
| **MySQL** | 5.7 – 9.x | Amazon RDS, Aurora MySQL, PlanetScale, Azure MySQL |
| **MariaDB** | 10.2 – 11.x | Via MySQL driver |
| **SQL Server** | 2008 – 2022 | Azure SQL Database, Amazon RDS |
| **Snowflake** | Current | AWS, Azure, GCP |
| **SQLite** | 3.35+ | Local file, in-memory |

---

## 60-Second Quickstart

### Install

**Homebrew (macOS / Linux):**
```bash
brew install faucetdb/tap/faucet
```

**Docker:**
```bash
docker run -p 8080:8080 -v faucet-data:/data faucetdb/faucet
```

**Go:**
```bash
go install github.com/faucetdb/faucet/cmd/faucet@latest
```

**Binary download:** See [GitHub Releases](https://github.com/faucetdb/faucet/releases) for pre-built binaries (Linux, macOS, Windows).

### Run

```bash
# Start the server
faucet serve

# Create an admin account
faucet admin create --email admin@example.com --password changeme123

# Add a database
faucet db add mydb --driver postgres --dsn "postgres://user:pass@localhost/mydb?sslmode=disable"

# Create an API key
faucet key create --role default

# Query your data
curl -H "X-API-Key: faucet_YOUR_KEY_HERE" http://localhost:8080/api/v1/mydb/_table/users?limit=10
```

Open **http://localhost:8080** for the admin dashboard.

---

## MCP Server for AI Agents

Faucet includes a built-in [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server, enabling AI agents like Claude, GPT, and Copilot to query and modify your databases through governed, tool-based access.

### Claude Desktop / Claude Code

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "faucet": {
      "command": "faucet",
      "args": ["mcp"]
    }
  }
}
```

### HTTP Mode (remote clients)

```bash
faucet mcp --transport http --port 3001
```

### Available MCP Tools

| Tool | Description |
|------|-------------|
| `faucet_list_services` | List all connected databases |
| `faucet_list_tables` | List tables in a database |
| `faucet_describe_table` | Get column names, types, and constraints |
| `faucet_query` | Query records with filters, ordering, pagination |
| `faucet_insert` | Insert new records |
| `faucet_update` | Update existing records |
| `faucet_delete` | Delete records |
| `faucet_raw_sql` | Execute raw SQL (admin only) |

---

## CLI Reference

```bash
faucet serve                    # Start HTTP server (default :8080)
faucet db add NAME              # Add database connection
faucet db list                  # List configured databases
faucet db test NAME             # Test database connectivity
faucet db schema NAME           # Dump database schema
faucet key create               # Create API key
faucet key list                 # List API keys
faucet role create              # Create RBAC role
faucet admin create             # Create admin account
faucet mcp                      # Start MCP server (stdio)
faucet openapi                  # Generate OpenAPI spec
faucet config set KEY VALUE     # Set configuration value
faucet version                  # Show version info
```

## API Routes

```
GET  /healthz                                    # Liveness probe
GET  /readyz                                     # Readiness probe
GET  /openapi.json                               # OpenAPI 3.1 spec

POST   /api/v1/system/admin/session              # Admin login
GET    /api/v1/system/service                    # List services
POST   /api/v1/system/service                    # Create service
GET    /api/v1/system/role                       # List roles
POST   /api/v1/system/role                       # Create role
POST   /api/v1/system/api-key                    # Create API key

GET    /api/v1/{service}/_table                  # List tables
GET    /api/v1/{service}/_table/{table}          # Query records
POST   /api/v1/{service}/_table/{table}          # Insert records
PUT    /api/v1/{service}/_table/{table}          # Replace records
PATCH  /api/v1/{service}/_table/{table}          # Update records
DELETE /api/v1/{service}/_table/{table}          # Delete records

GET    /api/v1/{service}/_schema                 # List table schemas
POST   /api/v1/{service}/_schema                 # Create table
GET    /api/v1/{service}/_proc                   # List stored procedures
POST   /api/v1/{service}/_proc/{proc}            # Call procedure
```

## Query Parameters

| Parameter | Example | Description |
|-----------|---------|-------------|
| `filter`  | `(age > 21) AND (name LIKE 'A%')` | DreamFactory-compatible filter syntax |
| `order`   | `created_at DESC, name ASC` | Sort order |
| `limit`   | `25` | Max records to return |
| `offset`  | `50` | Skip N records for pagination |
| `fields`  | `id,name,email` | Select specific columns |
| `ids`     | `1,2,3` | Filter by primary key values |
| `include_count` | `true` | Include total record count in response metadata |

---

## FAQ

**How is Faucet different from PostgREST?**
PostgREST only supports PostgreSQL. Faucet supports 5+ databases (PostgreSQL, MySQL, SQL Server, Snowflake, SQLite), includes a built-in admin UI, and provides native MCP support for AI agents — all in a single binary.

**How is Faucet different from Hasura?**
Hasura requires Docker, a PostgreSQL metadata database, and is primarily GraphQL-focused. Faucet is a single binary with no dependencies, generates REST APIs (not GraphQL), and includes built-in MCP server support for AI agent integration.

**How is Faucet different from DreamFactory?**
DreamFactory is a PHP/Laravel application requiring a full LAMP/LEMP stack. Faucet is a single Go binary (~22MB) with zero dependencies. Faucet is fully open-source under MIT license with all features included — no paid tiers required for core functionality.

**Does Faucet support GraphQL?**
Not currently. Faucet generates REST APIs and OpenAPI 3.1 specs. GraphQL support may be added in the future.

**Is Faucet production-ready?**
Faucet is under active development. It is suitable for internal tools, prototyping, and AI agent integration. Check the [releases page](https://github.com/faucetdb/faucet/releases) for the latest version.

**Can AI agents write data through Faucet?**
Yes. MCP tools include `faucet_insert`, `faucet_update`, and `faucet_delete`. All operations respect RBAC roles, so you can give AI agents read-only or read-write access per table.

**Does Faucet require a separate database for configuration?**
No. Faucet uses an embedded SQLite database for all configuration, credentials, roles, and API keys. Everything is stored locally in a single file.

---

## Building from Source

```bash
git clone https://github.com/faucetdb/faucet.git
cd faucet
make build      # Builds UI + Go binary
make test       # Runs all tests
make dev        # Dev mode with hot reload
```

## Tech Stack

- **Go 1.24+** — Chi router, sqlx, Cobra/Viper CLI
- **Preact + Vite + Tailwind** — Embedded admin UI
- **SQLite** (pure Go, no CGO) — Configuration store
- **MCP** (Model Context Protocol) — AI agent integration

## Documentation

Full documentation is available at the [Faucet Wiki](https://wiki.faucetdb.ai).

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines, or open an [issue](https://github.com/faucetdb/faucet/issues) to report bugs and request features.

## License

[MIT](LICENSE) — free for commercial and personal use.
