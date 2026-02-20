# Tutorial: PostgreSQL

This tutorial walks you through connecting a PostgreSQL database to Faucet and querying it via the REST API.

## Prerequisites

- Faucet installed and running (`faucet serve`)
- An admin account created (`faucet admin create`)
- A PostgreSQL server (12+) with a database and at least one table

## Step 1: Connect PostgreSQL to Faucet

The DSN format for PostgreSQL:

```
postgres://username:password@host:port/database?sslmode=disable
```

### Via CLI

```bash
faucet db add \
  --name mydb \
  --driver postgres \
  --dsn "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
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
    "name": "mydb",
    "driver": "postgres",
    "dsn": "postgres://user:pass@localhost:5432/mydb?sslmode=disable"
  }'
```

### Via Admin UI

1. Open `http://localhost:8080/admin`
2. Navigate to **Services**
3. Click **Add Service**
4. Select driver: **PostgreSQL**
5. Enter the service name and DSN
6. Click **Save**

## Step 2: Verify the Connection

```bash
faucet db test mydb
```

Or list tables via the API:

```bash
curl http://localhost:8080/api/v1/mydb/_table \
  -H "Authorization: Bearer $TOKEN"
```

Expected response:

```json
{
  "resource": [
    {"name": "customers"},
    {"name": "orders"},
    {"name": "products"}
  ]
}
```

## Step 3: Query Your Data

### List all records

```bash
curl "http://localhost:8080/api/v1/mydb/_table/customers?limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

### Filter records

```bash
curl "http://localhost:8080/api/v1/mydb/_table/orders?filter=status%20%3D%20'shipped'" \
  -H "Authorization: Bearer $TOKEN"
```

### Select specific fields

```bash
curl "http://localhost:8080/api/v1/mydb/_table/customers?fields=name,email" \
  -H "Authorization: Bearer $TOKEN"
```

### Paginate

```bash
curl "http://localhost:8080/api/v1/mydb/_table/orders?limit=10&offset=0&order=created_at%20DESC" \
  -H "Authorization: Bearer $TOKEN"
```

### Count records

```bash
curl "http://localhost:8080/api/v1/mydb/_table/orders?include_count=true" \
  -H "Authorization: Bearer $TOKEN"
```

## Step 4: Insert, Update, and Delete

### Insert a record

```bash
curl -X POST http://localhost:8080/api/v1/mydb/_table/customers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"name": "Acme Corp", "email": "contact@acme.com"}]}'
```

PostgreSQL supports RETURNING, so the response includes server-generated values:

```json
{
  "resource": [
    {"id": 42, "name": "Acme Corp", "email": "contact@acme.com", "created_at": "2024-01-15T10:30:00Z"}
  ]
}
```

### Update a record

```bash
curl -X PATCH "http://localhost:8080/api/v1/mydb/_table/customers?ids=42" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Corporation"}'
```

### Delete a record

```bash
curl -X DELETE "http://localhost:8080/api/v1/mydb/_table/customers?ids=42" \
  -H "Authorization: Bearer $TOKEN"
```

## PostgreSQL-Specific Notes

### RETURNING clause

PostgreSQL fully supports RETURNING. INSERT and UPDATE responses include the actual row data with server-generated values (serial IDs, defaults, triggers).

### Schema selection

PostgreSQL organizes tables into schemas. The default is `public`. To use a different schema:

```bash
faucet db add \
  --name mydb \
  --driver postgres \
  --dsn "postgres://user:pass@localhost/mydb?sslmode=disable&search_path=myschema" \
  --schema myschema
```

### Stored procedures

Faucet can execute PostgreSQL stored procedures and functions:

```bash
curl -X POST http://localhost:8080/api/v1/mydb/_proc/get_active_users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"params": {"min_age": 18}}'
```

### Supported features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | Yes |
| Upsert (ON CONFLICT) | Yes |
| Stored procedures | Yes |
| Views | Yes |
| Parameter style | `$1, $2, $3` |
| Default schema | `public` |

## Troubleshooting

### "password authentication failed"

Check your username and password. PostgreSQL uses `pg_hba.conf` to control authentication methods per host.

### "no pg_hba.conf entry for host"

Your client IP is not allowed in `pg_hba.conf`. Add an entry for your IP or use `sslmode=require`.

### "SSL is not enabled on the server"

Add `?sslmode=disable` to the DSN for local development, or enable SSL on your PostgreSQL server for production.

### "relation does not exist"

The table doesn't exist in the specified schema. Check your `search_path` or `--schema` setting, and verify the table exists:
```sql
\dt myschema.*
```

## What's Next

- [Filter Syntax](filter-syntax.md) -- query filtering operators
- [RBAC](rbac.md) -- restrict access per role
- [API Reference](api-reference.md) -- full REST API documentation
- [Database Connectors](database-connectors.md) -- all connector DSN formats
