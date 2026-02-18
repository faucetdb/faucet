# API Reference

Faucet exposes a REST API at `/api/v1` with two main sections:

- **System API** (`/api/v1/system/`) -- manage Faucet configuration (services, roles, API keys, admins)
- **Service API** (`/api/v1/{serviceName}/`) -- query and modify data in connected databases

## Authentication

All API endpoints (except login, health checks, and OpenAPI spec) require authentication. Faucet supports two authentication methods:

### API Key (X-API-Key header)

API keys are bound to roles that define what operations are allowed. Use the `X-API-Key` header:

```bash
curl http://localhost:8080/api/v1/mydb/_table/users \
  -H "X-API-Key: faucet_a1b2c3d4e5f6..."
```

### JWT Bearer Token (Authorization header)

Admin users authenticate with email/password and receive a JWT token. Use the `Authorization: Bearer` header:

```bash
curl http://localhost:8080/api/v1/mydb/_table/users \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
```

### Authentication Flow

```
Request arrives
  |
  +-- X-API-Key header present?
  |     YES -> Validate key hash against stored keys -> Principal{type:"api_key", role_id:N}
  |     NO  -> Continue
  |
  +-- Authorization: Bearer <token> present?
  |     YES -> Validate JWT signature + claims -> Principal{type:"admin", admin_id:N}
  |     NO  -> 401 Unauthorized
```

### Obtaining a JWT Token

```bash
curl -X POST http://localhost:8080/api/v1/system/admin/session \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@example.com", "password": "yourpassword"}'
```

Response:

```json
{
  "session_token": "eyJhbGciOiJIUzI1NiIs...",
  "token_type": "bearer",
  "expires_in": 86400,
  "admin_id": 1,
  "email": "admin@example.com",
  "name": "Admin"
}
```

Tokens expire after 24 hours by default.

---

## Response Format

### Success (list)

All list endpoints return a `resource` array with optional `meta` for pagination:

```json
{
  "resource": [
    {"id": 1, "name": "Alice", "email": "alice@example.com"},
    {"id": 2, "name": "Bob", "email": "bob@example.com"}
  ],
  "meta": {
    "count": 2,
    "total": 150,
    "limit": 25,
    "offset": 0,
    "took_ms": 3.45
  }
}
```

**Meta fields:**

| Field | Type | Description |
|-------|------|-------------|
| `count` | integer | Number of records in this response |
| `total` | integer | Total matching records (only when `include_count=true`) |
| `limit` | integer | Maximum records per page |
| `offset` | integer | Number of records skipped |
| `next_cursor` | string | Cursor for next page (when using cursor pagination) |
| `took_ms` | float | Query execution time in milliseconds |

### Success (single)

Single-resource endpoints return the resource object directly (not wrapped in `resource`).

### Error

```json
{
  "error": {
    "code": 404,
    "message": "Table not found: nonexistent",
    "context": {
      "service": "mydb",
      "table": "nonexistent"
    }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `code` | integer | HTTP status code |
| `message` | string | Human-readable error description |
| `context` | object | Additional error details (optional) |

---

## Health Check Endpoints

These endpoints require no authentication.

### GET /healthz

Liveness probe. Returns 200 if the process is running.

```json
{"status": "ok"}
```

### GET /readyz

Readiness probe. Returns 200 when the server is ready to accept traffic.

```json
{"status": "ok"}
```

### GET /openapi.json

Returns the combined OpenAPI 3.1 specification for all connected services.

---

## System API

All system endpoints (except session management) require admin authentication.

### Session Management

#### POST /api/v1/system/admin/session

Authenticate and obtain a JWT token.

**Request body:**

```json
{
  "email": "admin@example.com",
  "password": "yourpassword"
}
```

**Response (200):**

```json
{
  "session_token": "eyJhbGciOiJIUzI1NiIs...",
  "token_type": "bearer",
  "expires_in": 86400,
  "admin_id": 1,
  "email": "admin@example.com",
  "name": "Admin"
}
```

#### DELETE /api/v1/system/admin/session

Log out (instructs client to discard token).

**Response (200):**

```json
{
  "success": true,
  "message": "Session invalidated"
}
```

### Service Management

#### GET /api/v1/system/service

List all configured database services.

**Response (200):**

```json
{
  "resource": [
    {
      "id": 1,
      "name": "mydb",
      "label": "My Database",
      "driver": "postgres",
      "schema": "public",
      "read_only": false,
      "raw_sql_allowed": false,
      "is_active": true,
      "created_at": "2025-01-15T10:30:00Z",
      "updated_at": "2025-01-15T10:30:00Z"
    }
  ],
  "meta": {"count": 1}
}
```

Note: The DSN is never exposed in list responses.

#### POST /api/v1/system/service

Create a new database service.

**Request body:**

```json
{
  "name": "mydb",
  "label": "My Database",
  "driver": "postgres",
  "dsn": "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
  "schema": "public",
  "read_only": false,
  "raw_sql_allowed": false
}
```

Required fields: `name`, `driver`, `dsn`

#### GET /api/v1/system/service/{serviceName}

Get a single service by name.

#### PUT /api/v1/system/service/{serviceName}

Update a service configuration. Only non-empty fields in the request body are applied.

#### DELETE /api/v1/system/service/{serviceName}

Delete a service and disconnect it.

### Role Management

#### GET /api/v1/system/role

List all roles.

#### POST /api/v1/system/role

Create a new role with optional access rules.

**Request body:**

```json
{
  "name": "readonly",
  "description": "Read-only access to all services",
  "access": [
    {
      "service_name": "mydb",
      "component": "_table/*",
      "verb_mask": 1
    }
  ]
}
```

See [RBAC](rbac.md) for verb mask values and access rule details.

#### GET /api/v1/system/role/{roleId}

Get a single role by ID, including its access rules.

#### PUT /api/v1/system/role/{roleId}

Update a role. If `access` is provided, it replaces all existing access rules.

#### DELETE /api/v1/system/role/{roleId}

Delete a role by ID.

### Admin Management

#### GET /api/v1/system/admin

List all admin accounts (passwords are never exposed).

#### POST /api/v1/system/admin

Create a new admin account.

**Request body:**

```json
{
  "email": "newadmin@example.com",
  "password": "securepassword",
  "name": "New Admin"
}
```

Password must be at least 8 characters.

### API Key Management

#### GET /api/v1/system/api-key

List all API keys. The actual key value is never shown -- only the prefix.

**Response (200):**

```json
{
  "resource": [
    {
      "id": 1,
      "key_prefix": "faucet_a1b2c3d",
      "label": "CI pipeline",
      "role_id": 2,
      "is_active": true,
      "created_at": "2025-01-15T10:30:00Z",
      "last_used": "2025-01-16T08:00:00Z"
    }
  ],
  "meta": {"count": 1}
}
```

#### POST /api/v1/system/api-key

Create a new API key. The plaintext key is returned **once** in the response and cannot be retrieved again.

**Request body:**

```json
{
  "role_id": 2,
  "label": "CI pipeline",
  "expires_at": "2026-01-01T00:00:00Z"
}
```

Required field: `role_id`

**Response (201):**

```json
{
  "id": 1,
  "api_key": "faucet_a1b2c3d4e5f67890abcdef1234567890abcdef1234567890abcdef12345678",
  "key_prefix": "faucet_a1b2c3d",
  "label": "CI pipeline",
  "role_id": 2,
  "is_active": true,
  "created_at": "2025-01-15T10:30:00Z"
}
```

Save the `api_key` value immediately. It cannot be retrieved after this response.

#### DELETE /api/v1/system/api-key/{keyId}

Revoke (deactivate) an API key by ID.

---

## Table CRUD Endpoints

These endpoints operate on data in connected databases. The URL pattern is:

```
/api/v1/{serviceName}/_table/{tableName}
```

Where `{serviceName}` is the name of a registered database service and `{tableName}` is a table in that database.

### GET /api/v1/{serviceName}/_table

List all table names in the service's database.

**Response (200):**

```json
{
  "resource": [
    {"name": "users"},
    {"name": "orders"},
    {"name": "products"}
  ]
}
```

### GET /api/v1/{serviceName}/_table/{tableName}

Query records from a table.

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `filter` | string | -- | Filter expression ([syntax](filter-syntax.md)) |
| `fields` | string | all | Comma-separated column names to return |
| `order` | string | -- | Sort order (e.g., `name ASC, created_at DESC`) |
| `limit` | integer | 25 | Max records to return (capped at 1000) |
| `offset` | integer | 0 | Number of records to skip |
| `include_count` | boolean | false | Include total count in meta |
| `ids` | string | -- | Comma-separated primary key values |

**Examples:**

```bash
# Basic query
curl "http://localhost:8080/api/v1/mydb/_table/users"

# With filtering
curl "http://localhost:8080/api/v1/mydb/_table/users?filter=age%20%3E%2021%20AND%20status%20%3D%20'active'"

# Select specific fields
curl "http://localhost:8080/api/v1/mydb/_table/users?fields=id,name,email"

# With ordering and pagination
curl "http://localhost:8080/api/v1/mydb/_table/users?order=created_at%20DESC&limit=10&offset=20"

# Include total count
curl "http://localhost:8080/api/v1/mydb/_table/users?include_count=true&limit=10"
```

**Response (200):**

```json
{
  "resource": [
    {"id": 1, "name": "Alice", "email": "alice@example.com"},
    {"id": 2, "name": "Bob", "email": "bob@example.com"}
  ],
  "meta": {
    "count": 2,
    "limit": 25,
    "offset": 0,
    "took_ms": 1.23
  }
}
```

**NDJSON streaming:** Set `Accept: application/x-ndjson` to receive results as newline-delimited JSON (one JSON object per line). Useful for large result sets.

### POST /api/v1/{serviceName}/_table/{tableName}

Insert one or more records. Accepts three body formats:

**Single record:**

```json
{"name": "Alice", "email": "alice@example.com"}
```

**Array of records:**

```json
[
  {"name": "Alice", "email": "alice@example.com"},
  {"name": "Bob", "email": "bob@example.com"}
]
```

**Resource envelope (DreamFactory-compatible):**

```json
{
  "resource": [
    {"name": "Alice", "email": "alice@example.com"},
    {"name": "Bob", "email": "bob@example.com"}
  ]
}
```

**Response (201):** For databases that support `RETURNING` (PostgreSQL), the created records including auto-generated fields are returned. For others, the input records plus a row count are returned.

```json
{
  "resource": [
    {"id": 1, "name": "Alice", "email": "alice@example.com", "created_at": "2025-01-15T10:30:00Z"}
  ],
  "meta": {
    "count": 1,
    "took_ms": 2.45
  }
}
```

### PUT /api/v1/{serviceName}/_table/{tableName}

Replace records (full update). Each record in the body must include its primary key (`id` field) or a `filter` query parameter must be provided.

**Request body:**

```json
{
  "resource": [
    {"id": 1, "name": "Alice Updated", "email": "alice.new@example.com", "status": "active"}
  ]
}
```

### PATCH /api/v1/{serviceName}/_table/{tableName}

Partial update of records matching a filter or ID list. Only the provided fields are modified.

**Query parameters:** `filter` or `ids` (at least one required)

**Request body:**

```json
{"status": "archived"}
```

Or with IDs in the body:

```json
{
  "ids": [1, 2, 3],
  "status": "archived"
}
```

**Examples:**

```bash
# Update by filter
curl -X PATCH "http://localhost:8080/api/v1/mydb/_table/users?filter=status%20%3D%20'inactive'" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "archived"}'

# Update by IDs
curl -X PATCH "http://localhost:8080/api/v1/mydb/_table/users?ids=1,2,3" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"status": "archived"}'
```

### DELETE /api/v1/{serviceName}/_table/{tableName}

Delete records matching a filter or ID list. A filter or IDs must be provided to prevent accidental full-table deletes.

**Query parameters:** `filter` or `ids` (at least one required)

**Request body (alternative):**

```json
{"ids": [1, 2, 3]}
```

Or:

```json
{
  "resource": [
    {"id": 1},
    {"id": 2}
  ]
}
```

**Response (200):**

```json
{
  "meta": {
    "count": 3,
    "took_ms": 1.12
  }
}
```

---

## Schema Endpoints

Introspect and modify database schemas.

### GET /api/v1/{serviceName}/_schema

Get the full schema for a service, including all tables, views, columns, primary keys, foreign keys, and indexes.

### GET /api/v1/{serviceName}/_schema/{tableName}

Get the detailed schema for a single table.

**Response (200):**

```json
{
  "name": "users",
  "columns": [
    {
      "name": "id",
      "type": "integer",
      "is_primary_key": true,
      "is_nullable": false,
      "default": "nextval('users_id_seq'::regclass)"
    },
    {
      "name": "name",
      "type": "character varying(255)",
      "is_primary_key": false,
      "is_nullable": false
    },
    {
      "name": "email",
      "type": "character varying(255)",
      "is_primary_key": false,
      "is_nullable": true
    }
  ]
}
```

### POST /api/v1/{serviceName}/_schema

Create a new table.

**Request body:**

```json
{
  "name": "products",
  "columns": [
    {"name": "id", "type": "serial", "is_primary_key": true},
    {"name": "name", "type": "varchar(255)", "is_nullable": false},
    {"name": "price", "type": "decimal(10,2)"},
    {"name": "created_at", "type": "timestamp", "default": "now()"}
  ]
}
```

Returns the schema of the newly created table (201).

### PUT /api/v1/{serviceName}/_schema/{tableName}

Alter an existing table. Provide a list of schema changes.

**Request body:**

```json
{
  "changes": [
    {
      "type": "add_column",
      "column": "description",
      "definition": {"name": "description", "type": "text", "is_nullable": true}
    },
    {
      "type": "drop_column",
      "column": "old_field"
    },
    {
      "type": "rename_column",
      "column": "name",
      "new_name": "title"
    }
  ]
}
```

Supported change types: `add_column`, `drop_column`, `rename_column`, `modify_column`

### DELETE /api/v1/{serviceName}/_schema/{tableName}

Drop a table. This is irreversible.

**Response (200):**

```json
{
  "success": true,
  "message": "Table 'products' dropped successfully"
}
```

---

## Stored Procedure Endpoints

### GET /api/v1/{serviceName}/_proc

List all stored procedures and functions for a service.

**Response (200):**

```json
{
  "resource": [
    {
      "name": "calculate_total",
      "type": "function",
      "return_type": "numeric",
      "parameters": [
        {"name": "order_id", "type": "integer"}
      ]
    }
  ],
  "meta": {"count": 1}
}
```

### POST /api/v1/{serviceName}/_proc/{procName}

Call a stored procedure with parameters.

**Request body:**

```json
{
  "order_id": 42
}
```

Query parameters are also accepted and merged into the parameter map. This allows simple calls like:

```bash
curl -X POST "http://localhost:8080/api/v1/mydb/_proc/calculate_total?order_id=42" \
  -H "Authorization: Bearer $TOKEN"
```

**Response (200):**

```json
{
  "resource": [
    {"total": 299.99}
  ],
  "meta": {
    "count": 1,
    "took_ms": 5.67
  }
}
```

---

## Per-Service OpenAPI Spec

### GET /api/v1/{serviceName}/_doc

Returns an OpenAPI 3.1 specification for a single service, including schemas derived from actual table definitions.

---

## CORS

Faucet enables CORS with the following defaults:

- **Allowed origins:** `*`
- **Allowed methods:** GET, POST, PUT, PATCH, DELETE, OPTIONS
- **Allowed headers:** Accept, Authorization, Content-Type, X-API-Key, X-Requested-With
- **Exposed headers:** X-Total-Count, X-Request-ID, Link
- **Max age:** 300 seconds

---

## Rate Limiting

Request rate limiting middleware is available. See [Deployment](deployment.md) for configuration.

---

## Request Size Limit

The maximum request body size is 10 MB by default.
