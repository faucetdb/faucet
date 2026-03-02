# @faucetdb/faucet

Turn any SQL database into a secure REST API + MCP server. One binary. One command.

## Quick Start

```bash
npx @faucetdb/faucet serve
```

Or install globally:

```bash
npm install -g @faucetdb/faucet
faucet serve
```

## What is Faucet?

Faucet connects to your SQL databases, introspects their schemas, and auto-generates a full CRUD REST API with authentication, RBAC, OpenAPI docs, and an MCP server for AI agents — all in a single binary.

**Supported databases:** PostgreSQL, MySQL, MariaDB, SQL Server, Oracle, Snowflake, SQLite.

## Usage

```bash
# Start the server
npx @faucetdb/faucet serve

# Connect a database
npx @faucetdb/faucet db add --name mydb --driver postgres \
  --dsn "postgres://user:pass@localhost/mydb?sslmode=disable"

# Create an API key
npx @faucetdb/faucet key create --role default

# Query your data
curl -H "X-API-Key: faucet_YOUR_KEY" http://localhost:8080/api/v1/mydb/_table/users
```

## Links

- [Documentation](https://wiki.faucetdb.ai)
- [GitHub](https://github.com/faucetdb/faucet)
- [Website](https://faucetdb.ai)

## License

MIT
