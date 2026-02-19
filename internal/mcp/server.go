package mcp

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
)

// MCPServer wraps the mcp-go server with Faucet-specific tool and resource
// registrations. It exposes database services as MCP tools so AI agents can
// discover schemas, query data, and perform CRUD operations.
type MCPServer struct {
	registry *connector.Registry
	store    *config.Store
	logger   *slog.Logger
	server   *server.MCPServer
}

// NewMCPServer creates an MCPServer pre-loaded with all Faucet tools and
// resources. The returned server is ready to serve over stdio or HTTP.
func NewMCPServer(registry *connector.Registry, store *config.Store, logger *slog.Logger) *MCPServer {
	s := &MCPServer{
		registry: registry,
		store:    store,
		logger:   logger,
	}

	mcpServer := server.NewMCPServer(
		"Faucet Database API",
		"0.1.0",
		server.WithResourceCapabilities(true, false),
		server.WithToolCapabilities(true),
	)

	// Register tools (query, insert, update, delete, etc.)
	s.registerTools(mcpServer)

	// Register resources (service list, schema templates)
	s.registerResources(mcpServer)

	s.server = mcpServer
	return s
}

// Server returns the underlying mcp-go MCPServer instance. Useful for
// advanced configuration or testing.
func (s *MCPServer) Server() *server.MCPServer {
	return s.server
}

// ServeStdio starts the MCP server in stdio mode. This is the primary
// integration path for Claude Code, Claude Desktop, and other MCP clients
// that launch the server as a subprocess.
func (s *MCPServer) ServeStdio() error {
	s.logger.Info("starting MCP server in stdio mode")
	return server.ServeStdio(s.server)
}

// ServeHTTP starts the MCP server in Streamable HTTP mode, listening on
// the given address (e.g. ":3001"). This is suitable for remote MCP clients.
func (s *MCPServer) ServeHTTP(addr string) error {
	httpServer := server.NewStreamableHTTPServer(s.server,
		server.WithHeartbeatInterval(30*time.Second),
	)
	s.logger.Info("MCP HTTP server starting", "addr", addr)
	return httpServer.Start(addr)
}

// HTTPHandler returns an http.Handler implementing the Streamable HTTP MCP
// transport. This is suitable for mounting on an existing HTTP server/router
// so the MCP endpoint runs alongside the REST API on the same port.
func (s *MCPServer) HTTPHandler() http.Handler {
	return server.NewStreamableHTTPServer(s.server,
		server.WithHeartbeatInterval(30*time.Second),
	)
}

// toolAnnotation returns a standard ToolAnnotation for read-only vs
// mutating tools.
func readOnlyAnnotation() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint: boolPtr(true),
	}
}

func mutatingAnnotation() mcp.ToolAnnotation {
	return mcp.ToolAnnotation{
		ReadOnlyHint: boolPtr(false),
	}
}

func boolPtr(b bool) *bool {
	return &b
}
