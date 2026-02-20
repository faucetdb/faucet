# Architecture

This document describes the internal architecture of Faucet for contributors and LLM agents working on the codebase.

## Project Structure

```
faucet/
├── cmd/faucet/
│   ├── main.go              # Entry point
│   └── cli/
│       ├── root.go           # Root command, flag setup, config init
│       ├── serve.go          # HTTP server startup
│       ├── db.go             # Database service management
│       ├── admin.go          # Admin user management
│       ├── key.go            # API key management
│       ├── role.go           # RBAC role management
│       ├── openapi.go        # OpenAPI spec generation
│       ├── mcp.go            # MCP server startup
│       ├── benchmark.go      # Load testing
│       ├── config_cmd.go     # Config file init/show
│       ├── version.go        # Version info
│       └── helpers.go        # Shared config store/registry helpers
├── internal/
│   ├── config/
│   │   ├── store.go          # SQLite config store (CRUD for all entities)
│   │   ├── store_test.go     # Config store tests
│   │   ├── migrations.go     # Schema migrations
│   │   ├── yaml.go           # YAML config support
│   │   └── errors.go         # Sentinel errors (ErrNotFound, etc.)
│   ├── connector/
│   │   ├── connector.go      # Connector interface + request types
│   │   ├── registry.go       # Connector factory/registry
│   │   ├── postgres/         # PostgreSQL connector
│   │   ├── mysql/            # MySQL connector
│   │   ├── mssql/            # SQL Server connector
│   │   ├── snowflake/        # Snowflake connector
│   │   └── sqlite/           # SQLite connector
│   ├── handler/
│   │   ├── system.go         # System API handlers (services, roles, admins, keys)
│   │   ├── table.go          # Table CRUD handlers
│   │   ├── schema.go         # Schema introspection/DDL handlers
│   │   ├── proc.go           # Stored procedure handlers
│   │   ├── openapi.go        # OpenAPI spec endpoint
│   │   └── helpers.go        # JSON read/write utilities
│   ├── mcp/
│   │   ├── server.go         # MCP server (stdio + HTTP)
│   │   ├── handler.go        # JSON-RPC message dispatch
│   │   ├── tools.go          # MCP tool definitions
│   │   └── resources.go      # MCP resource definitions
│   ├── model/
│   │   ├── service.go        # ServiceConfig, PoolConfig
│   │   ├── admin.go          # Admin model
│   │   ├── role.go           # Role, RoleAccess, Filter, verb constants
│   │   ├── apikey.go         # APIKey model
│   │   ├── schema.go         # Schema, TableSchema, Column, ForeignKey, etc.
│   │   └── response.go       # ListResponse, ResponseMeta envelope
│   ├── openapi/
│   │   ├── generator.go      # OpenAPI 3.1 spec generation from schemas
│   │   └── types.go          # DB type → OpenAPI type mapping
│   ├── query/
│   │   ├── parser.go         # Filter expression parser
│   │   ├── builder.go        # SQL query builder
│   │   └── sanitizer.go      # Input sanitization
│   ├── server/
│   │   ├── server.go         # HTTP server, Chi router, route setup
│   │   └── middleware/
│   │       ├── auth.go       # Authentication middleware (JWT + API key)
│   │       ├── logging.go    # Structured request logging
│   │       ├── ratelimit.go  # Token bucket rate limiting
│   │       └── requestid.go  # X-Request-ID header injection
│   ├── service/
│   │   ├── auth.go           # AuthService (JWT issue/validate, API key lookup)
│   │   └── auth_test.go      # Auth service tests
│   └── ui/
│       ├── embed.go          # go:embed directive for dist/
│       └── dist/             # Built Preact UI assets
├── ui/                       # Preact UI source code
│   ├── src/
│   ├── package.json
│   └── vite.config.ts
├── docs/                     # Documentation
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── .goreleaser.yml
```

## Request Lifecycle

Every API request follows this path:

```
HTTP Request → Chi Router
    ↓
Global Middleware Stack (in order):
    1. RequestID   — generates UUID, sets X-Request-ID header
    2. Logger      — structured request logging via slog
    3. Recoverer   — panic recovery
    4. RealIP      — extract real client IP
    5. CORS        — cross-origin resource sharing
    6. Compress    — gzip response compression
    ↓
Route Matching:
    /healthz, /readyz         → Health check handlers (no auth)
    /openapi.json             → Combined OpenAPI spec (no auth)
    /api/v1/system/*          → System handlers (admin auth required)
    /api/v1/{service}/*       → Service handlers (API key or JWT auth)
    /admin, /setup, etc.      → Embedded UI (SPA, no auth — UI handles login)
    ↓
Authentication Middleware (on protected routes):
    Extract API key from X-API-Key header
    OR extract JWT from Authorization: Bearer header
    → Creates Principal context value
    ↓
Handler:
    1. Parse query parameters (filter, fields, order, limit, offset)
    2. Resolve service name → connector from registry
    3. Introspect schema (cached)
    4. Build parameterized SQL via connector's query builder
    5. Execute query via sqlx with connection pooling
    6. Format response as JSON envelope {resource: [...], meta: {...}}
    ↓
Response
```

## Key Abstractions

### Connector Interface

The central abstraction. Every database implements this interface:

```go
type Connector interface {
    Connect(cfg ConnectionConfig) error
    Disconnect() error
    Ping(ctx context.Context) error
    DB() *sqlx.DB

    IntrospectSchema(ctx context.Context) (*model.Schema, error)
    IntrospectTable(ctx context.Context, tableName string) (*model.TableSchema, error)
    GetTableNames(ctx context.Context) ([]string, error)
    GetStoredProcedures(ctx context.Context) ([]model.StoredProcedure, error)

    BuildSelect(ctx context.Context, req SelectRequest) (string, []interface{}, error)
    BuildInsert(ctx context.Context, req InsertRequest) (string, []interface{}, error)
    BuildUpdate(ctx context.Context, req UpdateRequest) (string, []interface{}, error)
    BuildDelete(ctx context.Context, req DeleteRequest) (string, []interface{}, error)
    BuildCount(ctx context.Context, req CountRequest) (string, []interface{}, error)

    CreateTable(ctx context.Context, def model.TableSchema) error
    AlterTable(ctx context.Context, tableName string, changes []SchemaChange) error
    DropTable(ctx context.Context, tableName string) error

    CallProcedure(ctx context.Context, name string, params map[string]interface{}) ([]map[string]interface{}, error)

    DriverName() string
    QuoteIdentifier(name string) string
    SupportsReturning() bool
    SupportsUpsert() bool
    ParameterPlaceholder(index int) string
}
```

Each connector (postgres, mysql, mssql, snowflake, sqlite) implements dialect-specific SQL generation. The handlers are database-agnostic — they only interact through this interface.

### Registry

The `connector.Registry` is a thread-safe map of service name → active Connector. It also holds driver factories for lazy instantiation.

### Config Store

`config.Store` wraps an embedded SQLite database that persists:
- Database service configurations (name, driver, DSN, pool settings)
- Admin accounts (email, bcrypt password hash)
- RBAC roles and access rules
- API keys (SHA-256 hashed)

All CLI commands and system API handlers share the same store.

### Auth Service

`service.AuthService` handles:
- JWT token issuance and validation
- API key lookup (hash the incoming key, query store)
- Principal creation from authenticated credentials

## Adding a New Database Connector

1. Create `internal/connector/newdb/connector.go` implementing the `Connector` interface
2. Add introspection queries in `internal/connector/newdb/introspect.go`
3. Add query builder in `internal/connector/newdb/query_builder.go`
4. Register the driver in the registry setup (in `serve.go`, `mcp.go`, `helpers.go`)
5. Add the driver to `go.mod` dependencies
6. Add tests

## Configuration Precedence

1. CLI flags (highest priority)
2. Environment variables (`FAUCET_*` prefix)
3. Config file (`faucet.yaml` in cwd or `~/.faucet/`)
4. Defaults

Viper handles the merging. The config store (SQLite) is the persistent state for services, roles, keys, and admins.

## Concurrency Model

- The HTTP server uses Go's `net/http` with Chi router — each request gets its own goroutine
- Each database service has its own `sql.DB` connection pool with configurable limits
- The connector registry uses `sync.RWMutex` for thread-safe access
- The config store uses SQLite with `MaxOpenConns=1` (SQLite limitation for writes)
- Graceful shutdown via `signal.NotifyContext` for SIGINT/SIGTERM

## Security

- All filter values are parameterized — never string-interpolated into SQL
- API keys are stored as SHA-256 hashes
- Admin passwords are hashed (SHA-256 currently, bcrypt planned)
- DSN values are never exposed in API list responses
- Rate limiting middleware available per-key and per-role
- RBAC with row-level security filters
