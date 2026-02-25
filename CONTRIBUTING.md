# Contributing to Faucet

Thanks for your interest in contributing! Here's what you need to know.

## Getting Started

```bash
git clone https://github.com/faucetdb/faucet.git
cd faucet
cd ui && npm install && cd ..
make build
make test
```

**Requirements:** Go 1.25+, Node.js 18+ (for UI), golangci-lint

## Development Workflow

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Run `make test` and `make lint` before committing
4. Open a PR targeting the `dev` branch

### Useful Commands

| Command | What it does |
|---------|--------------|
| `make dev` | Run the server with hot reload |
| `make dev-ui` | Run the UI dev server (Vite) |
| `make test` | Run all tests with race detection |
| `make test-v` | Verbose test output |
| `make test-cover` | Generate coverage report |
| `make lint` | Run golangci-lint |
| `make bench` | Run benchmarks |
| `make build` | Build UI + Go binary |

## Project Structure

```
cmd/faucet/       → CLI entrypoint (Cobra)
internal/
  api/            → HTTP handlers and middleware (Chi router)
  config/         → SQLite config store
  connector/      → Database drivers (PostgreSQL, MySQL, SQL Server, Snowflake, SQLite)
  mcp/            → MCP server implementation
  ui/             → Embedded admin UI assets
ui/               → Preact + Vite + Tailwind source
```

## What We're Looking For

- **Bug fixes** — Always welcome. Include a test if possible.
- **New database connectors** — Implement the `connector.Connector` interface.
- **Documentation** — Improvements to the [wiki](https://github.com/faucetdb/wiki) are appreciated.
- **Performance improvements** — Include benchmark results (`make bench`).

## Code Style

- Follow existing patterns in the codebase
- `golangci-lint` must pass (`make lint`)
- Tests use the standard `testing` package — no test frameworks
- Keep PRs focused. One feature or fix per PR.

## Reporting Bugs

Open an [issue](https://github.com/faucetdb/faucet/issues) with:
- Faucet version (`faucet version`)
- Database type and version
- Steps to reproduce
- Expected vs. actual behavior

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
