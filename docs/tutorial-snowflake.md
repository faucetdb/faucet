# Tutorial: Snowflake

This tutorial walks you through connecting a Snowflake data warehouse to Faucet and querying it via the REST API.

## Prerequisites

- Faucet installed and running (`faucet serve`)
- An admin account created (`faucet admin create`)
- A Snowflake account with:
  - Account identifier (e.g., `myorg-myaccount`)
  - Username and password **OR** an RSA key pair for JWT authentication
  - A warehouse name (e.g., `COMPUTE_WH`)
  - A database with at least one table

## Step 1: Gather Your Connection Details

You'll need these Snowflake values:

| Value | Example | Where to find it |
|-------|---------|-----------------|
| Account identifier | `myorg-myaccount` | Snowflake UI → Account → Locator |
| Username | `ANALYST_USER` | Your Snowflake login |
| Password | `s3cretP@ss` | Your Snowflake password |
| Database | `ANALYTICS` | The database to expose |
| Schema | `PUBLIC` | Schema within the database (default: `PUBLIC`) |
| Warehouse | `COMPUTE_WH` | A running warehouse for queries |
| Role (optional) | `ANALYST` | Snowflake role for access control |

Your DSN will look like:

```
username:password@account_identifier/database/schema?warehouse=WAREHOUSE&role=ROLE
```

## Step 2: Connect Snowflake to Faucet

### Via CLI

```bash
faucet db add \
  --name analytics \
  --driver snowflake \
  --dsn "ANALYST_USER:s3cretP@ss@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH&role=ANALYST"
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
    "name": "analytics",
    "driver": "snowflake",
    "dsn": "ANALYST_USER:s3cretP@ss@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH&role=ANALYST"
  }'
```

### Via Admin UI

1. Open `http://localhost:8080/admin`
2. Navigate to **Services**
3. Click **Add Service**
4. Select driver: **Snowflake**
5. Enter the service name and DSN
6. Click **Save**

## Step 3: Verify the Connection

```bash
faucet db test analytics
```

Or list tables via the API:

```bash
curl http://localhost:8080/api/v1/analytics/_table \
  -H "Authorization: Bearer $TOKEN"
```

Expected response:

```json
{
  "resource": [
    {"name": "CUSTOMERS"},
    {"name": "ORDERS"},
    {"name": "PRODUCTS"}
  ]
}
```

## Step 4: Query Your Data

### List all records

```bash
curl "http://localhost:8080/api/v1/analytics/_table/CUSTOMERS?limit=5" \
  -H "Authorization: Bearer $TOKEN"
```

### Filter records

```bash
curl "http://localhost:8080/api/v1/analytics/_table/ORDERS?filter=STATUS%20%3D%20'SHIPPED'&limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

### Select specific fields

```bash
curl "http://localhost:8080/api/v1/analytics/_table/CUSTOMERS?fields=CUSTOMER_ID,NAME,EMAIL" \
  -H "Authorization: Bearer $TOKEN"
```

### Get record count

```bash
curl "http://localhost:8080/api/v1/analytics/_table/ORDERS?include_count=true" \
  -H "Authorization: Bearer $TOKEN"
```

## Step 5: Insert and Update Data

### Insert a record

```bash
curl -X POST http://localhost:8080/api/v1/analytics/_table/CUSTOMERS \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"NAME": "Acme Corp", "EMAIL": "contact@acme.com", "REGION": "US-WEST"}]}'
```

Note: Snowflake does not support RETURNING, so the response echoes back the submitted data. Use a subsequent GET to retrieve server-generated values.

### Update a record

```bash
curl -X PATCH "http://localhost:8080/api/v1/analytics/_table/CUSTOMERS?filter=NAME%20%3D%20'Acme%20Corp'" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resource": [{"REGION": "US-EAST"}]}'
```

## Key Pair (JWT) Authentication

Many Snowflake environments require key pair authentication instead of username/password. Faucet supports this natively.

### Generate an RSA Key Pair

```bash
# Generate a 2048-bit PKCS#8 private key (no passphrase)
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out snowflake_key.p8

# Extract the public key
openssl pkey -in snowflake_key.p8 -pubout -out snowflake_key.pub
```

### Register the Public Key in Snowflake

```sql
-- Get the public key content (without headers/footers)
-- Then assign it to your user:
ALTER USER ANALYST_USER SET RSA_PUBLIC_KEY='MIIBIjANBg...your_public_key_here...';

-- Verify it's set:
DESC USER ANALYST_USER;
```

### Connect Using Key Pair Auth

**Via CLI:**

```bash
faucet db add \
  --name analytics \
  --driver snowflake \
  --dsn "ANALYST_USER@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH&role=ANALYST" \
  --private-key-path /path/to/snowflake_key.p8
```

Note: No password is needed in the DSN when using key pair auth.

**Via API:**

```bash
curl -X POST http://localhost:8080/api/v1/system/service \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "analytics",
    "driver": "snowflake",
    "dsn": "ANALYST_USER@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH&role=ANALYST",
    "private_key_path": "/path/to/snowflake_key.p8"
  }'
```

**Via YAML config:**

```yaml
services:
  - name: analytics
    driver: snowflake
    dsn: "ANALYST_USER@myorg-myaccount/ANALYTICS/PUBLIC?warehouse=COMPUTE_WH"
    private_key_path: /path/to/snowflake_key.p8
```

### Supported Key Formats

| Format | PEM Header | Notes |
|--------|-----------|-------|
| PKCS#8 | `-----BEGIN PRIVATE KEY-----` | Recommended; output of `openssl genpkey` |
| PKCS#1 | `-----BEGIN RSA PRIVATE KEY-----` | Legacy; output of `openssl genrsa` |

Encrypted (passphrase-protected) keys are **not** supported — decrypt first with:

```bash
openssl pkey -in encrypted_key.p8 -out decrypted_key.p8
```

## Snowflake-Specific Notes

### Table and column names are case-sensitive

Snowflake defaults to uppercase identifiers. When querying via the API, use the exact case:

```bash
# Correct — uppercase table name
curl ".../analytics/_table/CUSTOMERS"

# Incorrect — will fail if created without quotes
curl ".../analytics/_table/customers"
```

### Warehouse must be running

Faucet queries require an active warehouse. If the warehouse is suspended, Snowflake will auto-resume it (which may add a few seconds of latency on the first query).

### Schema defaults to PUBLIC

If you don't specify a schema in the DSN, Faucet defaults to `PUBLIC`. Override with:

```bash
faucet db add \
  --name analytics \
  --driver snowflake \
  --dsn "user:pass@account/DB/MY_SCHEMA?warehouse=WH" \
  --schema MY_SCHEMA
```

### Connection pool tuning

Snowflake connections are heavier than local databases. Recommended pool settings:

```json
{
  "pool": {
    "max_open_conns": 10,
    "max_idle_conns": 2,
    "conn_max_lifetime": "10m",
    "conn_max_idle_time": "2m"
  }
}
```

### Stored procedures

Faucet can execute Snowflake stored procedures:

```bash
curl -X POST http://localhost:8080/api/v1/analytics/_proc/MY_PROCEDURE \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"params": {"input_date": "2024-01-01"}}'
```

### Supported features

| Feature | Supported |
|---------|-----------|
| Schema introspection | Yes |
| RETURNING clause | No |
| Upsert | No |
| Stored procedures | Yes |
| Views | Yes |
| Key pair (JWT) auth | Yes |
| Parameter style | `?` |
| Default schema | `PUBLIC` |

## Troubleshooting

### "390100: Missing account identifier"

Your DSN is missing the account identifier. Format: `user:pass@ACCOUNT/DB/SCHEMA?warehouse=WH`

### "390144: Role does not exist or not authorized"

The role specified in the DSN doesn't exist or your user doesn't have access to it. Check `SHOW GRANTS TO USER your_user` in Snowflake.

### "No active warehouse"

Add `?warehouse=COMPUTE_WH` to your DSN, or ensure the user has a default warehouse set.

### Slow first query

Snowflake auto-suspends idle warehouses. The first query may take 5-30 seconds while the warehouse resumes. Set `AUTO_SUSPEND` to a higher value in Snowflake to reduce this.

## What's Next

- [Filter Syntax](filter-syntax.md) -- query filtering operators
- [RBAC](rbac.md) -- restrict access per role
- [API Reference](api-reference.md) -- full REST API documentation
- [Database Connectors](database-connectors.md) -- all connector DSN formats
