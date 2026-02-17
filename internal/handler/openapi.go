package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
)

// OpenAPIHandler generates and serves OpenAPI 3.1 specifications dynamically
// based on introspected database schemas. Each service produces a complete
// OpenAPI spec with paths for all tables, views, and stored procedures.
type OpenAPIHandler struct {
	registry *connector.Registry
	store    *config.Store
}

// NewOpenAPIHandler creates a new OpenAPIHandler.
func NewOpenAPIHandler(registry *connector.Registry, store *config.Store) *OpenAPIHandler {
	return &OpenAPIHandler{
		registry: registry,
		store:    store,
	}
}

// ServeCombinedSpec returns a combined OpenAPI spec covering all registered
// services. Each service's tables are namespaced under /api/v2/{serviceName}.
// GET /api/v2/openapi.json
func (h *OpenAPIHandler) ServeCombinedSpec(w http.ResponseWriter, r *http.Request) {
	services, err := h.store.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list services: "+err.Error())
		return
	}

	paths := make(map[string]interface{})
	schemas := make(map[string]interface{})
	tags := make([]map[string]interface{}, 0)

	for _, svc := range services {
		if !svc.IsActive {
			continue
		}

		conn, err := h.registry.Get(svc.Name)
		if err != nil {
			continue // skip services that aren't connected
		}

		schema, err := conn.IntrospectSchema(r.Context())
		if err != nil {
			continue // skip services that fail introspection
		}

		tags = append(tags, map[string]interface{}{
			"name":        svc.Name,
			"description": fmt.Sprintf("Database service: %s (%s)", svc.Label, svc.Driver),
		})

		servicePaths, serviceSchemas := buildServiceSpec(svc.Name, schema)
		for k, v := range servicePaths {
			paths[k] = v
		}
		for k, v := range serviceSchemas {
			schemas[k] = v
		}
	}

	spec := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":       "Faucet API",
			"description": "Auto-generated REST API for all database services",
			"version":     "1.0.0",
		},
		"tags":  tags,
		"paths": paths,
		"components": map[string]interface{}{
			"schemas": schemas,
			"securitySchemes": map[string]interface{}{
				"apiKey": map[string]interface{}{
					"type": "apiKey",
					"in":   "header",
					"name": "X-DreamFactory-API-Key",
				},
				"bearer": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		},
		"security": []map[string]interface{}{
			{"apiKey": []string{}},
			{"bearer": []string{}},
		},
	}

	writeJSON(w, http.StatusOK, spec)
}

// ServeServiceSpec returns the OpenAPI spec for a single service.
// GET /api/v2/{serviceName}/openapi.json
func (h *OpenAPIHandler) ServeServiceSpec(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	svc, _ := h.store.GetServiceByName(r.Context(), serviceName)
	label := serviceName
	driver := ""
	if svc != nil {
		label = svc.Label
		driver = svc.Driver
	}

	schema, err := conn.IntrospectSchema(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to introspect schema: "+err.Error())
		return
	}

	paths, schemas := buildServiceSpec(serviceName, schema)

	spec := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":       label + " API",
			"description": fmt.Sprintf("Auto-generated REST API for %s (%s)", label, driver),
			"version":     "1.0.0",
		},
		"paths": paths,
		"components": map[string]interface{}{
			"schemas": schemas,
			"securitySchemes": map[string]interface{}{
				"apiKey": map[string]interface{}{
					"type": "apiKey",
					"in":   "header",
					"name": "X-DreamFactory-API-Key",
				},
				"bearer": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		},
		"security": []map[string]interface{}{
			{"apiKey": []string{}},
			{"bearer": []string{}},
		},
	}

	writeJSON(w, http.StatusOK, spec)
}

// ---------------------------------------------------------------------------
// OpenAPI spec generation helpers
// ---------------------------------------------------------------------------

// buildServiceSpec generates paths and component schemas for a single service
// based on its introspected schema.
func buildServiceSpec(serviceName string, schema *model.Schema) (map[string]interface{}, map[string]interface{}) {
	paths := make(map[string]interface{})
	schemas := make(map[string]interface{})

	basePath := fmt.Sprintf("/api/v2/%s", serviceName)

	// Generate paths for each table.
	for _, table := range schema.Tables {
		tableListPath := fmt.Sprintf("%s/_table/%s", basePath, table.Name)
		schemaRef := fmt.Sprintf("%s_%s", serviceName, table.Name)

		// Build the JSON Schema for this table's records.
		schemas[schemaRef] = buildTableSchema(table)
		schemas[schemaRef+"_list"] = buildListSchema(schemaRef)

		paths[tableListPath] = buildTablePaths(table, schemaRef, serviceName)
	}

	// Generate paths for views (read-only).
	for _, view := range schema.Views {
		viewPath := fmt.Sprintf("%s/_table/%s", basePath, view.Name)
		schemaRef := fmt.Sprintf("%s_%s", serviceName, view.Name)

		schemas[schemaRef] = buildTableSchema(view)
		schemas[schemaRef+"_list"] = buildListSchema(schemaRef)

		paths[viewPath] = map[string]interface{}{
			"get": map[string]interface{}{
				"summary":     fmt.Sprintf("Query %s records", view.Name),
				"operationId": fmt.Sprintf("get_%s_%s", serviceName, view.Name),
				"tags":        []string{serviceName},
				"parameters":  buildQueryParameters(),
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Success",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/" + schemaRef + "_list",
								},
							},
						},
					},
				},
			},
		}
	}

	// Generate paths for stored procedures.
	for _, proc := range schema.Procedures {
		procPath := fmt.Sprintf("%s/_proc/%s", basePath, proc.Name)
		paths[procPath] = buildProcPath(proc, serviceName)
	}
	for _, fn := range schema.Functions {
		fnPath := fmt.Sprintf("%s/_proc/%s", basePath, fn.Name)
		paths[fnPath] = buildProcPath(fn, serviceName)
	}

	return paths, schemas
}

// buildTableSchema generates a JSON Schema object for a table's record type.
func buildTableSchema(table model.TableSchema) map[string]interface{} {
	properties := make(map[string]interface{})
	required := make([]string, 0)

	for _, col := range table.Columns {
		prop := map[string]interface{}{
			"type": jsonSchemaType(col.JsonType),
		}
		if col.Comment != "" {
			prop["description"] = col.Comment
		}
		if col.MaxLength != nil {
			prop["maxLength"] = *col.MaxLength
		}
		if col.IsAutoIncrement {
			prop["readOnly"] = true
		}
		properties[col.Name] = prop

		// Non-nullable, non-auto-increment columns are required.
		if !col.Nullable && !col.IsAutoIncrement && col.Default == nil {
			required = append(required, col.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// buildListSchema generates the list response envelope schema.
func buildListSchema(recordSchemaRef string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"resource": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"$ref": "#/components/schemas/" + recordSchemaRef,
				},
			},
			"meta": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"count":  map[string]interface{}{"type": "integer"},
					"total":  map[string]interface{}{"type": "integer"},
					"limit":  map[string]interface{}{"type": "integer"},
					"offset": map[string]interface{}{"type": "integer"},
					"took_ms": map[string]interface{}{
						"type":   "number",
						"format": "double",
					},
				},
			},
		},
	}
}

// buildTablePaths generates the full CRUD path item for a table.
func buildTablePaths(table model.TableSchema, schemaRef, serviceName string) map[string]interface{} {
	ref := "#/components/schemas/" + schemaRef
	listRef := "#/components/schemas/" + schemaRef + "_list"

	return map[string]interface{}{
		"get": map[string]interface{}{
			"summary":     fmt.Sprintf("Query %s records", table.Name),
			"operationId": fmt.Sprintf("get_%s_%s", serviceName, table.Name),
			"tags":        []string{serviceName},
			"parameters":  buildQueryParameters(),
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Success",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{"$ref": listRef},
						},
					},
				},
			},
		},
		"post": map[string]interface{}{
			"summary":     fmt.Sprintf("Create %s records", table.Name),
			"operationId": fmt.Sprintf("create_%s_%s", serviceName, table.Name),
			"tags":        []string{serviceName},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"$ref": ref},
					},
				},
			},
			"responses": map[string]interface{}{
				"201": map[string]interface{}{
					"description": "Created",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{"$ref": listRef},
						},
					},
				},
			},
		},
		"put": map[string]interface{}{
			"summary":     fmt.Sprintf("Replace %s records", table.Name),
			"operationId": fmt.Sprintf("replace_%s_%s", serviceName, table.Name),
			"tags":        []string{serviceName},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"$ref": ref},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Updated",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{"$ref": listRef},
						},
					},
				},
			},
		},
		"patch": map[string]interface{}{
			"summary":     fmt.Sprintf("Update %s records", table.Name),
			"operationId": fmt.Sprintf("update_%s_%s", serviceName, table.Name),
			"tags":        []string{serviceName},
			"parameters": []map[string]interface{}{
				{
					"name":        "filter",
					"in":          "query",
					"description": "Filter expression",
					"schema":      map[string]interface{}{"type": "string"},
				},
				{
					"name":        "ids",
					"in":          "query",
					"description": "Comma-separated list of record IDs",
					"schema":      map[string]interface{}{"type": "string"},
				},
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"$ref": ref},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Updated",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{"$ref": listRef},
						},
					},
				},
			},
		},
		"delete": map[string]interface{}{
			"summary":     fmt.Sprintf("Delete %s records", table.Name),
			"operationId": fmt.Sprintf("delete_%s_%s", serviceName, table.Name),
			"tags":        []string{serviceName},
			"parameters": []map[string]interface{}{
				{
					"name":        "filter",
					"in":          "query",
					"description": "Filter expression",
					"schema":      map[string]interface{}{"type": "string"},
				},
				{
					"name":        "ids",
					"in":          "query",
					"description": "Comma-separated list of record IDs",
					"schema":      map[string]interface{}{"type": "string"},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Deleted",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"meta": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"count": map[string]interface{}{"type": "integer"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// buildQueryParameters returns common query parameters for table GET endpoints.
func buildQueryParameters() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "fields",
			"in":          "query",
			"description": "Comma-separated list of fields to return",
			"schema":      map[string]interface{}{"type": "string"},
		},
		{
			"name":        "filter",
			"in":          "query",
			"description": "DreamFactory-compatible filter expression",
			"schema":      map[string]interface{}{"type": "string"},
		},
		{
			"name":        "order",
			"in":          "query",
			"description": "Order by expression (e.g. 'created_at DESC')",
			"schema":      map[string]interface{}{"type": "string"},
		},
		{
			"name":        "limit",
			"in":          "query",
			"description": "Maximum records to return (default 25, max 1000)",
			"schema": map[string]interface{}{
				"type":    "integer",
				"default": 25,
				"maximum": 1000,
			},
		},
		{
			"name":        "offset",
			"in":          "query",
			"description": "Number of records to skip",
			"schema": map[string]interface{}{
				"type":    "integer",
				"default": 0,
			},
		},
		{
			"name":        "include_count",
			"in":          "query",
			"description": "Include total record count in response meta",
			"schema":      map[string]interface{}{"type": "boolean"},
		},
	}
}

// buildProcPath generates a path item for a stored procedure or function.
func buildProcPath(proc model.StoredProcedure, serviceName string) map[string]interface{} {
	return map[string]interface{}{
		"post": map[string]interface{}{
			"summary":     fmt.Sprintf("Call %s %s", proc.Type, proc.Name),
			"operationId": fmt.Sprintf("call_%s_%s", serviceName, proc.Name),
			"tags":        []string{serviceName},
			"requestBody": map[string]interface{}{
				"description": "Procedure parameters",
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"type":                 "object",
							"additionalProperties": true,
						},
					},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Procedure result",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"resource": map[string]interface{}{
										"type":  "array",
										"items": map[string]interface{}{"type": "object"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// jsonSchemaType maps a Faucet JSON type string (from model.Column.JsonType)
// to a valid JSON Schema type.
func jsonSchemaType(jsonType string) string {
	// Strip format suffixes like "string(date-time)" -> "string"
	base := jsonType
	if idx := strings.Index(jsonType, "("); idx >= 0 {
		base = jsonType[:idx]
	}

	switch base {
	case "integer":
		return "integer"
	case "number":
		return "number"
	case "boolean":
		return "boolean"
	case "object":
		return "object"
	case "array":
		return "array"
	case "string":
		return "string"
	default:
		return "string"
	}
}
