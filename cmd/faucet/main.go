// Command faucet turns a SQL database (PostgreSQL, MySQL, MariaDB, SQL Server,
// Oracle, Snowflake, SQLite) into a REST API and an MCP server for AI agents,
// with role-based access control, an OpenAPI 3.1 spec and an embedded admin UI.
//
// Run "faucet serve" and open http://localhost:8080. The MCP endpoint is
// served at /mcp on the same port and authenticates with an API key in the
// X-API-Key header.
//
// Install:
//
//	go install github.com/faucetdb/faucet/cmd/faucet@latest
//
// Docs: https://wiki.faucetdb.ai
package main

import (
	"fmt"
	"os"

	"github.com/faucetdb/faucet/cmd/faucet/cli"
)

// Set via -ldflags at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := cli.Execute(version, commit, date); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
