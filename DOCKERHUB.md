# Faucet — Database to REST API in Seconds

[![GitHub](https://img.shields.io/badge/GitHub-faucetdb%2Ffaucet-blue?logo=github)](https://github.com/faucetdb/faucet)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://github.com/faucetdb/faucet/blob/main/LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)

**Faucet** is an open-source REST API generator that connects to any SQL database, introspects its schema, and instantly exposes a full CRUD API with authentication, role-based access control, OpenAPI documentation, and an AI-ready MCP server. Written in Go. Ships as a single ~22MB binary with zero external dependencies.

> *Your database already has an API. You just haven't turned it on yet.*

---

## Quick Start

```bash
docker run -d --name faucet \
  -p 8080:8080 \
  -v faucet-data:/data \
  faucetdb/faucet
```

Then open **http://localhost:8080** for the admin UI setup wizard, or use the CLI:

```bash
# Add a PostgreSQL database
docker exec faucet faucet db add mydb \
  --driver postgres \
  --dsn "postgres://user:pass@host:5432/mydb?sslmode=disable"

# Create an API key
docker exec faucet faucet key create --role default

# Query your data
curl -H "X-API-Key: YOUR_KEY" \
  http://localhost:8080/api/v1/mydb/_table/users?limit=10
```

**That's it.** Full REST API with filtering, pagination, and OpenAPI docs — live in under 60 seconds.

---

## Supported Databases

| Database | Driver | Use Case |
|----------|--------|----------|
| **PostgreSQL** | `pgx/v5` | Primary OLTP, most popular choice |
| **MySQL** / MariaDB | `go-sql-driver/mysql` | Legacy systems, WordPress DBs, CMS backends |
| **SQL Server** | `go-mssqldb` (Microsoft official) | Enterprise, .NET ecosystems |
| **Snowflake** | `gosnowflake` (official) | Cloud data warehouse, analytics APIs |

All connectors support schema introspection, stored procedures, views, connection pooling, and health checks.

---

## Key Features

### REST API Generation
- **Full CRUD** — `GET`, `POST`, `PUT`, `PATCH`, `DELETE` auto-generated for every table
- **Filtering** — DreamFactory-compatible syntax: `(age > 21) AND (status = 'active')`
- **Pagination** — `limit`, `offset`, cursor-based, with total count via `count=true`
- **Field selection** — `fields=id,name,email` to reduce payload size
- **Ordering** — `order=created_at DESC, name ASC`
- **Batch operations** — Insert/update/delete multiple records in a single request

### Authentication & Access Control
- **JWT authentication** — HMAC-SHA256 tokens with configurable expiry
- **API keys** — SHA-256 hashed, bound to RBAC roles
- **Role-based access control** — Per-table, per-verb permissions (GET/POST/PUT/PATCH/DELETE)
- **Row-level security** — Server-side filters restrict visible and modifiable rows per role

### OpenAPI 3.1 Documentation
- Auto-generated from live database schema at `/openapi.json`
- Covers all services, tables, columns, and data types
- Import directly into Swagger UI, Postman, Insomnia, or any OpenAPI client

### MCP Server for AI Agents
Built-in [Model Context Protocol](https://modelcontextprotocol.io) server — 8 tools for AI agent integration:

| Tool | Description |
|------|-------------|
| `faucet_list_services` | Discover connected databases |
| `faucet_list_tables` | List tables with row counts |
| `faucet_describe_table` | Get column names, types, constraints |
| `faucet_query` | Search with filters, ordering, pagination |
| `faucet_insert` | Create records |
| `faucet_update` | Modify records |
| `faucet_delete` | Remove records |
| `faucet_raw_sql` | Execute raw SQL (optional, disabled by default) |

Works with **Claude Desktop**, **Claude Code**, **ChatGPT**, and any MCP-compatible client via stdio or HTTP transport.

### Schema Management
- Introspect tables, columns, indexes, constraints, and views
- Create, alter, and drop tables via the REST API
- Execute stored procedures with full parameter support

### Embedded Admin UI
- Built with Preact + Tailwind, compiled into the binary
- Setup wizard for initial configuration
- Manage databases, roles, API keys, and admin accounts
- Visual query builder for testing

---

## Docker Compose with PostgreSQL

```yaml
version: "3.9"

services:
  faucet:
    image: faucetdb/faucet:latest
    ports:
      - "8080:8080"
    environment:
      FAUCET_AUTH_JWT_SECRET: change-me-in-production
    volumes:
      - faucet_data:/data
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: demo
      POSTGRES_USER: faucet
      POSTGRES_PASSWORD: faucet
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U faucet -d demo"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  faucet_data:
  postgres_data:
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FAUCET_SERVER_HOST` | `0.0.0.0` | Bind address |
| `FAUCET_SERVER_PORT` | `8080` | HTTP port |
| `FAUCET_AUTH_JWT_SECRET` | *(auto-generated)* | JWT signing secret — **set this in production** |
| `FAUCET_AUTH_JWT_EXPIRY` | `1h` | JWT token lifetime |
| `FAUCET_AUTH_API_KEY_HEADER` | `X-API-Key` | Header name for API key auth |
| `FAUCET_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `FAUCET_LOG_FORMAT` | `text` | Log format: `text` or `json` |
| `FAUCET_MCP_ENABLED` | `false` | Enable built-in MCP server |

### Volumes

| Path | Purpose |
|------|---------|
| `/data` | SQLite config database, admin credentials, service definitions |

### Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| `8080` | HTTP | REST API, Admin UI, OpenAPI spec, health checks |

---

## Health Checks

```yaml
healthcheck:
  test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/healthz"]
  interval: 30s
  timeout: 5s
  retries: 3
```

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Liveness probe — is the process running? |
| `GET /readyz` | Readiness probe — is the server ready for traffic? |

---

## API Routes at a Glance

```
GET  /healthz                              Liveness probe
GET  /readyz                               Readiness probe
GET  /openapi.json                         OpenAPI 3.1 specification

POST   /api/v1/system/admin/session        Admin login (returns JWT)
GET    /api/v1/system/service              List database services
POST   /api/v1/system/service              Add a database service
GET    /api/v1/system/role                 List RBAC roles
POST   /api/v1/system/role                 Create RBAC role
POST   /api/v1/system/api-key             Create API key

GET    /api/v1/{service}/_table            List tables
GET    /api/v1/{service}/_table/{table}    Query records
POST   /api/v1/{service}/_table/{table}    Insert records
PUT    /api/v1/{service}/_table/{table}    Replace records
PATCH  /api/v1/{service}/_table/{table}    Update records
DELETE /api/v1/{service}/_table/{table}    Delete records

GET    /api/v1/{service}/_schema           List table schemas
POST   /api/v1/{service}/_schema           Create table
GET    /api/v1/{service}/_proc             List stored procedures
POST   /api/v1/{service}/_proc/{name}      Call stored procedure
```

---

## CLI Reference

The container includes the full `faucet` CLI:

```bash
docker exec faucet faucet serve             # Start server (default CMD)
docker exec faucet faucet db add NAME       # Add database connection
docker exec faucet faucet db list           # List databases
docker exec faucet faucet db test NAME      # Test connectivity
docker exec faucet faucet db schema NAME    # Dump schema as JSON
docker exec faucet faucet key create        # Create API key
docker exec faucet faucet role create       # Create RBAC role
docker exec faucet faucet admin create      # Create admin account
docker exec faucet faucet mcp              # Start MCP server (stdio)
docker exec faucet faucet openapi          # Generate OpenAPI spec
docker exec faucet faucet version          # Show version info
```

---

## Performance

Faucet is built for speed:

- **Cold start**: <100ms (single process, no warm-up)
- **Memory**: ~20-50MB baseline (vs 256MB-1GB for PHP-based alternatives)
- **Concurrency**: Go goroutines handle thousands of concurrent requests
- **Routing**: Sub-millisecond via Chi router
- **JSON**: High-performance serialization with `goccy/go-json`
- **Connection pooling**: Configurable per-service pool with health checks

---

## Image Details

- **Base**: `alpine:3.21` (minimal, ~5MB)
- **Architecture**: `linux/amd64`, `linux/arm64`
- **Binary**: Statically compiled Go, CGO disabled, no C dependencies
- **Build**: Multi-stage — Node 22 (UI) → Go 1.25 (binary) → Alpine (runtime)
- **User**: Runs as non-root when mounted volume permissions allow

---

## Production Checklist

- [ ] Set `FAUCET_AUTH_JWT_SECRET` to a strong random value
- [ ] Use a specific image tag instead of `:latest`
- [ ] Mount `/data` to a persistent volume
- [ ] Put behind a reverse proxy (nginx, Caddy, Traefik) with TLS
- [ ] Create RBAC roles with least-privilege access per table
- [ ] Disable raw SQL in MCP unless explicitly needed
- [ ] Configure connection pool limits for your database
- [ ] Set up liveness (`/healthz`) and readiness (`/readyz`) probes

---

## Links

- **GitHub**: [github.com/faucetdb/faucet](https://github.com/faucetdb/faucet)
- **Documentation**: [faucetdb.io/docs](https://faucetdb.io/docs)
- **License**: [MIT](https://github.com/faucetdb/faucet/blob/main/LICENSE)
- **Issues**: [github.com/faucetdb/faucet/issues](https://github.com/faucetdb/faucet/issues)

---

<sub>Faucet is an open-source database REST API generator written in Go. It supports PostgreSQL, MySQL, SQL Server, and Snowflake. Features include auto-generated CRUD endpoints, OpenAPI 3.1 documentation, JWT and API key authentication, role-based access control with row-level security, stored procedure execution, schema management, an embedded admin dashboard, and a built-in MCP server for AI agent integration with Claude, ChatGPT, and other LLM tools. Distributed as a single static binary with zero external dependencies. MIT licensed.</sub>
