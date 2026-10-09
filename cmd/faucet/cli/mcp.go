package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	fmcp "github.com/faucetdb/faucet/internal/mcp"
	"github.com/faucetdb/faucet/internal/server/middleware"
	"github.com/faucetdb/faucet/internal/service"
)

func newMCPCmd() *cobra.Command {
	var (
		transport string
		port      int
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP server for AI agents",
		Long: `Start a Model Context Protocol (MCP) server that exposes database operations
as tools for AI agents like Claude. Supports stdio (default) and HTTP transports.

In stdio mode, the MCP server communicates over stdin/stdout using JSON-RPC,
suitable for direct integration with Claude Desktop or other MCP clients.
The local process is trusted and runs with admin privileges.

In HTTP mode, the server listens on the specified port for Streamable HTTP
connections. Clients must authenticate with an X-API-Key header or an admin
Bearer token; API-key clients are subject to their role's access rules.`,
		Example: `  faucet mcp                            # stdio mode (for Claude Desktop)
  faucet mcp --transport http --port 3001  # authenticated Streamable HTTP mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCP(transport, port)
		},
	}

	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport mode: stdio or http")
	cmd.Flags().IntVar(&port, "port", 3001, "HTTP port (only used with --transport http)")

	return cmd
}

func runMCP(transport string, port int) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Initialize config store
	store, err := config.NewStore(resolveDataDir())
	if err != nil {
		return fmt.Errorf("init config store: %w", err)
	}
	defer store.Close()

	// Initialize connector registry
	registry := newRegistry()

	// Connect all active services
	services, err := store.ListServices(context.Background())
	if err != nil {
		logger.Warn("failed to load services", "error", err)
	}
	for _, svc := range services {
		if !svc.IsActive {
			registry.Pause(svc.Name)
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
		// Resolve the same secret the main server uses so that admin
		// tokens issued by "faucet serve" verify here when both share a
		// data directory.
		jwtSecret, err := resolveJWTSecret(context.Background(), store, logger)
		if err != nil {
			return fmt.Errorf("resolve jwt secret: %w", err)
		}
		authSvc := service.NewAuthService(store, jwtSecret)

		// The MCP transport itself does not authenticate; wrap it in the
		// same Authenticate middleware the main server uses so that every
		// tool call carries a principal for RBAC enforcement.
		httpServer := &http.Server{
			Addr:              addr,
			Handler:           middleware.Authenticate(authSvc)(mcpSrv.HTTPHandler()),
			ReadHeaderTimeout: 15 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		logger.Info("starting authenticated MCP HTTP server", "addr", addr)
		return httpServer.ListenAndServe()
	default:
		return fmt.Errorf("unsupported transport %q; use 'stdio' or 'http'", transport)
	}
}
