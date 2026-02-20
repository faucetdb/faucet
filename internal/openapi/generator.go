package openapi

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/faucetdb/faucet/internal/model"
)

// ServiceSpec holds the inputs needed to generate an OpenAPI spec for one database service.
type ServiceSpec struct {
	Name   string
	Label  string
	Driver string
	Schema *model.Schema
}

// GenerateServiceSpec generates an OpenAPI 3.1 spec for a single database service.
func GenerateServiceSpec(serviceName, serviceLabel, driver, baseURL string, schema *model.Schema) *openapi3.T {
	doc := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:       fmt.Sprintf("%s API", serviceLabel),
			Description: fmt.Sprintf("Auto-generated REST API for %s (%s database) by Faucet.", serviceLabel, driver),
			Version:     "1.0.0",
		},
		Servers: openapi3.Servers{
			{URL: baseURL},
		},
	}

	// Initialize components
	components := openapi3.NewComponents()
	components.Schemas = openapi3.Schemas{}
	components.SecuritySchemes = openapi3.SecuritySchemes{}
	doc.Components = &components

	// Add security schemes
	doc.Components.SecuritySchemes["apiKey"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type: "apiKey",
			In:   "header",
			Name: "X-API-Key",
		},
	}
	doc.Components.SecuritySchemes["bearerAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		},
	}

	doc.Security = openapi3.SecurityRequirements{
		{"apiKey": {}},
		{"bearerAuth": {}},
	}

	doc.Paths = openapi3.NewPaths()

	// Add shared error response schema
	doc.Components.Schemas["ErrorResponse"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"error": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"code":    &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"}},
							"message": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"context": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}},
						},
					},
				},
			},
		},
	}

	// Generate paths and schemas for each table
	for _, table := range schema.Tables {
		addTablePaths(doc, serviceName, table)
	}
	for _, view := range schema.Views {
		addViewPaths(doc, serviceName, view)
	}

	// Generate paths for stored procedures
	for _, proc := range schema.Procedures {
		addProcedurePath(doc, serviceName, proc)
	}
	for _, fn := range schema.Functions {
		addProcedurePath(doc, serviceName, fn)
	}

	return doc
}

// GenerateCombinedSpec combines OpenAPI specs from multiple services into a single spec.
func GenerateCombinedSpec(services []ServiceSpec, baseURL string) *openapi3.T {
	doc := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:       "Faucet API",
			Description: "Combined REST API for all database services managed by Faucet.",
			Version:     "1.0.0",
		},
		Servers: openapi3.Servers{
			{URL: baseURL},
		},
	}

	components := openapi3.NewComponents()
	components.Schemas = openapi3.Schemas{}
	components.SecuritySchemes = openapi3.SecuritySchemes{}
	doc.Components = &components

	// Add security schemes
	doc.Components.SecuritySchemes["apiKey"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type: "apiKey",
			In:   "header",
			Name: "X-API-Key",
		},
	}
	doc.Components.SecuritySchemes["bearerAuth"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		},
	}

	doc.Security = openapi3.SecurityRequirements{
		{"apiKey": {}},
		{"bearerAuth": {}},
	}

	doc.Paths = openapi3.NewPaths()

	// Add shared error response schema
	doc.Components.Schemas["ErrorResponse"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"error": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"code":    &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"}},
							"message": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
							"context": &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"object"}}},
						},
					},
				},
			},
		},
	}

	// Merge each service's tables/views/procs into the combined doc under namespaced paths
	for _, svc := range services {
		if svc.Schema == nil {
			continue
		}
		for _, table := range svc.Schema.Tables {
			addServiceTablePaths(doc, svc.Name, table)
		}
		for _, view := range svc.Schema.Views {
			addServiceViewPaths(doc, svc.Name, view)
		}
		for _, proc := range svc.Schema.Procedures {
			addServiceProcedurePath(doc, svc.Name, proc)
		}
		for _, fn := range svc.Schema.Functions {
			addServiceProcedurePath(doc, svc.Name, fn)
		}
	}

	return doc
}

// addTablePaths generates all CRUD paths for a table in a single-service spec.
func addTablePaths(doc *openapi3.T, serviceName string, table model.TableSchema) {
	tablePath := fmt.Sprintf("/api/v1/%s/_table/%s", serviceName, table.Name)
	schemaPath := fmt.Sprintf("/api/v1/%s/_schema/%s", serviceName, table.Name)
	tag := table.Name

	// Register component schemas
	schemaName := sanitizeSchemaName(serviceName, table.Name)
	doc.Components.Schemas[schemaName] = columnsToSchema(table.Columns)
	doc.Components.Schemas[schemaName+"Create"] = columnsToCreateSchema(table.Columns)
	doc.Components.Schemas[schemaName+"Update"] = columnsToUpdateSchema(table.Columns)

	schemaRef := fmt.Sprintf("#/components/schemas/%s", schemaName)
	createRef := fmt.Sprintf("#/components/schemas/%sCreate", schemaName)
	updateRef := fmt.Sprintf("#/components/schemas/%sUpdate", schemaName)

	// List/query response schema
	listResponseSchema := &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"resource": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:  &openapi3.Types{"array"},
						Items: openapi3.NewSchemaRef(schemaRef, nil),
					},
				},
				"meta": metaSchema(),
			},
		},
	}

	// Common query parameters for list endpoints
	queryParams := listQueryParameters()

	// Build PathItem for table CRUD
	pathItem := &openapi3.PathItem{
		Get:    listOperation(tag, table.Name, queryParams, listResponseSchema),
		Post:   createOperation(tag, table.Name, createRef, schemaRef),
		Put:    replaceOperation(tag, table.Name, createRef, schemaRef),
		Patch:  updateOperation(tag, table.Name, updateRef, schemaRef),
		Delete: deleteOperation(tag, table.Name),
	}
	doc.Paths.Set(tablePath, pathItem)

	// Schema endpoint
	doc.Paths.Set(schemaPath, &openapi3.PathItem{
		Get: schemaOperation(tag, table.Name),
	})
}

// addViewPaths generates read-only paths for a view.
func addViewPaths(doc *openapi3.T, serviceName string, view model.TableSchema) {
	viewPath := fmt.Sprintf("/api/v1/%s/_table/%s", serviceName, view.Name)
	schemaPath := fmt.Sprintf("/api/v1/%s/_schema/%s", serviceName, view.Name)
	tag := view.Name

	schemaName := sanitizeSchemaName(serviceName, view.Name)
	doc.Components.Schemas[schemaName] = columnsToSchema(view.Columns)

	schemaRef := fmt.Sprintf("#/components/schemas/%s", schemaName)

	listResponseSchema := &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"resource": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:  &openapi3.Types{"array"},
						Items: openapi3.NewSchemaRef(schemaRef, nil),
					},
				},
				"meta": metaSchema(),
			},
		},
	}

	queryParams := listQueryParameters()

	pathItem := &openapi3.PathItem{
		Get: listOperation(tag, view.Name, queryParams, listResponseSchema),
	}
	doc.Paths.Set(viewPath, pathItem)

	doc.Paths.Set(schemaPath, &openapi3.PathItem{
		Get: schemaOperation(tag, view.Name),
	})
}

// addProcedurePath generates a POST path for calling a stored procedure or function.
func addProcedurePath(doc *openapi3.T, serviceName string, proc model.StoredProcedure) {
	prefix := "_proc"
	if proc.Type == "function" {
		prefix = "_func"
	}
	procPath := fmt.Sprintf("/api/v1/%s/%s/%s", serviceName, prefix, proc.Name)
	tag := proc.Name

	// Build request body schema from parameters
	reqProps := openapi3.Schemas{}
	for _, p := range proc.Parameters {
		if p.Direction == "in" || p.Direction == "inout" {
			m := MapDBType(p.Type)
			reqProps[p.Name] = &openapi3.SchemaRef{
				Value: columnTypeSchema(m),
			}
		}
	}

	reqSchema := &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Properties: reqProps,
		},
	}

	op := &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("Call %s %s", proc.Type, proc.Name),
		OperationID: fmt.Sprintf("call_%s_%s", proc.Type, proc.Name),
		RequestBody: &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Description: fmt.Sprintf("Parameters for %s", proc.Name),
				Content:     openapi3.NewContentWithJSONSchemaRef(reqSchema),
			},
		},
		Responses: newResponses(
			"200", "Successful execution", &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
			},
		),
	}

	doc.Paths.Set(procPath, &openapi3.PathItem{Post: op})
}

// addServiceTablePaths is like addTablePaths but for combined spec (same logic, included for clarity).
func addServiceTablePaths(doc *openapi3.T, serviceName string, table model.TableSchema) {
	addTablePaths(doc, serviceName, table)
}

// addServiceViewPaths is like addViewPaths but for combined spec.
func addServiceViewPaths(doc *openapi3.T, serviceName string, view model.TableSchema) {
	addViewPaths(doc, serviceName, view)
}

// addServiceProcedurePath is like addProcedurePath but for combined spec.
func addServiceProcedurePath(doc *openapi3.T, serviceName string, proc model.StoredProcedure) {
	addProcedurePath(doc, serviceName, proc)
}

// ─── Schema Builders ────────────────────────────────────────────────────────

// columnsToSchema converts table columns to an OpenAPI object schema with all columns as properties.
func columnsToSchema(columns []model.Column) *openapi3.SchemaRef {
	props := openapi3.Schemas{}
	for _, col := range columns {
		m := MapDBType(col.Type)
		s := columnTypeSchema(m)
		s.Description = col.Comment
		if col.Nullable {
			s.Nullable = true
		}
		if col.MaxLength != nil {
			ml := uint64(*col.MaxLength)
			s.MaxLength = &ml
		}
		if col.IsAutoIncrement {
			s.ReadOnly = true
		}
		props[col.Name] = &openapi3.SchemaRef{Value: s}
	}
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Properties: props,
		},
	}
}

// columnsToCreateSchema generates a schema for record creation (POST).
// Auto-increment columns are excluded. Non-nullable columns without defaults are required.
func columnsToCreateSchema(columns []model.Column) *openapi3.SchemaRef {
	props := openapi3.Schemas{}
	var required []string

	for _, col := range columns {
		// Skip auto-increment columns - the database generates these
		if col.IsAutoIncrement {
			continue
		}

		m := MapDBType(col.Type)
		s := columnTypeSchema(m)
		s.Description = col.Comment
		if col.Nullable {
			s.Nullable = true
		}
		if col.MaxLength != nil {
			ml := uint64(*col.MaxLength)
			s.MaxLength = &ml
		}

		props[col.Name] = &openapi3.SchemaRef{Value: s}

		// Non-nullable columns without a default value are required on create
		if !col.Nullable && col.Default == nil {
			required = append(required, col.Name)
		}
	}

	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Properties: props,
			Required:   required,
		},
	}
}

// columnsToUpdateSchema generates a schema for record updates (PATCH).
// All fields are optional since you only send what you want to change.
func columnsToUpdateSchema(columns []model.Column) *openapi3.SchemaRef {
	props := openapi3.Schemas{}
	for _, col := range columns {
		// Skip auto-increment columns - cannot be updated
		if col.IsAutoIncrement {
			continue
		}

		m := MapDBType(col.Type)
		s := columnTypeSchema(m)
		s.Description = col.Comment
		if col.Nullable {
			s.Nullable = true
		}
		if col.MaxLength != nil {
			ml := uint64(*col.MaxLength)
			s.MaxLength = &ml
		}

		props[col.Name] = &openapi3.SchemaRef{Value: s}
	}

	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Properties: props,
		},
	}
}

// columnTypeSchema creates a basic Schema for the given type mapping.
func columnTypeSchema(m TypeMapping) *openapi3.Schema {
	s := &openapi3.Schema{
		Type: &openapi3.Types{m.Type},
	}
	if m.Format != "" {
		s.Format = m.Format
	}
	// For array types, add items schema
	if m.Type == "array" {
		s.Items = &openapi3.SchemaRef{Value: &openapi3.Schema{}}
	}
	return s
}

// ─── Operation Builders ─────────────────────────────────────────────────────

// listOperation generates a GET operation for listing/querying records.
func listOperation(tag, tableName string, params openapi3.Parameters, responseSchema *openapi3.SchemaRef) *openapi3.Operation {
	return &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("List %s records", tableName),
		Description: fmt.Sprintf("Retrieve records from %s with optional filtering, sorting, and pagination.", tableName),
		OperationID: fmt.Sprintf("get_%s", tableName),
		Parameters:  params,
		Responses: newResponses(
			"200", fmt.Sprintf("List of %s records", tableName), responseSchema,
		),
	}
}

// createOperation generates a POST operation for creating records.
func createOperation(tag, tableName, createRef, schemaRef string) *openapi3.Operation {
	reqBody := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Description: fmt.Sprintf("Record(s) to create in %s", tableName),
			Required:    true,
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							OneOf: openapi3.SchemaRefs{
								openapi3.NewSchemaRef(createRef, nil),
								{
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"object"},
										Properties: openapi3.Schemas{
											"resource": &openapi3.SchemaRef{
												Value: &openapi3.Schema{
													Type:  &openapi3.Types{"array"},
													Items: openapi3.NewSchemaRef(createRef, nil),
												},
											},
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

	return &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("Create %s record(s)", tableName),
		Description: fmt.Sprintf("Create one or more records in %s. Send a single object or {\"resource\": [...]} for batch.", tableName),
		OperationID: fmt.Sprintf("create_%s", tableName),
		RequestBody: reqBody,
		Responses: newResponses(
			"201", fmt.Sprintf("Created %s record(s)", tableName), &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: openapi3.Schemas{
						"resource": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type:  &openapi3.Types{"array"},
								Items: openapi3.NewSchemaRef(schemaRef, nil),
							},
						},
					},
				},
			},
		),
	}
}

// replaceOperation generates a PUT operation for replacing records.
func replaceOperation(tag, tableName, createRef, schemaRef string) *openapi3.Operation {
	reqBody := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Description: fmt.Sprintf("Record(s) to replace in %s (full replacement)", tableName),
			Required:    true,
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: openapi3.Schemas{
								"resource": &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type:  &openapi3.Types{"array"},
										Items: openapi3.NewSchemaRef(createRef, nil),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("Replace %s record(s)", tableName),
		Description: fmt.Sprintf("Replace existing records in %s. Records are matched by primary key.", tableName),
		OperationID: fmt.Sprintf("replace_%s", tableName),
		RequestBody: reqBody,
		Parameters:  writeQueryParameters(),
		Responses: newResponses(
			"200", fmt.Sprintf("Replaced %s record(s)", tableName), &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: openapi3.Schemas{
						"resource": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type:  &openapi3.Types{"array"},
								Items: openapi3.NewSchemaRef(schemaRef, nil),
							},
						},
					},
				},
			},
		),
	}
}

// updateOperation generates a PATCH operation for partial updates.
func updateOperation(tag, tableName, updateRef, schemaRef string) *openapi3.Operation {
	reqBody := &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Description: fmt.Sprintf("Fields to update in %s record(s)", tableName),
			Required:    true,
			Content: openapi3.Content{
				"application/json": &openapi3.MediaType{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: openapi3.Schemas{
								"resource": &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type:  &openapi3.Types{"array"},
										Items: openapi3.NewSchemaRef(updateRef, nil),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("Update %s record(s)", tableName),
		Description: fmt.Sprintf("Update fields in one or more %s records. Only provided fields are changed.", tableName),
		OperationID: fmt.Sprintf("update_%s", tableName),
		RequestBody: reqBody,
		Parameters:  writeQueryParameters(),
		Responses: newResponses(
			"200", fmt.Sprintf("Updated %s record(s)", tableName), &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: openapi3.Schemas{
						"resource": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type:  &openapi3.Types{"array"},
								Items: openapi3.NewSchemaRef(schemaRef, nil),
							},
						},
					},
				},
			},
		),
	}
}

// deleteOperation generates a DELETE operation for removing records.
func deleteOperation(tag, tableName string) *openapi3.Operation {
	filterParam := openapi3.NewQueryParameter("filter").
		WithDescription("Filter expression to select records to delete (e.g. \"id=5\" or \"status='inactive'\").").
		WithSchema(openapi3.NewStringSchema())

	idsParam := openapi3.NewQueryParameter("ids").
		WithDescription("Comma-separated list of primary key values to delete.").
		WithSchema(openapi3.NewStringSchema())

	return &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("Delete %s record(s)", tableName),
		Description: fmt.Sprintf("Delete one or more records from %s by filter or IDs.", tableName),
		OperationID: fmt.Sprintf("delete_%s", tableName),
		Parameters: openapi3.Parameters{
			&openapi3.ParameterRef{Value: filterParam},
			&openapi3.ParameterRef{Value: idsParam},
		},
		Responses: newResponses(
			"200", fmt.Sprintf("Deleted %s record(s)", tableName), &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: openapi3.Schemas{
						"resource": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"array"},
								Items: &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"object"},
									},
								},
							},
						},
					},
				},
			},
		),
	}
}

// schemaOperation generates a GET operation for retrieving table/view schema metadata.
func schemaOperation(tag, tableName string) *openapi3.Operation {
	return &openapi3.Operation{
		Tags:        []string{tag},
		Summary:     fmt.Sprintf("Get %s schema", tableName),
		Description: fmt.Sprintf("Retrieve the column definitions and metadata for %s.", tableName),
		OperationID: fmt.Sprintf("schema_%s", tableName),
		Responses: newResponses(
			"200", fmt.Sprintf("Schema for %s", tableName), &openapi3.SchemaRef{
				Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
			},
		),
	}
}

// ─── Query Parameter Builders ───────────────────────────────────────────────

// listQueryParameters returns the standard query parameters for list/query endpoints.
func listQueryParameters() openapi3.Parameters {
	return openapi3.Parameters{
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("filter").
				WithDescription("Filter expression (e.g. \"age>21\", \"name='John'\", \"status IN ('active','pending')\").").
				WithSchema(openapi3.NewStringSchema()),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("order").
				WithDescription("Sort order (e.g. \"name ASC\", \"created_at DESC\").").
				WithSchema(openapi3.NewStringSchema()),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("limit").
				WithDescription("Maximum number of records to return.").
				WithSchema(&openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"}),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("offset").
				WithDescription("Number of records to skip before returning results.").
				WithSchema(&openapi3.Schema{Type: &openapi3.Types{"integer"}, Format: "int32"}),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("fields").
				WithDescription("Comma-separated list of fields to include in the response.").
				WithSchema(openapi3.NewStringSchema()),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("ids").
				WithDescription("Comma-separated list of primary key values to retrieve.").
				WithSchema(openapi3.NewStringSchema()),
		},
		&openapi3.ParameterRef{
			Value: func() *openapi3.Parameter {
				p := openapi3.NewQueryParameter("count")
				p.Description = "Include total record count in meta (\"true\" to enable)."
				p.Schema = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"boolean"}}}
				return p
			}(),
		},
	}
}

// writeQueryParameters returns query parameters for write operations (PUT/PATCH) that use filters.
func writeQueryParameters() openapi3.Parameters {
	return openapi3.Parameters{
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("filter").
				WithDescription("Filter expression to select records to update.").
				WithSchema(openapi3.NewStringSchema()),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("ids").
				WithDescription("Comma-separated list of primary key values to update.").
				WithSchema(openapi3.NewStringSchema()),
		},
		&openapi3.ParameterRef{
			Value: openapi3.NewQueryParameter("fields").
				WithDescription("Comma-separated list of fields to return after the operation.").
				WithSchema(openapi3.NewStringSchema()),
		},
	}
}

// ─── Response Helpers ───────────────────────────────────────────────────────

// newResponses builds a Responses map with a success response and standard error responses.
func newResponses(statusCode, description string, schema *openapi3.SchemaRef) *openapi3.Responses {
	responses := openapi3.NewResponses()

	// Success response
	successDesc := description
	responses.Set(statusCode, &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: &successDesc,
			Content:     openapi3.NewContentWithJSONSchemaRef(schema),
		},
	})

	// Standard error responses
	errorRef := openapi3.NewSchemaRef("#/components/schemas/ErrorResponse", nil)

	badReqDesc := "Bad request"
	responses.Set("400", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: &badReqDesc,
			Content:     openapi3.NewContentWithJSONSchemaRef(errorRef),
		},
	})

	unauthDesc := "Unauthorized"
	responses.Set("401", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: &unauthDesc,
			Content:     openapi3.NewContentWithJSONSchemaRef(errorRef),
		},
	})

	notFoundDesc := "Not found"
	responses.Set("404", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: &notFoundDesc,
			Content:     openapi3.NewContentWithJSONSchemaRef(errorRef),
		},
	})

	serverErrDesc := "Internal server error"
	responses.Set("500", &openapi3.ResponseRef{
		Value: &openapi3.Response{
			Description: &serverErrDesc,
			Content:     openapi3.NewContentWithJSONSchemaRef(errorRef),
		},
	})

	return responses
}

// metaSchema returns the schema for the "meta" field in list responses.
func metaSchema() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"count": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Format:      "int64",
						Description: "Total number of records matching the query.",
					},
				},
				"limit": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Format:      "int32",
						Description: "Maximum records returned per page.",
					},
				},
				"offset": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:        &openapi3.Types{"integer"},
						Format:      "int32",
						Description: "Number of records skipped.",
					},
				},
			},
		},
	}
}

// ─── Naming Helpers ─────────────────────────────────────────────────────────

// sanitizeSchemaName creates a valid OpenAPI component schema name from service + table names.
func sanitizeSchemaName(serviceName, tableName string) string {
	// Capitalize first letter of each part for PascalCase style
	s := capitalize(serviceName) + "_" + capitalize(tableName)
	// Replace any non-alphanumeric chars (except underscore) with underscore
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// capitalize returns a string with its first character uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
