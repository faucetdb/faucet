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

## Releasing

Maintainers cut a release by pushing a tag: `git tag v0.1.14 && git push origin v0.1.14`.

1. `.github/workflows/release.yml` (GoReleaser) builds the archives and publishes the GitHub release, Docker images and the Homebrew formula.
2. `.github/workflows/npm-publish.yml` runs automatically once "Release" succeeds (`workflow_run`) and publishes `@faucetdb/faucet` plus the six platform packages from the release archives via `scripts/npm-publish.sh`. It can also be started by hand from the Actions tab ("Publish npm" -> Run workflow -> tag) to retry a partial publish; versions already on npm are skipped.

npm authentication: the workflow uses [trusted publishing (OIDC)](https://docs.npmjs.com/trusted-publishers) when the `NPM_TOKEN` secret is absent. Each of the 7 packages on npmjs.com must list the trusted publisher `faucetdb/faucet`, workflow file `npm-publish.yml`. If `NPM_TOKEN` is set it is used instead; note that npm granular tokens expire after at most 90 days, and an expired token shows up as `404 Not Found - PUT https://registry.npmjs.org/@faucetdb%2f...`.

To exercise the pipeline locally without publishing: `DRY_RUN=1 ./scripts/npm-publish.sh v0.1.13` (needs `gh` and `npm`).

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
