package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/query"
)

// registerTools registers all Faucet MCP tools on the given server.
func (s *MCPServer) registerTools(srv *server.MCPServer) {

	// ----- Discovery tools -----

	srv.AddTool(
		mcp.NewTool("faucet_list_services",
			mcp.WithDescription(
				"List all database services configured in Faucet. Returns each service's "+
					"name, driver type, active status, and access mode. Use this first to "+
					"discover available databases before querying.",
			),
			mcp.WithToolAnnotation(readOnlyAnnotation()),
		),
		s.handleListServices,
	)

	srv.AddTool(
		mcp.NewTool("faucet_list_tables",
			mcp.WithDescription(
				"List all tables in a database service, including approximate row counts "+
					"and column summaries. Use this to explore what data is available before "+
					"querying specific tables.",
			),
			mcp.WithToolAnnotation(readOnlyAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service to list tables for"),
			),
		),
		s.handleListTables,
	)

	srv.AddTool(
		mcp.NewTool("faucet_describe_table",
			mcp.WithDescription(
				"Get the detailed schema for a specific table, including all columns "+
					"with their types, nullability, defaults, primary keys, foreign keys, "+
					"and indexes. Use this to understand table structure before writing queries.",
			),
			mcp.WithToolAnnotation(readOnlyAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service"),
			),
			mcp.WithString("table",
				mcp.Required(),
				mcp.Description("Name of the table to describe"),
			),
		),
		s.handleDescribeTable,
	)

	// ----- Query tool -----

	srv.AddTool(
		mcp.NewTool("faucet_query",
			mcp.WithDescription(
				"Query records from a database table with optional filtering, field "+
					"selection, ordering, and pagination. Returns results as JSON.\n\n"+
					"Filter syntax (DreamFactory-compatible):\n"+
					"  - Simple: name = 'John'\n"+
					"  - Comparison: age > 21, price <= 100\n"+
					"  - Logical: status = 'active' AND role = 'admin'\n"+
					"  - IN: status IN ('active', 'pending')\n"+
					"  - LIKE: name LIKE 'J%'\n"+
					"  - NULL: email IS NOT NULL\n"+
					"  - BETWEEN: age BETWEEN 18 AND 65\n"+
					"  - CONTAINS: name CONTAINS 'smith'\n\n"+
					"Order syntax: 'column ASC, other_column DESC'",
			),
			mcp.WithToolAnnotation(readOnlyAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service"),
			),
			mcp.WithString("table",
				mcp.Required(),
				mcp.Description("Name of the table to query"),
			),
			mcp.WithString("filter",
				mcp.Description("Filter expression (e.g. \"status = 'active' AND age > 21\")"),
			),
			mcp.WithArray("fields",
				mcp.Description("List of column names to return. Omit for all columns."),
				mcp.WithStringItems(),
			),
			mcp.WithString("order",
				mcp.Description("Order clause (e.g. \"created_at DESC, name ASC\")"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of records to return (default 25, max 1000)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Number of records to skip for pagination"),
			),
		),
		s.handleQuery,
	)

	// ----- Mutation tools -----

	srv.AddTool(
		mcp.NewTool("faucet_insert",
			mcp.WithDescription(
				"Insert one or more records into a database table. Each record is a "+
					"JSON object mapping column names to values. Returns the inserted "+
					"records (with auto-generated fields like IDs) if the database supports RETURNING.",
			),
			mcp.WithToolAnnotation(mutatingAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service"),
			),
			mcp.WithString("table",
				mcp.Required(),
				mcp.Description("Name of the table to insert into"),
			),
			mcp.WithArray("records",
				mcp.Required(),
				mcp.Description("Array of record objects to insert (e.g. [{\"name\": \"Alice\", \"age\": 30}])"),
			),
		),
		s.handleInsert,
	)

	srv.AddTool(
		mcp.NewTool("faucet_update",
			mcp.WithDescription(
				"Update records in a database table that match a filter expression. "+
					"The record object contains the column values to set. A filter is "+
					"required to prevent accidental full-table updates.",
			),
			mcp.WithToolAnnotation(mutatingAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service"),
			),
			mcp.WithString("table",
				mcp.Required(),
				mcp.Description("Name of the table to update"),
			),
			mcp.WithString("filter",
				mcp.Required(),
				mcp.Description("Filter expression to select records to update (e.g. \"id = 42\")"),
			),
			mcp.WithObject("record",
				mcp.Required(),
				mcp.Description("Object with column names and new values (e.g. {\"status\": \"archived\"})"),
			),
		),
		s.handleUpdate,
	)

	srv.AddTool(
		mcp.NewTool("faucet_delete",
			mcp.WithDescription(
				"Delete records from a database table that match a filter expression. "+
					"A filter is required to prevent accidental full-table deletes. "+
					"Returns the number of deleted records.",
			),
			mcp.WithToolAnnotation(mutatingAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service"),
			),
			mcp.WithString("table",
				mcp.Required(),
				mcp.Description("Name of the table to delete from"),
			),
			mcp.WithString("filter",
				mcp.Required(),
				mcp.Description("Filter expression to select records to delete (e.g. \"status = 'expired'\")"),
			),
		),
		s.handleDelete,
	)

	// ----- Raw SQL tool -----

	srv.AddTool(
		mcp.NewTool("faucet_raw_sql",
			mcp.WithDescription(
				"Execute a raw SQL query against a database service. Only available "+
					"for services with raw_sql_allowed enabled. Use faucet_list_services "+
					"to check which services allow raw SQL.\n\n"+
					"The query is executed read-only by default. Parameters should be "+
					"passed as an array and referenced with positional placeholders "+
					"($1, $2 for PostgreSQL; ?, ? for MySQL).",
			),
			mcp.WithToolAnnotation(readOnlyAnnotation()),
			mcp.WithString("service",
				mcp.Required(),
				mcp.Description("Name of the database service"),
			),
			mcp.WithString("sql",
				mcp.Required(),
				mcp.Description("SQL query to execute"),
			),
			mcp.WithArray("params",
				mcp.Description("Positional parameters for the SQL query"),
			),
			mcp.WithNumber("timeout",
				mcp.Description("Query timeout in seconds (default 30)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of rows to return (default 100, max 10000)"),
			),
		),
		s.handleRawSQL,
	)
}

// =========================================================================
// Tool handlers
// =========================================================================

// handleListServices returns all configured database services.
func (s *MCPServer) handleListServices(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	services, err := s.store.ListServices(ctx)
	if err != nil {
		return toolError("Failed to list services: %v", err)
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

	return successJSON(items)
}

// handleListTables returns all tables in a service with row counts and column summaries.
func (s *MCPServer) handleListTables(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	schema, err := conn.IntrospectSchema(ctx)
	if err != nil {
		return toolError("Failed to introspect schema for %q: %v", serviceName, err)
	}

	type columnSummary struct {
		Name string `json:"name"`
		Type string `json:"type"`
		PK   bool   `json:"pk,omitempty"`
	}

	type tableInfo struct {
		Name     string          `json:"name"`
		Type     string          `json:"type"`
		RowCount *int64          `json:"row_count,omitempty"`
		Columns  []columnSummary `json:"columns"`
	}

	tables := make([]tableInfo, 0, len(schema.Tables)+len(schema.Views))
	for _, t := range schema.Tables {
		cols := make([]columnSummary, len(t.Columns))
		for i, c := range t.Columns {
			cols[i] = columnSummary{
				Name: c.Name,
				Type: c.Type,
				PK:   c.IsPrimaryKey,
			}
		}
		tables = append(tables, tableInfo{
			Name:     t.Name,
			Type:     "table",
			RowCount: t.RowCount,
			Columns:  cols,
		})
	}
	for _, v := range schema.Views {
		cols := make([]columnSummary, len(v.Columns))
		for i, c := range v.Columns {
			cols[i] = columnSummary{
				Name: c.Name,
				Type: c.Type,
			}
		}
		tables = append(tables, tableInfo{
			Name:     v.Name,
			Type:     "view",
			RowCount: v.RowCount,
			Columns:  cols,
		})
	}

	return successJSON(tables)
}

// handleDescribeTable returns detailed schema for a specific table.
func (s *MCPServer) handleDescribeTable(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}
	tableName, err := requireString(request, "table")
	if err != nil {
		return toolError("%v", err)
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	table, err := conn.IntrospectTable(ctx, tableName)
	if err != nil {
		// Provide available table names to help the LLM self-correct.
		names, _ := conn.GetTableNames(ctx)
		return toolError("Table %q not found in service %q: %v\n\nAvailable tables: %v",
			tableName, serviceName, err, names)
	}

	return successJSON(table)
}

// handleQuery queries records from a table.
func (s *MCPServer) handleQuery(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}
	tableName, err := requireString(request, "table")
	if err != nil {
		return toolError("%v", err)
	}

	filterStr := optionalString(request, "filter")
	fields := optionalStringSlice(request, "fields")
	orderStr := optionalString(request, "order")
	limit := clamp(optionalInt(request, "limit", 25), 1, 1000)
	offset := optionalInt(request, "offset", 0)
	if offset < 0 {
		offset = 0
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	// Parse and validate fields.
	if len(fields) > 0 {
		validated, err := query.ParseFieldSelection(strings.Join(fields, ","))
		if err != nil {
			tableSchema, _ := conn.IntrospectTable(ctx, tableName)
			var colNames []string
			if tableSchema != nil {
				for _, c := range tableSchema.Columns {
					colNames = append(colNames, c.Name)
				}
			}
			return toolError("Invalid fields: %v\n\nAvailable columns: %v", err, colNames)
		}
		fields = validated
	}

	// Parse filter expression into parameterized SQL.
	var filterSQL string
	var filterParams []interface{}
	if filterStr != "" {
		phFunc := func(index int) string {
			return conn.ParameterPlaceholder(index)
		}
		parsed, err := query.ParseFilter(filterStr, phFunc, 1)
		if err != nil {
			return toolError("Invalid filter expression: %v\n\n"+
				"Filter syntax: column op value\n"+
				"  Operators: =, !=, <, >, <=, >=, LIKE, IN, IS NULL, IS NOT NULL, BETWEEN, CONTAINS\n"+
				"  Logical: AND, OR, NOT\n"+
				"  Example: status = 'active' AND age > 21", err)
		}
		if parsed != nil {
			filterSQL = parsed.SQL
			filterParams = parsed.Params
		}
	}

	// Parse order clause.
	var orderSQL string
	if orderStr != "" {
		clauses, err := query.ParseOrderClause(orderStr)
		if err != nil {
			return toolError("Invalid order clause: %v\n\n"+
				"Order syntax: column [ASC|DESC], ...\n"+
				"  Example: created_at DESC, name ASC", err)
		}
		orderSQL = query.BuildOrderSQL(clauses, conn.QuoteIdentifier)
		orderSQL = strings.TrimPrefix(orderSQL, "ORDER BY ")
	}

	// Build and execute the query.
	selectReq := connector.SelectRequest{
		Table:  tableName,
		Fields: fields,
		Filter: filterSQL,
		Order:  orderSQL,
		Limit:  limit,
		Offset: offset,
	}

	sqlStr, args, err := conn.BuildSelect(ctx, selectReq)
	if err != nil {
		names, _ := conn.GetTableNames(ctx)
		return toolError("Failed to build query: %v\n\nAvailable tables: %v", err, names)
	}

	allArgs := append(filterParams, args...)
	db := conn.DB()
	rows, err := db.QueryxContext(ctx, sqlStr, allArgs...)
	if err != nil {
		return toolError("Query execution failed: %v", err)
	}
	defer rows.Close()

	records := make([]map[string]interface{}, 0)
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return toolError("Failed to scan row: %v", err)
		}
		cleanMapValues(row)
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		return toolError("Row iteration error: %v", err)
	}

	result := map[string]interface{}{
		"records": records,
		"count":   len(records),
		"limit":   limit,
		"offset":  offset,
	}

	return successJSON(result)
}

// handleInsert inserts records into a table.
func (s *MCPServer) handleInsert(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}
	tableName, err := requireString(request, "table")
	if err != nil {
		return toolError("%v", err)
	}

	// Check read-only status.
	svc, err := s.store.GetServiceByName(ctx, serviceName)
	if err == nil && svc.ReadOnly {
		return toolError("Service %q is read-only. Insert operations are not permitted.", serviceName)
	}

	records := getObjectSliceArg(request, "records")
	if len(records) == 0 {
		return toolError("No records provided. The 'records' parameter must be an array "+
			"of objects, e.g. [{\"name\": \"Alice\", \"age\": 30}]")
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	insertReq := connector.InsertRequest{
		Table:   tableName,
		Records: records,
	}

	sqlStr, args, err := conn.BuildInsert(ctx, insertReq)
	if err != nil {
		names, _ := conn.GetTableNames(ctx)
		return toolError("Failed to build insert: %v\n\nAvailable tables: %v", err, names)
	}

	db := conn.DB()

	if conn.SupportsReturning() {
		rows, err := db.QueryxContext(ctx, sqlStr, args...)
		if err != nil {
			return toolError("Insert failed: %v", err)
		}
		defer rows.Close()

		created := make([]map[string]interface{}, 0)
		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				return toolError("Failed to scan returned row: %v", err)
			}
			cleanMapValues(row)
			created = append(created, row)
		}
		if err := rows.Err(); err != nil {
			return toolError("Row iteration error: %v", err)
		}

		return successJSON(map[string]interface{}{
			"inserted": created,
			"count":    len(created),
		})
	}

	// Connectors without RETURNING.
	result, err := db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return toolError("Insert failed: %v", err)
	}

	affected, _ := result.RowsAffected()
	return successJSON(map[string]interface{}{
		"count": affected,
	})
}

// handleUpdate updates records matching a filter.
func (s *MCPServer) handleUpdate(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}
	tableName, err := requireString(request, "table")
	if err != nil {
		return toolError("%v", err)
	}
	filterStr, err := requireString(request, "filter")
	if err != nil {
		return toolError("A filter is required for update operations to prevent "+
			"accidental full-table updates. Example: id = 42")
	}

	// Check read-only status.
	svc, err := s.store.GetServiceByName(ctx, serviceName)
	if err == nil && svc.ReadOnly {
		return toolError("Service %q is read-only. Update operations are not permitted.", serviceName)
	}

	record := getObjectArg(request, "record")
	if len(record) == 0 {
		return toolError("No fields to update. The 'record' parameter must be an object "+
			"with column names and values, e.g. {\"status\": \"archived\"}")
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	// Parse filter.
	phFunc := func(index int) string {
		return conn.ParameterPlaceholder(index)
	}
	parsed, err := query.ParseFilter(filterStr, phFunc, 1)
	if err != nil {
		return toolError("Invalid filter expression: %v", err)
	}

	var filterSQL string
	var filterParams []interface{}
	if parsed != nil {
		filterSQL = parsed.SQL
		filterParams = parsed.Params
	}

	updateReq := connector.UpdateRequest{
		Table:  tableName,
		Record: record,
		Filter: filterSQL,
	}

	sqlStr, args, err := conn.BuildUpdate(ctx, updateReq)
	if err != nil {
		return toolError("Failed to build update: %v", err)
	}

	allArgs := append(filterParams, args...)
	db := conn.DB()

	if conn.SupportsReturning() {
		rows, err := db.QueryxContext(ctx, sqlStr, allArgs...)
		if err != nil {
			return toolError("Update failed: %v", err)
		}
		defer rows.Close()

		updated := make([]map[string]interface{}, 0)
		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				return toolError("Failed to scan returned row: %v", err)
			}
			cleanMapValues(row)
			updated = append(updated, row)
		}
		if err := rows.Err(); err != nil {
			return toolError("Row iteration error: %v", err)
		}

		return successJSON(map[string]interface{}{
			"updated": updated,
			"count":   len(updated),
		})
	}

	result, err := db.ExecContext(ctx, sqlStr, allArgs...)
	if err != nil {
		return toolError("Update failed: %v", err)
	}

	affected, _ := result.RowsAffected()
	return successJSON(map[string]interface{}{
		"count": affected,
	})
}

// handleDelete deletes records matching a filter.
func (s *MCPServer) handleDelete(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}
	tableName, err := requireString(request, "table")
	if err != nil {
		return toolError("%v", err)
	}
	filterStr, err := requireString(request, "filter")
	if err != nil {
		return toolError("A filter is required for delete operations to prevent "+
			"accidental full-table deletes. Example: id = 42")
	}

	// Check read-only status.
	svc, err := s.store.GetServiceByName(ctx, serviceName)
	if err == nil && svc.ReadOnly {
		return toolError("Service %q is read-only. Delete operations are not permitted.", serviceName)
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	// Parse filter.
	phFunc := func(index int) string {
		return conn.ParameterPlaceholder(index)
	}
	parsed, err := query.ParseFilter(filterStr, phFunc, 1)
	if err != nil {
		return toolError("Invalid filter expression: %v", err)
	}

	var filterSQL string
	var filterParams []interface{}
	if parsed != nil {
		filterSQL = parsed.SQL
		filterParams = parsed.Params
	}

	deleteReq := connector.DeleteRequest{
		Table:  tableName,
		Filter: filterSQL,
	}

	sqlStr, args, err := conn.BuildDelete(ctx, deleteReq)
	if err != nil {
		return toolError("Failed to build delete: %v", err)
	}

	allArgs := append(filterParams, args...)
	db := conn.DB()

	result, err := db.ExecContext(ctx, sqlStr, allArgs...)
	if err != nil {
		return toolError("Delete failed: %v", err)
	}

	affected, _ := result.RowsAffected()
	return successJSON(map[string]interface{}{
		"deleted": affected,
	})
}

// handleRawSQL executes a raw SQL query against a service.
func (s *MCPServer) handleRawSQL(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {

	serviceName, err := requireString(request, "service")
	if err != nil {
		return toolError("%v. Available services: %v", err, s.registry.ListServices())
	}
	sqlStr, err := requireString(request, "sql")
	if err != nil {
		return toolError("%v", err)
	}

	params := getAnySliceArg(request, "params")
	timeoutSec := optionalInt(request, "timeout", 30)
	limit := clamp(optionalInt(request, "limit", 100), 1, 10000)

	// Check that the service allows raw SQL.
	svc, err := s.store.GetServiceByName(ctx, serviceName)
	if err != nil {
		return toolError("Service %q not found. Available services: %v",
			serviceName, s.registry.ListServices())
	}
	if !svc.RawSQL {
		return toolError("Raw SQL is not enabled for service %q. "+
			"Use the structured query tools (faucet_query, faucet_insert, etc.) instead, "+
			"or ask the administrator to enable raw_sql_allowed for this service.", serviceName)
	}

	conn, err := s.registry.Get(serviceName)
	if err != nil {
		return toolError("Service %q not connected. Available services: %v",
			serviceName, s.registry.ListServices())
	}

	// Apply timeout.
	if timeoutSec < 1 {
		timeoutSec = 30
	}
	if timeoutSec > 300 {
		timeoutSec = 300
	}
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Convert params to []interface{} for the query.
	var queryParams []interface{}
	if len(params) > 0 {
		queryParams = params
	}

	db := conn.DB()
	rows, err := db.QueryxContext(queryCtx, sqlStr, queryParams...)
	if err != nil {
		return toolError("SQL execution failed: %v\n\nSQL: %s", err, sqlStr)
	}
	defer rows.Close()

	records := make([]map[string]interface{}, 0)
	rowCount := 0
	for rows.Next() {
		if rowCount >= limit {
			break
		}
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return toolError("Failed to scan row: %v", err)
		}
		cleanMapValues(row)
		records = append(records, row)
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return toolError("Row iteration error: %v", err)
	}

	truncated := false
	if rowCount >= limit {
		truncated = true
	}

	result := map[string]interface{}{
		"records":   records,
		"count":     len(records),
		"truncated": truncated,
	}
	if truncated {
		result["message"] = fmt.Sprintf(
			"Results truncated at %d rows. Increase the 'limit' parameter or add a WHERE clause to narrow results.",
			limit,
		)
	}

	return successJSON(result)
}

// cleanMapValues converts []byte values from database scans into strings
// for clean JSON serialization.
func cleanMapValues(m map[string]interface{}) {
	for k, v := range m {
		if b, ok := v.([]byte); ok {
			m[k] = string(b)
		}
	}
}
