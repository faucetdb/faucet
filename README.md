# Faucet

Turn any SQL database into a secure REST API. One binary. One command.

```
faucet serve
```

Faucet auto-generates REST endpoints, OpenAPI specs, and MCP tools from your database schema at runtime. No code generation, no ORM, no boilerplate.

## 60-Second Quickstart

```bash
# Install
go install github.com/faucetdb/faucet/cmd/faucet@latest

# Start the server
faucet serve

# Add a database
faucet db add mydb --driver postgres --dsn "postgres://user:pass@localhost/mydb?sslmode=disable"

# Query your data
curl -H "X-API-Key: YOUR_KEY" http://localhost:8080/api/v1/mydb/_table/users?limit=10
```

## Features

- **4 Database Connectors** - PostgreSQL, MySQL, SQL Server, Snowflake
- **Full CRUD REST API** - GET, POST, PUT, PATCH, DELETE with filtering, ordering, pagination
- **DreamFactory-Compatible Filters** - `(age > 21) AND (status = 'active')`
- **Schema Introspection & DDL** - Discover tables, create/alter/drop via API
- **Stored Procedure Calls** - Execute stored procedures with parameters
- **RBAC** - Roles with per-table verb permissions and row-level security filters
- **API Key + JWT Auth** - SHA-256 hashed keys, HMAC-SHA256 signed tokens
- **OpenAPI 3.1 Spec** - Auto-generated from live database schema
- **MCP Server** - 8 tools + 2 resources for AI agent integration (Claude, etc.)
- **Embedded Admin UI** - Preact + Tailwind dashboard with setup wizard
- **Single Binary** - Zero external dependencies, ~22MB, cross-platform
- **SQLite Config Store** - All configuration stored locally, no external DB required

## API Routes

```
# Health
GET  /healthz                                    # Liveness probe
GET  /readyz                                     # Readiness probe

# OpenAPI
GET  /openapi.json                               # Combined spec for all services

# System (admin JWT required)
POST   /api/v1/system/admin/session              # Login
DELETE /api/v1/system/admin/session              # Logout
GET    /api/v1/system/service                    # List services
POST   /api/v1/system/service                    # Create service
GET    /api/v1/system/role                       # List roles
POST   /api/v1/system/role                       # Create role
POST   /api/v1/system/api-key                    # Create API key

# Database CRUD (API key or JWT required)
GET    /api/v1/{service}/_table                  # List tables
GET    /api/v1/{service}/_table/{table}          # Query records
POST   /api/v1/{service}/_table/{table}          # Insert records
PUT    /api/v1/{service}/_table/{table}          # Replace records
PATCH  /api/v1/{service}/_table/{table}          # Update records
DELETE /api/v1/{service}/_table/{table}          # Delete records

# Schema
GET    /api/v1/{service}/_schema                 # List tables with schema
GET    /api/v1/{service}/_schema/{table}         # Get table schema
POST   /api/v1/{service}/_schema                 # Create table
DELETE /api/v1/{service}/_schema/{table}         # Drop table

# Stored Procedures
GET    /api/v1/{service}/_proc                   # List procedures
POST   /api/v1/{service}/_proc/{proc}            # Call procedure
```

## Query Parameters

| Parameter | Example | Description |
|-----------|---------|-------------|
| `filter`  | `(age > 21) AND (name LIKE 'A%')` | DreamFactory-compatible filter |
| `order`   | `created_at DESC, name ASC` | Sort order |
| `limit`   | `25` | Max records to return |
| `offset`  | `50` | Skip N records |
| `fields`  | `id,name,email` | Select specific columns |
| `ids`     | `1,2,3` | Filter by primary key |
| `count`   | `true` | Include total count in meta |

## MCP Server (for AI Agents)

```bash
# stdio mode (for Claude Desktop / Claude Code)
faucet mcp

# HTTP mode (for remote clients)
faucet mcp --transport http --port 3001
```

Claude Desktop config (`claude_desktop_config.json`):
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

Available tools: `faucet_list_services`, `faucet_list_tables`, `faucet_describe_table`, `faucet_query`, `faucet_insert`, `faucet_update`, `faucet_delete`, `faucet_raw_sql`

## CLI

```bash
faucet serve                    # Start HTTP server
faucet db add NAME              # Add database connection
faucet db list                  # List configured databases
faucet db test NAME             # Test connectivity
faucet db schema NAME           # Dump schema
faucet key create               # Create API key
faucet key list                 # List API keys
faucet role create              # Create RBAC role
faucet admin create             # Create admin account
faucet mcp                      # Start MCP server
faucet openapi                  # Generate OpenAPI spec
faucet version                  # Show version
```

## Docker

```bash
docker run -p 8080:8080 -v faucet-data:/data faucetdb/faucet
```

## Building from Source

```bash
git clone https://github.com/faucetdb/faucet.git
cd faucet
make build      # Builds UI + Go binary
make test       # Runs all tests
make dev        # Dev mode with hot reload
```

## Tech Stack

- **Go 1.24+** with Chi router, sqlx, Cobra/Viper
- **Preact + Vite + Tailwind** for the admin UI
- **SQLite** (pure Go, no CGO) for configuration
- **MCP** (Model Context Protocol) for AI integration

## License

MIT
