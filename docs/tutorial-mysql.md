# Tutorial: MySQL

This tutorial walks you through connecting a MySQL database to Faucet and querying it via the REST API.

## Prerequisites

- Faucet installed and running (`faucet serve`)
- An admin account created (`faucet admin create`)
- A MySQL server (5.7+ or 8.0+) with a database and at least one table

## Step 1: Connect MySQL to Faucet

The DSN format for MySQL:

```
username:password@tcp(host:port)/database?parseTime=true
```

### Via CLI

```bash
faucet db add \
  --name mydb \
  --driver mysql \
  --dsn "root:password@tcp(localhost:3306)/mydb?parseTime=true"
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
    "driver": "mysql",
    "dsn": "root:password@tcp(localhost:3306)/mydb?parseTime=true"
  }'
```

### Via Admin UI

1. Open `http://localhost:8080/admin`
2. Navigate to **Services**
3. Click **Add Service**
4. Select driver: **MySQL**
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

Note: MySQL does not support RETURNING, so the response echoes back the submitted data. Use a subsequent GET to retrieve auto-incremented IDs.

### Update a record

```bash
curl -X PATCH "http://localhost:8080/api/v1/mydb/_table/customers?ids=1" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Corporation"}'
```

### Delete a record

```bash
curl -X DELETE "http://localhost:8080/api/v1/mydb/_table/customers?ids=1" \
  -H "Authorization: Bearer $TOKEN"
```

## MySQL-Specific Notes

### Always use parseTime=true

The `parseTime=true` parameter is strongly recommended. Without it, MySQL DATE and DATETIME columns are returned as byte strings instead of proper timestamps.

### Stored procedures

Faucet can execute MySQL stored procedures:

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
| RETURNING clause | No (MySQL < 8.0.21) |
| Upsert (ON DUPLICATE KEY) | Yes |
| Stored procedures | Yes |
| Views | Yes |
| Parameter style | `?` |
| Default schema | database name |

## Troubleshooting

### "Access denied for user"

Check that your username, password, and host are correct. MySQL restricts access by host -- a user allowed from `localhost` may not be allowed from `127.0.0.1`.

### "Unknown database"

The database specified in the DSN doesn't exist. Create it first:
```sql
CREATE DATABASE mydb;
```

### "dial tcp: connect: connection refused"

MySQL isn't running on the specified host/port, or a firewall is blocking the connection.

### Timezone issues

If timestamps look wrong, add `&loc=Local` or `&loc=UTC` to the DSN to control timezone interpretation.

## What's Next

- [Filter Syntax](filter-syntax.md) -- query filtering operators
- [RBAC](rbac.md) -- restrict access per role
- [API Reference](api-reference.md) -- full REST API documentation
- [Database Connectors](database-connectors.md) -- all connector DSN formats
