# Faucet: REST API and MCP server for any SQL database

[![GitHub](https://img.shields.io/badge/GitHub-faucetdb%2Ffaucet-blue?logo=github)](https://github.com/faucetdb/faucet)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](https://github.com/faucetdb/faucet/blob/main/LICENSE)
[![Docs](https://img.shields.io/badge/docs-wiki.faucetdb.ai-blue)](https://wiki.faucetdb.ai)

**Faucet** is an open-source (MIT) single Go binary that turns PostgreSQL, MySQL, MariaDB, SQL Server, Oracle, Snowflake or SQLite into a REST API and an MCP server for AI agents, with role-based access control (RBAC), OpenAPI 3.1 and a built-in admin UI. Endpoints, the OpenAPI spec and MCP tools are generated from your schema at runtime.

Use it to put a database you already have, including one on-prem, behind a REST API and an MCP endpoint so Claude, ChatGPT, Cursor, VS Code, Base44, Replit and your own apps can use it.

---

## Quick Start

```bash
docker run -d --name faucet \
  -p 8080:8080 \
  -v faucet-data:/data \
  faucetdb/faucet
```

Open **http://localhost:8080**. The setup wizard creates your admin account and connects your first database: pick the engine, enter host, port, user and password, click **Test connection**, and save. Then create a role on the **Roles** page and an API key on the **API keys** page.

Prefer the CLI? Run it inside the container:

```bash
# Add a PostgreSQL database (no connection string needed).
# From inside the container, a database on your machine is host.docker.internal
# (on Linux, start the container with --add-host=host.docker.internal:host-gateway).
docker exec faucet faucet db add --name mydb --driver postgres \
  --host host.docker.internal --user app --password 's3cret' --database mydb

# Create a role (GET-only on every service) and an API key bound to it
docker exec faucet faucet role create --name default --verbs GET
docker exec faucet faucet key create --role default

# Databases added with the CLI are loaded on the next server start
docker restart faucet

# Query your data
curl -H "X-API-Key: YOUR_KEY" \
  "http://localhost:8080/api/v1/mydb/_table/users?limit=10"
```

---

## Connect AI agents (MCP)

The server includes a [Model Context Protocol](https://modelcontextprotocol.io) endpoint at **`http://localhost:8080/mcp`** (Streamable HTTP), on the same port as the REST API. Authenticate with an API key in the **`X-API-Key`** header; the key's role decides which databases and tables the agent can see and change.

**Claude Code**

```bash
claude mcp add --transport http faucet http://localhost:8080/mcp \
  --header "X-API-Key: YOUR_API_KEY"
```

**Cursor** (`~/.cursor/mcp.json`)

```json
{
  "mcpServers": {
    "faucet": {
      "url": "http://localhost:8080/mcp",
      "headers": { "X-API-Key": "YOUR_API_KEY" }
    }
  }
}
```

The admin UI's **AI agents (MCP)** page has copy-ready configs for Claude Code, Claude Desktop (via `mcp-remote`), Cursor, VS Code, Windsurf and ChatGPT (a Custom GPT Action that imports `/openapi.json`; ChatGPT connectors need OAuth, which Faucet does not support). Full guide: [wiki.faucetdb.ai/mcp-server](https://wiki.faucetdb.ai/mcp-server).

**stdio** (local only, runs with admin rights; roles are not applied):

```bash
docker run -i --rm -v faucet-data:/data faucetdb/faucet mcp
```

| MCP tool | Description |
|----------|-------------|
| `faucet_list_services` | List connected databases |
| `faucet_list_tables` | List tables in a database |
| `faucet_describe_table` | Get column names, types, and constraints |
| `faucet_query` | Query records with filters, ordering, pagination |
| `faucet_insert` | Insert records |
| `faucet_update` | Update records |
| `faucet_delete` | Delete records |
| `faucet_raw_sql` | Execute raw SQL (off unless allowed for the service and granted all verbs) |

---

## Supported Databases

| Database | Tutorial |
|----------|----------|
| **PostgreSQL** | [Guide](https://wiki.faucetdb.ai/tutorial-postgres) |
| **MySQL** | [Guide](https://wiki.faucetdb.ai/tutorial-mysql) |
| **MariaDB** (via the MySQL driver) | [Guide](https://wiki.faucetdb.ai/tutorial-mysql) |
| **SQL Server** | [Guide](https://wiki.faucetdb.ai/tutorial-sqlserver) |
| **Oracle** | [Guide](https://wiki.faucetdb.ai/tutorial-oracle) |
| **Snowflake** | [Guide](https://wiki.faucetdb.ai/tutorial-snowflake) |
| **SQLite** | [Guide](https://wiki.faucetdb.ai/tutorial-sqlite) |

Supported versions and cloud variants: [github.com/faucetdb/faucet#supported-databases](https://github.com/faucetdb/faucet#supported-databases).

---

## Key Features

### REST API
- **Full CRUD**: `GET`, `POST`, `PUT`, `PATCH`, `DELETE` for every table
- **Filtering**: `filter=(age > 21) AND (status = 'active')`, safely parameterized
- **Pagination**: `limit` and `offset`, with a total count via `include_count=true`
- **Field selection and ordering**: `fields=id,name,email`, `order=created_at DESC`
- **Schema and stored procedures**: introspect, create and alter tables; call procedures

### Access control
- **API keys**: SHA-256 hashed, each bound to a role
- **RBAC**: per-service, per-table, per-verb rules; fail closed
- **Read-only services**: reject every non-GET request
- **Row-level filters**: stored on access rules, but **not enforced yet**
- **Admin sessions**: JWT (HMAC-SHA256)

### OpenAPI 3.1
- Generated from the live schema at `/openapi.json`
- Import into Swagger UI, Postman, Insomnia, a Custom GPT Action, or a client generator

### Admin UI
- Embedded in the binary: setup wizard, database connections with **Test connection**, schema and data browser, API explorer, roles, API keys, and MCP setup

---

## Docker Compose with PostgreSQL

```yaml
services:
  faucet:
    image: faucetdb/faucet:latest
    ports:
      - "8080:8080"
    environment:
      # Optional: pin the JWT signing secret. If unset, Faucet generates one
      # on first start and stores it in the /data volume.
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

In the setup wizard, connect to host `postgres`, port `5432`, user `faucet`, password `faucet`, database `demo`.

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FAUCET_SERVER_HOST` | `0.0.0.0` | Bind address |
| `FAUCET_SERVER_PORT` | `8080` | HTTP port (REST API, admin UI, `/mcp`, `/openapi.json`) |
| `FAUCET_AUTH_JWT_SECRET` | *(generated on first start)* | JWT signing secret for admin sessions. If unset, a random secret is generated once and stored in the `/data` volume, so it survives restarts but not a volume wipe. Set it explicitly to pin the secret across hosts or replicas. `FAUCET_JWT_SECRET` is accepted as an alias. |
| `FAUCET_ADMIN_EMAIL`, `FAUCET_ADMIN_PASSWORD` | *(unset)* | Create the first admin account on startup when none exists (password at least 8 characters) |
| `FAUCET_DATA_DIR` | `/data` | Directory for the SQLite config store |
| `FAUCET_TELEMETRY` | *(enabled)* | Set to `0` to turn off anonymous usage telemetry ([details](https://github.com/faucetdb/faucet/blob/main/TELEMETRY.md)) |

### Volumes

| Path | Purpose |
|------|---------|
| `/data` | SQLite config store: database connections, roles, API keys, admin accounts |

### Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| `8080` | HTTP | REST API, MCP endpoint (`/mcp`), admin UI, OpenAPI spec, health checks |

---

## Health Checks

The image has a built-in `HEALTHCHECK` against `/healthz`.

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe |

---

## API Routes at a Glance

```
GET  /healthz                              Liveness probe
GET  /readyz                               Readiness probe
GET  /openapi.json                         OpenAPI 3.1 specification
POST /mcp                                  MCP endpoint (Streamable HTTP, X-API-Key)

POST   /api/v1/system/admin/session        Admin login (returns JWT)
POST   /api/v1/system/connection/test      Test database connection settings
GET    /api/v1/system/service              List database services
POST   /api/v1/system/service              Add a database service
GET    /api/v1/system/role                 List RBAC roles
POST   /api/v1/system/role                 Create RBAC role
POST   /api/v1/system/api-key              Create API key

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
docker exec faucet faucet db add --name NAME --driver DRIVER --host HOST --user USER --password PASS --database DB
docker exec faucet faucet db list           # List databases
docker exec faucet faucet db test NAME      # Test connectivity
docker exec faucet faucet db schema NAME    # Dump schema as JSON
docker exec faucet faucet role create       # Create RBAC role (use --verbs to grant access)
docker exec faucet faucet role grant        # Add an access rule to an existing role
docker exec faucet faucet role list         # List roles and their access rules
docker exec faucet faucet key create        # Create API key
docker exec faucet faucet admin create      # Create admin account
docker exec faucet faucet openapi           # Generate OpenAPI spec
docker exec faucet faucet version           # Show version info
```

`--dsn` still works if you already have a connection string. Full reference: [wiki.faucetdb.ai/cli-reference](https://wiki.faucetdb.ai/cli-reference).

---

## Image Details

- **Base**: `alpine:3.21`
- **Architectures**: `linux/amd64`, `linux/arm64`
- **Binary**: statically compiled Go, CGO disabled
- **User**: runs as the non-root `faucet` user
- **Tags**: `latest` and one tag per release version (for example `0.1.14`)

---

## Production Checklist

- [ ] Set `FAUCET_AUTH_JWT_SECRET` to a strong random value (otherwise one is generated and stored in `/data`; back up the volume or pin it)
- [ ] Use a specific image tag instead of `:latest`
- [ ] Mount `/data` to a persistent volume
- [ ] Put Faucet behind a reverse proxy (nginx, Caddy, Traefik) with TLS
- [ ] Give each app or agent its own API key with a least-privilege role
- [ ] Leave raw SQL off unless a service needs it
- [ ] Set up liveness (`/healthz`) and readiness (`/readyz`) probes

More: [wiki.faucetdb.ai/deployment](https://wiki.faucetdb.ai/deployment).

---

## Links

- **Website**: [faucetdb.ai](https://faucetdb.ai)
- **Documentation**: [wiki.faucetdb.ai](https://wiki.faucetdb.ai)
- **MCP setup**: [wiki.faucetdb.ai/mcp-server](https://wiki.faucetdb.ai/mcp-server)
- **GitHub**: [github.com/faucetdb/faucet](https://github.com/faucetdb/faucet)
- **npm**: [@faucetdb/faucet](https://www.npmjs.com/package/@faucetdb/faucet)
- **Issues**: [github.com/faucetdb/faucet/issues](https://github.com/faucetdb/faucet/issues)
- **License**: [MIT](https://github.com/faucetdb/faucet/blob/main/LICENSE)

---

<sub>Faucet is an open-source (MIT) database-to-API server written in Go. It supports PostgreSQL, MySQL, MariaDB, SQL Server, Oracle, Snowflake and SQLite. It generates CRUD REST endpoints and an OpenAPI 3.1 spec from the live schema, serves an MCP endpoint at /mcp for AI agents such as Claude, Cursor and VS Code, and secures both with API keys and role-based access control. It ships as a single binary with an embedded admin UI.</sub>
