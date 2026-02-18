# Getting Started

Faucet turns any SQL database into a secure REST API. One binary, one command, zero configuration required.

## Installation

### Homebrew (macOS / Linux)

```bash
brew install faucetdb/tap/faucet
```

### Go Install

Requires Go 1.25+:

```bash
go install github.com/faucetdb/faucet/cmd/faucet@latest
```

### Docker

```bash
docker pull faucetdb/faucet:latest
docker run -p 8080:8080 faucetdb/faucet:latest
```

### Binary Download

Download pre-built binaries from the [GitHub Releases](https://github.com/faucetdb/faucet/releases) page. Available for:

| OS      | Architecture |
|---------|-------------|
| Linux   | amd64, arm64 |
| macOS   | amd64, arm64 |
| Windows | amd64, arm64 |

Extract and place the `faucet` binary in your `PATH`.

### Build from Source

```bash
git clone https://github.com/faucetdb/faucet.git
cd faucet
make build
# Binary is at ./bin/faucet
```

To build with the embedded admin UI:

```bash
cd ui && npm install && npm run build && cd ..
make build
```

## 60-Second Quickstart

### 1. Start Faucet

```bash
faucet serve
```

Output:

```
 _____ _   _   _  ___ ___ _____
|  ___/ \ | | | |/ __| __|_   _|
| |_ / _ \| |_| | (__|  _| | |
|_| /_/ \_\___,_|\___|___| |_|

-> Faucet v0.1.0
-> Listening on http://0.0.0.0:8080
-> Admin UI:   http://0.0.0.0:8080/admin
-> OpenAPI:    http://0.0.0.0:8080/openapi.json
-> Health:     http://0.0.0.0:8080/healthz
-> Connected databases: 0
```

### 2. Create an Admin Account

```bash
faucet admin create --email admin@example.com --password changeme123
```

Or visit `http://localhost:8080/setup` in your browser.

### 3. Connect a Database

Using the CLI:

```bash
faucet db add \
  --name mydb \
  --driver postgres \
  --dsn "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
```

Or via the API after logging in:

```bash
# Get a session token
TOKEN=$(curl -s http://localhost:8080/api/v1/system/admin/session \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"changeme123"}' \
  | jq -r '.session_token')

# Add a database service
curl -X POST http://localhost:8080/api/v1/system/service \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "mydb",
    "driver": "postgres",
    "dsn": "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
  }'
```

### 4. Make Your First API Call

```bash
# List all tables
curl http://localhost:8080/api/v1/mydb/_table \
  -H "Authorization: Bearer $TOKEN"

# Query records from a table
curl "http://localhost:8080/api/v1/mydb/_table/users?limit=10" \
  -H "Authorization: Bearer $TOKEN"

# Insert a record
curl -X POST http://localhost:8080/api/v1/mydb/_table/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"name": "Alice", "email": "alice@example.com"}]}'

# Filter records
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=name%20%3D%20'Alice'" \
  -H "Authorization: Bearer $TOKEN"
```

### 5. Explore the Admin UI

Open `http://localhost:8080/admin` in your browser to:
- Manage database connections
- Browse schemas and data
- Create roles and API keys
- Test API calls with the API Explorer

## Configuration

Faucet looks for configuration in this order:

1. Command-line flags (highest priority)
2. Environment variables with `FAUCET_` prefix
3. `faucet.yaml` in the current directory
4. `~/.faucet/faucet.yaml`

### Configuration File Example

```yaml
server:
  host: 0.0.0.0
  port: 8080

auth:
  jwt_secret: "your-secret-key-change-in-production"
```

### Environment Variables

All configuration options can be set via environment variables with the `FAUCET_` prefix:

```bash
export FAUCET_SERVER_PORT=9090
export FAUCET_AUTH_JWT_SECRET="my-production-secret"
```

### Data Directory

Faucet stores its configuration (services, roles, API keys, admin accounts) in a SQLite database. By default this lives at `~/.faucet/`. Override with:

```bash
faucet serve --data-dir /var/lib/faucet
```

## Supported Databases

| Database   | Driver name  | Status |
|------------|-------------|--------|
| PostgreSQL | `postgres`  | Full support |
| MySQL      | `mysql`     | Full support |
| SQL Server | `mssql`     | Full support |
| Snowflake  | `snowflake` | Full support |
| SQLite     | `sqlite`    | Full support |

## What's Next

- [API Reference](api-reference.md) -- full REST API documentation
- [CLI Reference](cli-reference.md) -- all CLI commands and flags
- [Filter Syntax](filter-syntax.md) -- query filtering language
- [Database Connectors](database-connectors.md) -- DSN formats and driver details
- [RBAC](rbac.md) -- roles, API keys, and access control
- [MCP Server](mcp-server.md) -- AI agent integration
- [Deployment](deployment.md) -- production deployment guide
- [Snowflake Tutorial](tutorial-snowflake.md) -- connect and query Snowflake
- [SQLite Tutorial](tutorial-sqlite.md) -- connect and query SQLite databases
