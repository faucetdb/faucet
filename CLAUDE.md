# Faucet - CLAUDE.md

Instructions for AI assistants working on this codebase.

## What Is This?

Faucet is an open-source Go application that turns any SQL database into a secure REST API. Single binary, zero configuration, built-in MCP server for AI agents.

## Architecture

- **Language**: Go 1.24+
- **HTTP Router**: Chi (`go-chi/chi/v5`)
- **DB Access**: `jmoiron/sqlx` over `database/sql`
- **CLI**: Cobra + Viper
- **Config Storage**: Embedded SQLite (`modernc.org/sqlite`, pure Go)
- **Frontend**: Preact + Vite + Tailwind, embedded via `go:embed`

## Project Structure

```
cmd/faucet/          - CLI entry point and Cobra commands
internal/
  server/            - HTTP server, Chi router, middleware
  handler/           - HTTP request handlers
  service/           - Business logic layer
  connector/         - Database connector interface + implementations
    postgres/        - PostgreSQL connector
    mysql/           - MySQL connector
    mssql/           - SQL Server connector
    snowflake/       - Snowflake connector
  query/             - Filter parser, query builder utilities
  openapi/           - OpenAPI 3.1 spec generator
  mcp/               - MCP server implementation
  config/            - SQLite config store, YAML config
  model/             - Data models (shared types)
  ui/dist/           - Embedded frontend assets
ui/                  - Frontend source (Preact)
```

## Key Commands

```bash
make build          # Build binary (UI + Go)
make build-go       # Build Go binary only
make test           # Run all tests
make dev            # Run in dev mode
make lint           # Run linter
```

## Conventions

- All filter values MUST be parameterized (never string-interpolated) to prevent SQL injection
- Every database connector implements the `connector.Connector` interface
- Response format: `{"resource": [...], "meta": {...}}` for lists
- Error format: `{"error": {"code": N, "message": "...", "context": {...}}}`
- API keys are SHA-256 hashed, admin passwords are bcrypt hashed
- All table/column names must pass `query.ValidateIdentifier()`

## Testing

```bash
go test ./...                          # Unit tests
go test -tags=integration ./...        # Integration tests (need Docker)
```

## Git Workflow

- PRs target `dev` branch, never `main` directly
- Commit messages should be descriptive
