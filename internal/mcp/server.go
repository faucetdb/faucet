package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/rbac"
)

// MCPServer wraps the mcp-go server with Faucet-specific tool and resource
// registrations. It exposes database services as MCP tools so AI agents can
// discover schemas, query data, and perform CRUD operations.
//
// Every tool call and resource read is authorized against the same
// role-based access rules that govern the REST API: the principal attached
// to the request context (see rbac.PrincipalFromContext) must hold a role
// whose access rules grant the HTTP verb equivalent of the operation on the
// service/component being touched. Admin principals bypass the check; a
// missing principal is refused.
type MCPServer struct {
	registry *connector.Registry
	store    *config.Store
	logger   *slog.Logger
	server   *server.MCPServer
	enforcer *rbac.Enforcer
}

// Version is reported to clients in the MCP handshake. The CLI sets it to the
// build version before creating servers.
var Version = "dev"

// NewMCPServer creates an MCPServer pre-loaded with all Faucet tools and
// resources. The returned server is ready to serve over stdio or HTTP.
func NewMCPServer(registry *connector.Registry, store *config.Store, logger *slog.Logger) *MCPServer {
	s := &MCPServer{
		registry: registry,
		store:    store,
		logger:   logger,
		enforcer: rbac.NewEnforcer(store),
	}

	mcpServer := server.NewMCPServer(
		"Faucet Database API",
		Version,
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
//
// The local process is trusted: whoever can launch it already has direct
// access to the Faucet config database, so every request is executed with
// an admin principal and RBAC rules are bypassed.
func (s *MCPServer) ServeStdio() error {
	s.logger.Info("starting MCP server in stdio mode with local admin privileges (RBAC bypassed)")
	// server.ServeStdio installs SIGINT/SIGTERM handling so deferred cleanup
	// in the caller runs; newStdioServer is kept for tests.
	return server.ServeStdio(s.server, server.WithStdioContextFunc(stdioAdminContext))
}

// newStdioServer builds the stdio transport for this server. The returned
// transport injects an admin principal into every request context, so all
// tool calls and resource reads made over it bypass RBAC (see ServeStdio).
// It is separated from ServeStdio so tests can drive Listen over in-memory
// pipes.
func (s *MCPServer) newStdioServer() *server.StdioServer {
	stdio := server.NewStdioServer(s.server)
	stdio.SetContextFunc(stdioAdminContext)
	return stdio
}

// stdioAdminContext attaches an admin principal to ctx. It is the context
// function installed on the stdio transport.
func stdioAdminContext(ctx context.Context) context.Context {
	return rbac.WithPrincipal(ctx, &rbac.Principal{Type: rbac.PrincipalAdmin, IsAdmin: true})
}

// HTTPHandler returns an http.Handler implementing the Streamable HTTP MCP
// transport. This is suitable for mounting on an existing HTTP server/router
// so the MCP endpoint runs alongside the REST API on the same port.
//
// The handler performs no authentication itself: it must be mounted behind
// middleware.Authenticate so that a principal is present on the request
// context. Requests without a principal are refused by every tool.
func (s *MCPServer) HTTPHandler() http.Handler {
	return server.NewStreamableHTTPServer(s.server,
		server.WithHeartbeatInterval(30*time.Second),
	)
}

// ---------------------------------------------------------------------------
// Authorization
// ---------------------------------------------------------------------------

// authorize checks that the principal carried by ctx may perform verb on
// service/component. It returns nil when the call is allowed; otherwise it
// returns a tool error result whose text starts with "Forbidden: " (for an
// RBAC denial) or "Authorization check failed: " (for a loader failure).
// A missing principal is refused (fail closed). Handlers must call it after
// parsing their required arguments and before touching the registry or any
// database.
func (s *MCPServer) authorize(ctx context.Context, service, component string, verb int) *mcp.CallToolResult {
	principal := rbac.PrincipalFromContext(ctx)
	err := s.enforcer.Authorize(ctx, principal, service, component, verb)
	if err == nil {
		return nil
	}
	return authorizationError(err)
}

// authorizationError converts an error returned by the rbac package into a
// tool error result.
func authorizationError(err error) *mcp.CallToolResult {
	var denial *rbac.Denial
	if errors.As(err, &denial) {
		return mcp.NewToolResultError("Forbidden: " + denial.Reason)
	}
	return mcp.NewToolResultError(fmt.Sprintf("Authorization check failed: %v", err))
}

// visibleServices returns the configured services the principal carried by
// ctx may see. Admin principals see every service; API-key principals see
// only the services their role grants at least one verb on. It returns a
// *rbac.Denial when there is no principal or the role cannot be used, and
// a plain error when the role or service list cannot be loaded.
func (s *MCPServer) visibleServices(ctx context.Context) ([]model.ServiceConfig, error) {
	principal := rbac.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, &rbac.Denial{Reason: "Authentication required"}
	}

	var role *model.Role
	if !principal.IsAdmin {
		// Resolve the role before touching the service list so that a bad
		// principal never reaches the config store.
		var err error
		role, err = s.enforcer.Role(ctx, principal)
		if err != nil {
			return nil, err
		}
	}

	services, err := s.store.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return services, nil
	}

	visible := make([]model.ServiceConfig, 0, len(services))
	for _, svc := range services {
		if rbac.CanAccessService(role.Access, svc.Name) {
			visible = append(visible, svc)
		}
	}
	return visible, nil
}

// serviceHint returns a suffix such as " Available services: [a b]" naming
// the services the principal carried by ctx may see, for appending to
// error messages so an LLM client can self-correct. It never names a
// service the caller's role cannot access: when the visible list is empty
// or cannot be determined it returns "" so no hint is given.
func (s *MCPServer) serviceHint(ctx context.Context) string {
	services, err := s.visibleServices(ctx)
	if err != nil || len(services) == 0 {
		return ""
	}
	names := make([]string, 0, len(services))
	for _, svc := range services {
		if svc.IsActive {
			names = append(names, svc.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return " Available services: [" + strings.Join(names, " ") + "]"
}

// serviceUnavailable explains why the registry has no connection for
// serviceName. The registry error is not echoed: it lists every connected
// service and can carry driver errors naming internal hosts.
func (s *MCPServer) serviceUnavailable(ctx context.Context, serviceName string, err error) string {
	switch {
	case errors.Is(err, connector.ErrServicePaused):
		return fmt.Sprintf("Service %q is paused, so its tools are unavailable until an admin resumes it.%s", serviceName, s.serviceHint(ctx))
	case errors.Is(err, connector.ErrServiceNotConnected):
		return fmt.Sprintf("Service %q is not connected to its database right now.%s", serviceName, s.serviceHint(ctx))
	default:
		return fmt.Sprintf("Service %q not found.%s", serviceName, s.serviceHint(ctx))
	}
}

// writableService loads the service configuration for serviceName and
// refuses the operation when the service is flagged read_only. refusal is
// the sentence appended to the read-only error, e.g. "Insert operations are
// not permitted.". It fails closed: a config store error other than
// "not found" is reported as an authorization failure rather than treated
// as writable. The second return value is nil when the operation may
// proceed; otherwise it is the tool error result to return to the caller.
func (s *MCPServer) writableService(ctx context.Context, serviceName, refusal string) (*model.ServiceConfig, *mcp.CallToolResult) {
	svc, err := s.store.GetServiceByName(ctx, serviceName)
	switch {
	case errors.Is(err, config.ErrNotFound):
		return nil, mcp.NewToolResultError(fmt.Sprintf("Service %q not found.%s", serviceName, s.serviceHint(ctx)))
	case err != nil:
		return nil, mcp.NewToolResultError(fmt.Sprintf("Authorization check failed: %v", err))
	case svc.ReadOnly:
		return nil, mcp.NewToolResultError(fmt.Sprintf("Service %q is read-only. %s", serviceName, refusal))
	}
	return svc, nil
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
