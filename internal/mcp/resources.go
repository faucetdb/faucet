package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerResources adds MCP resource definitions to the server. Resources
// provide read-only data that LLM clients can load into their context.
func (s *MCPServer) registerResources(srv *server.MCPServer) {

	// -------------------------------------------------------------------
	// faucet://services — list of all connected database services
	// -------------------------------------------------------------------
	srv.AddResource(
		mcp.NewResource(
			"faucet://services",
			"Connected Database Services",
			mcp.WithResourceDescription(
				"List of all database services configured in Faucet, "+
					"including their driver type and active status.",
			),
			mcp.WithMIMEType("application/json"),
		),
		s.handleServicesResource,
	)

	// -------------------------------------------------------------------
	// faucet://schema/{service} — full schema for a service (template)
	// -------------------------------------------------------------------
	srv.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"faucet://schema/{service}",
			"Database Schema",
			mcp.WithTemplateDescription(
				"Full schema introspection for a database service, "+
					"including tables, columns, primary keys, foreign keys, and indexes.",
			),
			mcp.WithTemplateMIMEType("application/json"),
		),
		s.handleSchemaResource,
	)
}

// handleServicesResource returns a JSON list of all configured services.
func (s *MCPServer) handleServicesResource(
	ctx context.Context,
	request mcp.ReadResourceRequest,
) ([]mcp.ResourceContents, error) {

	services, err := s.store.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	type serviceInfo struct {
		Name     string `json:"name"`
		Label    string `json:"label,omitempty"`
		Driver   string `json:"driver"`
		IsActive bool   `json:"is_active"`
		ReadOnly bool   `json:"read_only"`
		RawSQL   bool   `json:"raw_sql_allowed"`
	}

	items := make([]serviceInfo, len(services))
	for i, svc := range services {
		items[i] = serviceInfo{
			Name:     svc.Name,
			Label:    svc.Label,
			Driver:   svc.Driver,
			IsActive: svc.IsActive,
			ReadOnly: svc.ReadOnly,
			RawSQL:   svc.RawSQL,
		}
	}

	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal services: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "faucet://services",
			MIMEType: "application/json",
			Text:     string(b),
		},
	}, nil
}

// handleSchemaResource returns the full introspected schema for a service.
func (s *MCPServer) handleSchemaResource(
	ctx context.Context,
	request mcp.ReadResourceRequest,
) ([]mcp.ResourceContents, error) {

	// Extract service name from URI: "faucet://schema/{service}"
	uri := request.Params.URI
	serviceName := strings.TrimPrefix(uri, "faucet://schema/")
	if serviceName == "" || serviceName == uri {
		return nil, fmt.Errorf("invalid schema URI %q: expected faucet://schema/{service}", uri)
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return nil, fmt.Errorf("service %q not found: %w (available: %v)",
			serviceName, err, s.registry.ListServices())
	}

	schema, err := conn.IntrospectSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect schema for %q: %w", serviceName, err)
	}

	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(b),
		},
	}, nil
}
