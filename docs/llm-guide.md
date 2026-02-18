# LLM Integration Guide

This guide is for AI agents and LLMs that want to interact with databases through Faucet. It covers both the MCP protocol (preferred) and the REST API.

## Quick Context for LLMs

Faucet is a database-to-API gateway. You point it at SQL databases (PostgreSQL, MySQL, SQL Server, Snowflake) and it generates REST APIs automatically. You can interact with these APIs via:

1. **MCP tools** (recommended) — structured, validated tool calls
2. **REST API** — standard HTTP with JSON payloads

## MCP Integration (Preferred)

If you're running as an MCP client (Claude Desktop, Claude Code, etc.), Faucet exposes these tools:

### Discovery (always start here)

```
faucet_list_services     → What databases are connected?
faucet_list_tables       → What tables exist in a database?
faucet_describe_table    → What columns/types does a table have?
```

### Reading Data

```
faucet_query             → SELECT with filter, fields, order, limit, offset
faucet_raw_sql           → Execute arbitrary SQL (if enabled for the service)
```

### Writing Data

```
faucet_insert            → INSERT records
faucet_update            → UPDATE records matching a filter
faucet_delete            → DELETE records matching a filter
```

### Recommended Workflow

1. Call `faucet_list_services` to discover available databases
2. Call `faucet_list_tables` on the relevant service
3. Call `faucet_describe_table` for tables you need to query
4. Use `faucet_query` with appropriate filters to get data

### Filter Syntax Reference

Filters use SQL-like syntax with these operators:

```
column = value                    Equality
column != value                   Inequality
column > value                    Greater than
column >= value                   Greater than or equal
column < value                    Less than
column <= value                   Less than or equal
column LIKE 'pattern'             Pattern match (% = wildcard)
column NOT LIKE 'pattern'         Negated pattern match
column IN ('a','b','c')           Set membership
column NOT IN ('a','b')           Negated set membership
column BETWEEN 10 AND 20          Range
column IS NULL                    Null check
column IS NOT NULL                Not null check
(expr1) AND (expr2)               Logical AND
(expr1) OR (expr2)                Logical OR
```

String values must be quoted with single quotes. Numeric values are unquoted.

### Error Recovery

If a tool call fails, the error message includes helpful context:
- **Service not found**: Lists available services
- **Table not found**: Lists available tables in that service
- **Invalid filter**: Shows filter syntax reference
- **Column not found**: Lists available columns

Use this context to self-correct without asking the user.

## REST API Integration

### Authentication

All data endpoints require authentication via one of:

```
X-API-Key: faucet_your_key_here
# OR
Authorization: Bearer eyJhbGci...
```

Admin operations (managing services, roles, keys) require JWT auth.

### Key Endpoints

| Method | URL | Description |
|--------|-----|-------------|
| GET | `/api/v1/{service}/_table` | List tables |
| GET | `/api/v1/{service}/_table/{table}` | Query records |
| POST | `/api/v1/{service}/_table/{table}` | Insert records |
| PATCH | `/api/v1/{service}/_table/{table}` | Update records |
| DELETE | `/api/v1/{service}/_table/{table}` | Delete records |
| GET | `/api/v1/{service}/_schema` | Full schema |
| GET | `/api/v1/{service}/_schema/{table}` | Table schema |
| POST | `/api/v1/{service}/_proc/{proc}` | Call stored procedure |
| GET | `/openapi.json` | Combined OpenAPI 3.1 spec |

### Query Parameters for GET /_table/{table}

| Parameter | Example | Description |
|-----------|---------|-------------|
| `filter` | `age > 21 AND status = 'active'` | WHERE clause |
| `fields` | `id,name,email` | SELECT specific columns |
| `order` | `created_at DESC` | ORDER BY clause |
| `limit` | `25` | Max rows (default 25, max 1000) |
| `offset` | `50` | Skip N rows |
| `include_count` | `true` | Include total count in meta |

### Response Envelope

All list responses use this format:

```json
{
  "resource": [ ... records ... ],
  "meta": {
    "count": 25,
    "limit": 25,
    "offset": 0
  }
}
```

### Batch Operations

Insert, update, and delete support batch operations:

```json
{
  "resource": [
    {"name": "Alice", "email": "alice@example.com"},
    {"name": "Bob", "email": "bob@example.com"}
  ]
}
```

## OpenAPI Spec

Faucet generates an OpenAPI 3.1 specification at `/openapi.json` that describes all endpoints, request/response schemas derived from actual database column types, and available operations. This can be used for:
- Auto-generating client SDKs
- Validating requests
- Providing schema context to AI agents

## Performance Tips for LLMs

1. **Use `fields` parameter** to select only needed columns — reduces token count
2. **Use `limit`** — don't fetch all records when you need a sample
3. **Start with schema discovery** — understand the data model before querying
4. **Use `faucet_query` over `faucet_raw_sql`** — structured queries are safer
5. **Chain filters** — combine conditions in one query rather than fetching all and filtering
