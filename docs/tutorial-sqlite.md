# Tutorial: SQLite

This tutorial walks you through connecting a SQLite database to Faucet and querying it via the REST API. SQLite is perfect for development, prototyping, embedded applications, and datasets that don't need a full database server.

## Prerequisites

- Faucet installed and running (`faucet serve`)
- An admin account created (`faucet admin create`)
- A SQLite database file (or we'll create one)

## Step 1: Create a Sample Database (Optional)

If you don't have a SQLite database yet, create one:

```bash
sqlite3 ~/mydata.db <<'SQL'
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    published BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

INSERT INTO users (name, email) VALUES
    ('Alice', 'alice@example.com'),
    ('Bob', 'bob@example.com'),
    ('Charlie', 'charlie@example.com');

INSERT INTO posts (user_id, title, body, published) VALUES
    (1, 'Hello World', 'My first post!', 1),
    (1, 'SQLite Tips', 'SQLite is great for prototyping.', 1),
    (2, 'Draft Post', 'Work in progress...', 0);
SQL
```

## Step 2: Connect SQLite to Faucet

The DSN for SQLite is simply the file path, optionally with query parameters.

### Via CLI

```bash
faucet db add \
  --name mydata \
  --driver sqlite \
  --dsn "/Users/you/mydata.db"
```

With WAL mode (recommended for concurrent access):

```bash
faucet db add \
  --name mydata \
  --driver sqlite \
  --dsn "/Users/you/mydata.db?_journal_mode=WAL"
```

### Via API

```bash
TOKEN=$(curl -s http://localhost:8080/api/v1/system/admin/session \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"changeme123"}' \
  | jq -r '.session_token')

curl -X POST http://localhost:8080/api/v1/system/service \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "mydata",
    "driver": "sqlite",
    "dsn": "/Users/you/mydata.db?_journal_mode=WAL"
  }'
```

### Via Admin UI

1. Open `http://localhost:8080/admin`
2. Navigate to **Services**
3. Click **Add Service**
4. Select driver: **SQLite**
5. Enter the service name and file path as DSN
6. Click **Save**

## Step 3: Verify the Connection

```bash
faucet db test mydata
```

Or list tables via the API:

```bash
curl http://localhost:8080/api/v1/mydata/_table \
  -H "Authorization: Bearer $TOKEN"
```

Expected response:

```json
{
  "resource": [
    {"name": "posts"},
    {"name": "users"}
  ]
}
```

## Step 4: Explore the Schema

View table structure:

```bash
curl http://localhost:8080/api/v1/mydata/_schema/users \
  -H "Authorization: Bearer $TOKEN"
```

Response shows columns, types, primary keys, and foreign keys:

```json
{
  "name": "users",
  "type": "table",
  "columns": [
    {"name": "id", "db_type": "INTEGER", "is_primary_key": true, "is_auto_increment": true},
    {"name": "name", "db_type": "TEXT", "nullable": false},
    {"name": "email", "db_type": "TEXT", "nullable": false},
    {"name": "created_at", "db_type": "DATETIME", "nullable": true}
  ],
  "primary_key": ["id"]
}
```

## Step 5: Query Your Data

### List all records

```bash
curl "http://localhost:8080/api/v1/mydata/_table/users" \
  -H "Authorization: Bearer $TOKEN"
```

### Filter records

```bash
curl "http://localhost:8080/api/v1/mydata/_table/posts?filter=published%20%3D%201" \
  -H "Authorization: Bearer $TOKEN"
```

### Select specific fields

```bash
curl "http://localhost:8080/api/v1/mydata/_table/users?fields=name,email" \
  -H "Authorization: Bearer $TOKEN"
```

### Paginate

```bash
curl "http://localhost:8080/api/v1/mydata/_table/posts?limit=10&offset=0&order=created_at%20DESC" \
  -H "Authorization: Bearer $TOKEN"
```

### Count records

```bash
curl "http://localhost:8080/api/v1/mydata/_table/posts?include_count=true" \
  -H "Authorization: Bearer $TOKEN"
```

## Step 6: Insert, Update, and Delete

### Insert a record

```bash
curl -X POST http://localhost:8080/api/v1/mydata/_table/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"name": "Diana", "email": "diana@example.com"}]}'
```

SQLite supports RETURNING, so the response includes the server-generated ID:

```json
{
  "resource": [
    {"id": 4, "name": "Diana", "email": "diana@example.com", "created_at": "2024-01-15 10:30:00"}
  ]
}
```

### Update a record

```bash
curl -X PATCH "http://localhost:8080/api/v1/mydata/_table/users?ids=4" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Diana Prince"}'
```

### Delete a record

```bash
curl -X DELETE "http://localhost:8080/api/v1/mydata/_table/users?ids=4" \
  -H "Authorization: Bearer $TOKEN"
```

## SQLite-Specific Notes

### DSN format

The DSN is a file path with optional query parameters:

```
/path/to/database.db?param1=value1&param2=value2
```

Common parameters:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `_journal_mode` | `delete` | Set to `WAL` for better concurrent read performance |
| `_busy_timeout` | `5000` | Milliseconds to wait when the database is locked |
| `_foreign_keys` | `on` | Enable foreign key enforcement |
| `_cache_size` | `-2000` | Page cache size (negative = KB, positive = pages) |

### In-memory databases

Use `:memory:` as the DSN for an ephemeral in-memory database (useful for testing):

```bash
faucet db add --name scratch --driver sqlite --dsn ":memory:"
```

Note: In-memory databases are lost when Faucet restarts.

### WAL mode is recommended

For any production use, enable WAL (Write-Ahead Logging) mode. It allows concurrent reads while writing:

```bash
faucet db add \
  --name mydata \
  --driver sqlite \
  --dsn "/path/to/db.sqlite?_journal_mode=WAL"
```

### SQLite supports RETURNING

Unlike MySQL and Snowflake, SQLite (3.35+) supports RETURNING clauses. INSERT and UPDATE responses include the actual server-generated values (auto-increment IDs, defaults).

### No stored procedures

SQLite does not support stored procedures. The `_proc` endpoint will return an error for SQLite services.

### No schema qualification

Unlike PostgreSQL and MySQL, SQLite tables are not schema-qualified. The default schema is `main` but table names are used directly (e.g., `SELECT * FROM "users"` not `SELECT * FROM "main"."users"`).

### Connection pooling

SQLite is an embedded database, so connection pool settings are less critical than with network databases. Recommended settings:

```json
{
  "pool": {
    "max_open_conns": 1,
    "max_idle_conns": 1,
    "conn_max_lifetime": "0",
    "conn_max_idle_time": "0"
  }
}
```

Setting `max_open_conns` to 1 avoids `SQLITE_BUSY` errors since SQLite supports only one writer at a time.

### Supported features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | Yes (SQLite 3.35+) |
| Upsert (ON CONFLICT) | Yes |
| Stored procedures | No |
| Views | Yes |
| Parameter style | `?` |
| Default schema | `main` |

## Use Cases

SQLite is ideal for:

- **Development and prototyping** -- no server to install
- **Small to medium datasets** -- millions of rows, single-digit GB
- **Edge/embedded** -- run Faucet on an IoT device or edge server
- **Single-user applications** -- desktop apps, personal tools
- **Read-heavy workloads** -- analytics dashboards, reporting

For high-concurrency write workloads, consider PostgreSQL or MySQL instead.

## Troubleshooting

### "database is locked"

SQLite allows only one writer at a time. Solutions:
- Enable WAL mode (`?_journal_mode=WAL`)
- Set `max_open_conns` to 1
- Increase busy timeout (`?_busy_timeout=10000`)

### "no such table"

The table doesn't exist in the database file. Verify with:
```bash
sqlite3 /path/to/db.sqlite ".tables"
```

### "unable to open database file"

The file path in the DSN doesn't exist or Faucet doesn't have read/write permission. Use an absolute path and check file permissions.

### Foreign keys not enforced

SQLite has foreign keys disabled by default. Add `?_foreign_keys=on` to the DSN to enable them.

## What's Next

- [Filter Syntax](filter-syntax.md) -- query filtering operators
- [RBAC](rbac.md) -- restrict access per role
- [API Reference](api-reference.md) -- full REST API documentation
- [Database Connectors](database-connectors.md) -- all connector DSN formats
