# Tutorial: SQL Server

This tutorial walks you through connecting a SQL Server database to Faucet and querying it via the REST API.

## Prerequisites

- Faucet installed and running (`faucet serve`)
- An admin account created (`faucet admin create`)
- A SQL Server instance (2016+) with a database and at least one table

## Step 1: Connect SQL Server to Faucet

The DSN format for SQL Server:

```
sqlserver://username:password@host:port?database=dbname
```

### Via CLI

```bash
faucet db add \
  --name mydb \
  --driver mssql \
  --dsn "sqlserver://sa:YourStrong!Passw0rd@localhost:1433?database=mydb"
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
    "driver": "mssql",
    "dsn": "sqlserver://sa:YourStrong!Passw0rd@localhost:1433?database=mydb"
  }'
```

### Via Admin UI

1. Open `http://localhost:8080/admin`
2. Navigate to **Services**
3. Click **Add Service**
4. Select driver: **SQL Server**
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
    {"name": "Customers"},
    {"name": "Orders"},
    {"name": "Products"}
  ]
}
```

## Step 3: Query Your Data

### List all records

```bash
curl "http://localhost:8080/api/v1/mydb/_table/Customers?limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

### Filter records

```bash
curl "http://localhost:8080/api/v1/mydb/_table/Orders?filter=Status%20%3D%20'Shipped'" \
  -H "Authorization: Bearer $TOKEN"
```

### Select specific fields

```bash
curl "http://localhost:8080/api/v1/mydb/_table/Customers?fields=Name,Email" \
  -H "Authorization: Bearer $TOKEN"
```

### Paginate

```bash
curl "http://localhost:8080/api/v1/mydb/_table/Orders?limit=10&offset=0&order=CreatedAt%20DESC" \
  -H "Authorization: Bearer $TOKEN"
```

### Count records

```bash
curl "http://localhost:8080/api/v1/mydb/_table/Orders?include_count=true" \
  -H "Authorization: Bearer $TOKEN"
```

## Step 4: Insert, Update, and Delete

### Insert a record

```bash
curl -X POST http://localhost:8080/api/v1/mydb/_table/Customers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"Name": "Acme Corp", "Email": "contact@acme.com"}]}'
```

SQL Server supports RETURNING via OUTPUT clause, so the response includes server-generated values:

```json
{
  "resource": [
    {"Id": 42, "Name": "Acme Corp", "Email": "contact@acme.com", "CreatedAt": "2024-01-15T10:30:00Z"}
  ]
}
```

### Update a record

```bash
curl -X PATCH "http://localhost:8080/api/v1/mydb/_table/Customers?ids=42" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"Name": "Acme Corporation"}'
```

### Delete a record

```bash
curl -X DELETE "http://localhost:8080/api/v1/mydb/_table/Customers?ids=42" \
  -H "Authorization: Bearer $TOKEN"
```

## SQL Server-Specific Notes

### OUTPUT clause (RETURNING equivalent)

SQL Server uses OUTPUT INSERTED.* instead of RETURNING. Faucet handles this transparently -- INSERT and UPDATE responses include the actual row data.

### Upsert support

SQL Server supports upsert via MERGE:

```bash
curl -X POST "http://localhost:8080/api/v1/mydb/_table/Customers?upsert=true" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"Id": 42, "Name": "Updated Name", "Email": "new@example.com"}]}'
```

### Schema selection

SQL Server defaults to the `dbo` schema. To use a different schema:

```bash
faucet db add \
  --name mydb \
  --driver mssql \
  --dsn "sqlserver://sa:pass@localhost:1433?database=mydb" \
  --schema sales
```

### Stored procedures

Faucet can execute SQL Server stored procedures:

```bash
curl -X POST http://localhost:8080/api/v1/mydb/_proc/GetActiveUsers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"params": {"MinAge": 18}}'
```

### Supported features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | Via OUTPUT clause |
| Upsert (MERGE) | Yes |
| Stored procedures | Yes |
| Views | Yes |
| Parameter style | `@p1, @p2, @p3` |
| Default schema | `dbo` |

### Docker for local development

Run SQL Server locally with Docker:

```bash
docker run -e "ACCEPT_EULA=Y" -e "SA_PASSWORD=YourStrong!Passw0rd" \
  -p 1433:1433 --name sqlserver \
  -d mcr.microsoft.com/mssql/server:2022-latest
```

Then connect:

```bash
faucet db add \
  --name localdb \
  --driver mssql \
  --dsn "sqlserver://sa:YourStrong!Passw0rd@localhost:1433?database=master"
```

## Troubleshooting

### "Login failed for user"

Check your username and password. For `sa` account, ensure the SA_PASSWORD meets SQL Server's complexity requirements.

### "Cannot open server requested by the login"

The database specified in `?database=` doesn't exist. Create it first or connect to an existing database.

### "TLS Handshake failed"

Add `&encrypt=disable` for local development, or `&TrustServerCertificate=true` for self-signed certificates. For production, configure proper TLS certificates.

### "connection refused"

SQL Server isn't running, the port is wrong, or TCP/IP isn't enabled. Check SQL Server Configuration Manager and ensure TCP/IP is enabled on port 1433.

## What's Next

- [Filter Syntax](filter-syntax.md) -- query filtering operators
- [RBAC](rbac.md) -- restrict access per role
- [API Reference](api-reference.md) -- full REST API documentation
- [Database Connectors](database-connectors.md) -- all connector DSN formats
