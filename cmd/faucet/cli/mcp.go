package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/mssql"
	"github.com/faucetdb/faucet/internal/connector/mysql"
	"github.com/faucetdb/faucet/internal/connector/postgres"
	"github.com/faucetdb/faucet/internal/connector/snowflake"
	fmcp "github.com/faucetdb/faucet/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	var (
		transport string
		port      int
		dataDir   string
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP server for AI agents",
		Long: `Start a Model Context Protocol (MCP) server that exposes database operations
as tools for AI agents like Claude. Supports stdio (default) and HTTP transports.

In stdio mode, the MCP server communicates over stdin/stdout using JSON-RPC,
suitable for direct integration with Claude Desktop or other MCP clients.

In HTTP mode, the server listens on the specified port for SSE connections.`,
		Example: `  faucet mcp                            # stdio mode (for Claude Desktop)
  faucet mcp --transport http --port 3001  # HTTP SSE mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCP(transport, port, dataDir)
		},
	}

	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport mode: stdio or http")
	cmd.Flags().IntVar(&port, "port", 3001, "HTTP port (only used with --transport http)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for SQLite config (default: ~/.faucet)")

	return cmd
}

func runMCP(transport string, port int, dataDir string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Initialize config store
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = home + "/.faucet"
	}
	store, err := config.NewStore(dataDir)
	if err != nil {
		return fmt.Errorf("init config store: %w", err)
	}
	defer store.Close()

	// Initialize connector registry
	registry := connector.NewRegistry()
	registry.RegisterDriver("postgres", func() connector.Connector { return postgres.New() })
	registry.RegisterDriver("mysql", func() connector.Connector { return mysql.New() })
	registry.RegisterDriver("mssql", func() connector.Connector { return mssql.New() })
	registry.RegisterDriver("snowflake", func() connector.Connector { return snowflake.New() })

	// Connect all active services
	services, err := store.ListServices(context.Background())
	if err != nil {
		logger.Warn("failed to load services", "error", err)
	}
	for _, svc := range services {
		if !svc.IsActive {
			continue
		}
		cfg := connector.ConnectionConfig{
			Driver:          svc.Driver,
			DSN:             svc.DSN,
			PrivateKeyPath:  svc.PrivateKeyPath,
			SchemaName:      svc.Schema,
			MaxOpenConns:    svc.Pool.MaxOpenConns,
			MaxIdleConns:    svc.Pool.MaxIdleConns,
			ConnMaxLifetime: svc.Pool.ConnMaxLifetime,
			ConnMaxIdleTime: svc.Pool.ConnMaxIdleTime,
		}
		if err := registry.Connect(svc.Name, cfg); err != nil {
			logger.Error("failed to connect service", "service", svc.Name, "error", err)
		} else {
			logger.Info("connected service", "service", svc.Name, "driver", svc.Driver)
		}
	}
	defer registry.CloseAll()

	// Create MCP server
	mcpSrv := fmcp.NewMCPServer(registry, store, logger)

	switch transport {
	case "stdio":
		return mcpSrv.ServeStdio()
	case "http":
		addr := fmt.Sprintf(":%d", port)
		jwtSecret := viper.GetString("auth.jwt_secret")
		if jwtSecret == "" {
			jwtSecret = "faucet-dev-secret-change-me"
		}
		logger.Info("starting MCP HTTP server", "addr", addr)
		return mcpSrv.ServeHTTP(addr)
	default:
		return fmt.Errorf("unsupported transport %q; use 'stdio' or 'http'", transport)
	}
}
