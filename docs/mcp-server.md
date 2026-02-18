# MCP Server

Faucet includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that allows AI agents like Claude to directly query and modify your databases through structured tool calls.

## What is MCP?

The Model Context Protocol is an open standard for connecting AI assistants to external data sources and tools. Instead of generating SQL from natural language (with risk of errors and injection), MCP provides structured, type-safe tools that the AI can call with validated parameters.

Faucet's MCP server exposes:
- **Tools** -- callable functions for querying, inserting, updating, and deleting data
- **Resources** -- read-only data that AI clients can load into context (service lists, schemas)

## Transport Modes

### stdio Mode (default)

The MCP server communicates over stdin/stdout using JSON-RPC. This is the standard integration path for Claude Desktop, Claude Code, and other MCP clients that launch the server as a subprocess.

```bash
faucet mcp
```

### HTTP Mode

The MCP server listens for Streamable HTTP connections on a specified port. This is suitable for remote MCP clients or multi-agent architectures.

```bash
faucet mcp --transport http --port 3001
```

## Available Tools

### Discovery Tools

These are read-only tools for exploring available databases and schemas.

#### faucet_list_services

List all database services configured in Faucet.

**Parameters:** None

**Returns:** Array of services with name, driver, active status, read-only flag, and raw SQL permission.

```json
[
  {
    "name": "mydb",
    "label": "My Database",
    "driver": "postgres",
    "is_active": true,
    "read_only": false,
    "raw_sql_allowed": true
  }
]
```

#### faucet_list_tables

List all tables in a database service with column summaries and approximate row counts.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |

**Returns:** Array of tables/views with column names, types, primary key indicators, and row counts.

```json
[
  {
    "name": "users",
    "type": "table",
    "row_count": 15000,
    "columns": [
      {"name": "id", "type": "integer", "pk": true},
      {"name": "name", "type": "varchar(255)"},
      {"name": "email", "type": "varchar(255)"}
    ]
  },
  {
    "name": "active_users",
    "type": "view",
    "columns": [
      {"name": "id", "type": "integer"},
      {"name": "name", "type": "varchar(255)"}
    ]
  }
]
```

#### faucet_describe_table

Get detailed schema for a specific table, including all columns, types, nullability, defaults, primary keys, foreign keys, and indexes.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |
| `table` | Yes | Name of the table to describe |

**Returns:** Full table schema object. If the table is not found, the error message includes a list of available tables to help the AI self-correct.

### Query Tool

#### faucet_query

Query records from a database table with optional filtering, field selection, ordering, and pagination. Results are returned as JSON.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |
| `table` | Yes | Name of the table to query |
| `filter` | No | Filter expression (see [Filter Syntax](filter-syntax.md)) |
| `fields` | No | Array of column names to return (omit for all) |
| `order` | No | Order clause (e.g., `"created_at DESC, name ASC"`) |
| `limit` | No | Max records (default 25, max 1000) |
| `offset` | No | Records to skip for pagination |

**Returns:**

```json
{
  "records": [
    {"id": 1, "name": "Alice", "email": "alice@example.com"}
  ],
  "count": 1,
  "limit": 25,
  "offset": 0
}
```

### Mutation Tools

These tools modify data. They check the service's `read_only` flag and refuse operations on read-only services.

#### faucet_insert

Insert one or more records into a table.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |
| `table` | Yes | Name of the table |
| `records` | Yes | Array of record objects to insert |

**Example call:**

```json
{
  "service": "mydb",
  "table": "users",
  "records": [
    {"name": "Alice", "email": "alice@example.com", "age": 30}
  ]
}
```

**Returns:** Inserted records (with auto-generated fields like IDs on databases that support RETURNING) and count.

#### faucet_update

Update records matching a filter expression.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |
| `table` | Yes | Name of the table |
| `filter` | Yes | Filter to select records (required to prevent full-table updates) |
| `record` | Yes | Object with column names and new values |

**Example call:**

```json
{
  "service": "mydb",
  "table": "users",
  "filter": "id = 42",
  "record": {"status": "archived"}
}
```

#### faucet_delete

Delete records matching a filter expression.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |
| `table` | Yes | Name of the table |
| `filter` | Yes | Filter to select records (required to prevent full-table deletes) |

**Returns:** Count of deleted records.

### Raw SQL Tool

#### faucet_raw_sql

Execute a raw SQL query against a service. Only available for services with `raw_sql_allowed` enabled.

**Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `service` | Yes | Name of the database service |
| `sql` | Yes | SQL query to execute |
| `params` | No | Positional parameters for the query |
| `timeout` | No | Query timeout in seconds (default 30, max 300) |
| `limit` | No | Max rows to return (default 100, max 10000) |

**Example call:**

```json
{
  "service": "mydb",
  "sql": "SELECT u.name, COUNT(o.id) as order_count FROM users u LEFT JOIN orders o ON o.user_id = u.id GROUP BY u.name ORDER BY order_count DESC LIMIT 10",
  "timeout": 30
}
```

**Returns:**

```json
{
  "records": [...],
  "count": 10,
  "truncated": false
}
```

If results exceed the limit, `truncated` is `true` and a helpful message is included.

## Available Resources

MCP resources provide read-only data that AI clients can load into their context.

### faucet://services

List of all connected database services with their driver type, active status, and permissions.

**URI:** `faucet://services`
**MIME type:** `application/json`

### faucet://schema/{service}

Full schema introspection for a database service, including tables, columns, primary keys, foreign keys, and indexes.

**URI template:** `faucet://schema/{service}`
**MIME type:** `application/json`

Example: `faucet://schema/mydb` returns the complete schema for the `mydb` service.

## Claude Desktop Configuration

Add Faucet to your Claude Desktop `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "faucet": {
      "command": "faucet",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

If Faucet is not in your PATH, use the full path:

```json
{
  "mcpServers": {
    "faucet": {
      "command": "/usr/local/bin/faucet",
      "args": ["mcp"],
      "env": {}
    }
  }
}
```

### With a Custom Data Directory

```json
{
  "mcpServers": {
    "faucet": {
      "command": "faucet",
      "args": ["mcp", "--data-dir", "/path/to/faucet/data"],
      "env": {}
    }
  }
}
```

## Claude Code Configuration

Add to your `.mcp.json` or project MCP configuration:

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

## HTTP Mode for Remote Access

For shared or remote MCP access:

```bash
faucet mcp --transport http --port 3001
```

Then configure your MCP client to connect via HTTP:

```json
{
  "mcpServers": {
    "faucet": {
      "url": "http://your-server:3001"
    }
  }
}
```

## Usage Examples

### Example 1: Data Exploration

A typical AI agent interaction flow:

1. **Agent calls `faucet_list_services`** to discover databases
2. **Agent calls `faucet_list_tables`** on the relevant service to see available data
3. **Agent calls `faucet_describe_table`** to understand the schema of tables of interest
4. **Agent calls `faucet_query`** with appropriate filters to answer the user's question

### Example 2: Natural Language to Database Query

User: "How many active users signed up last month?"

The AI agent would:

```
1. faucet_list_services -> ["mydb"]
2. faucet_describe_table(service="mydb", table="users")
   -> columns: id, name, email, status, created_at, ...
3. faucet_query(
     service="mydb",
     table="users",
     filter="status = 'active' AND created_at BETWEEN '2025-01-01' AND '2025-01-31'",
     fields=["id"],
     limit=1
   )
   -> {"records": [...], "count": 42}
```

### Example 3: Data Modification

User: "Mark all orders from before 2024 as archived"

```
1. faucet_query(
     service="mydb",
     table="orders",
     filter="created_at < '2024-01-01' AND status != 'archived'",
     fields=["id"],
     limit=1
   )
   -> Shows matching count to confirm scope

2. faucet_update(
     service="mydb",
     table="orders",
     filter="created_at < '2024-01-01' AND status != 'archived'",
     record={"status": "archived"}
   )
   -> {"updated": [...], "count": 156}
```

### Example 4: Cross-Table Analysis with Raw SQL

User: "What are the top 10 customers by total order value?"

```
faucet_raw_sql(
  service="mydb",
  sql="SELECT c.name, SUM(o.total) as total_value FROM customers c JOIN orders o ON o.customer_id = c.id GROUP BY c.id, c.name ORDER BY total_value DESC LIMIT 10"
)
```

## Error Handling

MCP tools return helpful error messages that include context for the AI to self-correct:

- **Service not found:** Includes list of available services
- **Table not found:** Includes list of available tables
- **Invalid filter:** Includes filter syntax reference
- **Invalid fields:** Includes list of available columns
- **Read-only violation:** Suggests using structured query tools
- **Raw SQL disabled:** Suggests alternatives

This design allows AI agents to recover from errors without user intervention.

## Tool Annotations

Each MCP tool is annotated with read/write metadata:

- **Read-only tools** (`faucet_list_services`, `faucet_list_tables`, `faucet_describe_table`, `faucet_query`, `faucet_raw_sql`): Marked with `readOnlyHint: true`
- **Mutating tools** (`faucet_insert`, `faucet_update`, `faucet_delete`): Marked with `readOnlyHint: false`

MCP clients can use these annotations to require user confirmation before executing mutating operations.
