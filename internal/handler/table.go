package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/query"
)

// TableHandler handles CRUD operations on database table records.
type TableHandler struct {
	registry *connector.Registry
	store    *config.Store
}

// NewTableHandler creates a new TableHandler.
func NewTableHandler(registry *connector.Registry, store *config.Store) *TableHandler {
	return &TableHandler{
		registry: registry,
		store:    store,
	}
}

// ListTableNames returns the names of all tables in the service's database.
// GET /api/v1/{serviceName}/_table
func (h *TableHandler) ListTableNames(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	names, err := conn.GetTableNames(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list tables: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: stringsToResources("name", names),
	})
}

// QueryRecords retrieves records from a table with optional filtering,
// sorting, field selection, and pagination.
// GET /api/v1/{serviceName}/_table/{tableName}
func (h *TableHandler) QueryRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Parse query parameters.
	filterStr := queryString(r, "filter")
	fieldsStr := queryString(r, "fields")
	orderStr := queryString(r, "order")
	limit := clampInt(queryInt(r, "limit", 25), 0, 1000)
	offset := queryInt(r, "offset", 0)
	includeCount := queryBool(r, "include_count")

	// Validate and parse fields.
	var fields []string
	if fieldsStr != "" {
		fields, err = query.ParseFieldSelection(fieldsStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid fields parameter: "+err.Error())
			return
		}
	}

	// Parse and parameterize the filter expression.
	var filterSQL string
	var filterParams []interface{}
	if filterStr != "" {
		phFunc := func(index int) string {
			return conn.ParameterPlaceholder(index)
		}
		parsed, err := query.ParseFilter(filterStr, phFunc, 1)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid filter: "+err.Error())
			return
		}
		if parsed != nil {
			filterSQL = parsed.SQL
			filterParams = parsed.Params
		}
	}

	// Parse and validate order clause.
	var orderSQL string
	if orderStr != "" {
		clauses, err := query.ParseOrderClause(orderStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid order parameter: "+err.Error())
			return
		}
		orderSQL = query.BuildOrderSQL(clauses, conn.QuoteIdentifier)
		// Strip the "ORDER BY " prefix since the connector adds it.
		orderSQL = strings.TrimPrefix(orderSQL, "ORDER BY ")
	}

	// Build and execute the SELECT query.
	selectReq := connector.SelectRequest{
		Table:  tableName,
		Fields: fields,
		Filter: filterSQL,
		Order:  orderSQL,
		Limit:  limit,
		Offset: offset,
	}

	sqlStr, args, err := conn.BuildSelect(r.Context(), selectReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build query: "+err.Error())
		return
	}

	// Merge filter params with query args. The filter params come first
	// (they are embedded in the SQL), and the connector may append LIMIT/OFFSET
	// params after.
	allArgs := append(filterParams, args...)

	db := conn.DB()
	rows, err := db.QueryxContext(r.Context(), sqlStr, allArgs...)
	if err != nil {
		code, msg := classifyDBError(err, "Query failed")
		writeError(w, code, msg)
		return
	}
	defer rows.Close()

	// Check Accept header for NDJSON streaming.
	acceptNDJSON := strings.Contains(r.Header.Get("Accept"), "application/x-ndjson")

	if acceptNDJSON {
		// Stream results as newline-delimited JSON.
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		enc := json.NewEncoder(w)
		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				// Can't change status code mid-stream; log and stop.
				return
			}
			cleanMapValues(row)
			enc.Encode(row)
		}
		return
	}

	// Collect results into a slice.
	records := make([]map[string]interface{}, 0)
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to scan row: "+err.Error())
			return
		}
		cleanMapValues(row)
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Row iteration error: "+err.Error())
		return
	}

	// Optionally fetch total count.
	var total *int64
	if includeCount {
		countReq := connector.CountRequest{
			Table:  tableName,
			Filter: filterSQL,
		}
		countSQL, countArgs, err := conn.BuildCount(r.Context(), countReq)
		if err == nil {
			allCountArgs := append(filterParams, countArgs...)
			var count int64
			if err := db.QueryRowxContext(r.Context(), countSQL, allCountArgs...).Scan(&count); err == nil {
				total = &count
			}
		}
	}

	took := time.Since(start)

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: records,
		Meta: &model.ResponseMeta{
			Count:  len(records),
			Total:  total,
			Limit:  limit,
			Offset: offset,
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}

// CreateRecords inserts one or more records into a table.
// POST /api/v1/{serviceName}/_table/{tableName}
func (h *TableHandler) CreateRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Parse request body: accept either a single object or {"resource": [...]}
	records, err := parseRecordsBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "No records provided")
		return
	}

	insertReq := connector.InsertRequest{
		Table:   tableName,
		Records: records,
	}

	sqlStr, args, err := conn.BuildInsert(r.Context(), insertReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build insert: "+err.Error())
		return
	}

	db := conn.DB()

	if conn.SupportsReturning() {
		// Execute with RETURNING to get the created records back.
		rows, err := db.QueryxContext(r.Context(), sqlStr, args...)
		if err != nil {
			code, msg := classifyDBError(err, "Insert failed")
			writeError(w, code, msg)
			return
		}
		defer rows.Close()

		created := make([]map[string]interface{}, 0)
		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to scan result: "+err.Error())
				return
			}
			cleanMapValues(row)
			created = append(created, row)
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "Row iteration error: "+err.Error())
			return
		}

		took := time.Since(start)
		writeJSON(w, http.StatusCreated, model.ListResponse{
			Resource: created,
			Meta: &model.ResponseMeta{
				Count:  len(created),
				TookMs: float64(took.Microseconds()) / 1000.0,
			},
		})
		return
	}

	// For connectors without RETURNING, execute and report rows affected.
	result, err := db.ExecContext(r.Context(), sqlStr, args...)
	if err != nil {
		code, msg := classifyDBError(err, "Insert failed")
		writeError(w, code, msg)
		return
	}

	affected, _ := result.RowsAffected()
	took := time.Since(start)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"resource": records,
		"meta": model.ResponseMeta{
			Count:  int(affected),
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}

// ReplaceRecords performs a full record replacement (PUT) on a table.
// PUT /api/v1/{serviceName}/_table/{tableName}
func (h *TableHandler) ReplaceRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Parse request body.
	records, err := parseRecordsBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "No records provided")
		return
	}

	db := conn.DB()
	updated := make([]map[string]interface{}, 0)

	// Each record in a PUT is a full replacement keyed by ID.
	for _, record := range records {
		ids, filter := extractIDsOrFilter(record, r)

		updateReq := connector.UpdateRequest{
			Table:  tableName,
			Record: record,
			Filter: filter,
			IDs:    ids,
		}

		sqlStr, args, err := conn.BuildUpdate(r.Context(), updateReq)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to build update: "+err.Error())
			return
		}

		if conn.SupportsReturning() {
			rows, err := db.QueryxContext(r.Context(), sqlStr, args...)
			if err != nil {
				code, msg := classifyDBError(err, "Update failed")
				writeError(w, code, msg)
				return
			}
			for rows.Next() {
				row := make(map[string]interface{})
				if err := rows.MapScan(row); err != nil {
					rows.Close()
					writeError(w, http.StatusInternalServerError, "Failed to scan result: "+err.Error())
					return
				}
				cleanMapValues(row)
				updated = append(updated, row)
			}
			rows.Close()
		} else {
			_, err := db.ExecContext(r.Context(), sqlStr, args...)
			if err != nil {
				code, msg := classifyDBError(err, "Update failed")
				writeError(w, code, msg)
				return
			}
			updated = append(updated, record)
		}
	}

	took := time.Since(start)
	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: updated,
		Meta: &model.ResponseMeta{
			Count:  len(updated),
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}

// UpdateRecords partially updates records matching a filter or ID list.
// PATCH /api/v1/{serviceName}/_table/{tableName}
func (h *TableHandler) UpdateRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Parse the request body for the fields to update.
	var body map[string]interface{}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Extract the filter from query params or body.
	filterStr := queryString(r, "filter")
	var filterSQL string
	var filterParams []interface{}
	if filterStr != "" {
		phFunc := func(index int) string {
			return conn.ParameterPlaceholder(index)
		}
		parsed, err := query.ParseFilter(filterStr, phFunc, 1)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid filter: "+err.Error())
			return
		}
		if parsed != nil {
			filterSQL = parsed.SQL
			filterParams = parsed.Params
		}
	}

	// Extract IDs from query params or body.
	var ids []interface{}
	if idsRaw, ok := body["ids"]; ok {
		if idSlice, ok := idsRaw.([]interface{}); ok {
			ids = idSlice
		}
		delete(body, "ids")
	}
	if idsStr := queryString(r, "ids"); idsStr != "" && len(ids) == 0 {
		for _, id := range strings.Split(idsStr, ",") {
			ids = append(ids, strings.TrimSpace(id))
		}
	}

	// If body has a "resource" wrapper, use the first record's fields.
	record := body
	if res, ok := body["resource"]; ok {
		if resSlice, ok := res.([]interface{}); ok && len(resSlice) > 0 {
			if first, ok := resSlice[0].(map[string]interface{}); ok {
				record = first
			}
		}
	}

	// Remove meta-keys that aren't actual column values.
	delete(record, "resource")
	delete(record, "ids")

	if len(record) == 0 {
		writeError(w, http.StatusBadRequest, "No fields to update")
		return
	}

	if filterSQL == "" && len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "Filter or IDs required for update")
		return
	}

	updateReq := connector.UpdateRequest{
		Table:  tableName,
		Record: record,
		Filter: filterSQL,
		IDs:    ids,
	}

	sqlStr, args, err := conn.BuildUpdate(r.Context(), updateReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build update: "+err.Error())
		return
	}

	// Merge args in SQL placeholder order: SET values, then filter params,
	// then ID params. BuildUpdate returns [set_values..., id_values...],
	// and filter params must be spliced between them.
	numSetArgs := len(record)
	allArgs := make([]interface{}, 0, len(args)+len(filterParams))
	allArgs = append(allArgs, args[:numSetArgs]...)
	allArgs = append(allArgs, filterParams...)
	allArgs = append(allArgs, args[numSetArgs:]...)

	db := conn.DB()
	updated := make([]map[string]interface{}, 0)

	if conn.SupportsReturning() {
		rows, err := db.QueryxContext(r.Context(), sqlStr, allArgs...)
		if err != nil {
			code, msg := classifyDBError(err, "Update failed")
			writeError(w, code, msg)
			return
		}
		defer rows.Close()

		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to scan result: "+err.Error())
				return
			}
			cleanMapValues(row)
			updated = append(updated, row)
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, "Row iteration error: "+err.Error())
			return
		}
	} else {
		result, err := db.ExecContext(r.Context(), sqlStr, allArgs...)
		if err != nil {
			code, msg := classifyDBError(err, "Update failed")
			writeError(w, code, msg)
			return
		}
		affected, _ := result.RowsAffected()
		// Without RETURNING we can't return the full rows.
		updated = append(updated, map[string]interface{}{"rows_affected": affected})
	}

	took := time.Since(start)
	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: updated,
		Meta: &model.ResponseMeta{
			Count:  len(updated),
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}

// DeleteRecords removes records matching a filter or ID list.
// DELETE /api/v1/{serviceName}/_table/{tableName}
func (h *TableHandler) DeleteRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Extract filter from query params.
	filterStr := queryString(r, "filter")
	var filterSQL string
	var filterParams []interface{}
	if filterStr != "" {
		phFunc := func(index int) string {
			return conn.ParameterPlaceholder(index)
		}
		parsed, err := query.ParseFilter(filterStr, phFunc, 1)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid filter: "+err.Error())
			return
		}
		if parsed != nil {
			filterSQL = parsed.SQL
			filterParams = parsed.Params
		}
	}

	// Extract IDs from query string or request body.
	var ids []interface{}
	if idsStr := queryString(r, "ids"); idsStr != "" {
		for _, id := range strings.Split(idsStr, ",") {
			ids = append(ids, strings.TrimSpace(id))
		}
	}

	// Also check request body for IDs.
	if len(ids) == 0 {
		var body struct {
			IDs      []interface{} `json:"ids"`
			Resource []struct {
				ID interface{} `json:"id"`
			} `json:"resource"`
		}
		// Body may be empty for DELETE; ignore decode errors.
		if err := readJSON(r, &body); err == nil {
			if len(body.IDs) > 0 {
				ids = body.IDs
			} else if len(body.Resource) > 0 {
				for _, res := range body.Resource {
					if res.ID != nil {
						ids = append(ids, res.ID)
					}
				}
			}
		}
	}

	if filterSQL == "" && len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "Filter or IDs required for delete")
		return
	}

	deleteReq := connector.DeleteRequest{
		Table:  tableName,
		Filter: filterSQL,
		IDs:    ids,
	}

	sqlStr, args, err := conn.BuildDelete(r.Context(), deleteReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build delete: "+err.Error())
		return
	}

	// Merge filter params with query args.
	allArgs := append(filterParams, args...)

	db := conn.DB()
	result, err := db.ExecContext(r.Context(), sqlStr, allArgs...)
	if err != nil {
		code, msg := classifyDBError(err, "Delete failed")
		writeError(w, code, msg)
		return
	}

	affected, _ := result.RowsAffected()
	took := time.Since(start)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"meta": model.ResponseMeta{
			Count:  int(affected),
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}

// --- internal helpers ---

// parseRecordsBody reads the request body and returns a slice of record maps.
// Accepts either a single JSON object or a {"resource": [...]} envelope.
func parseRecordsBody(r *http.Request) ([]map[string]interface{}, error) {
	var raw json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Try the {"resource": [...]} envelope first.
	var envelope struct {
		Resource []map[string]interface{} `json:"resource"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Resource) > 0 {
		return envelope.Resource, nil
	}

	// Try an array of objects.
	var records []map[string]interface{}
	if err := json.Unmarshal(raw, &records); err == nil && len(records) > 0 {
		return records, nil
	}

	// Try a single object.
	var single map[string]interface{}
	if err := json.Unmarshal(raw, &single); err == nil && len(single) > 0 {
		return []map[string]interface{}{single}, nil
	}

	return nil, fmt.Errorf("expected JSON object, array, or {\"resource\": [...]}")
}

// extractIDsOrFilter extracts primary key IDs from a record map (removing the
// "id" field) for use as a WHERE condition. If no ID is present, falls back to
// the filter query parameter. This is used by PUT (ReplaceRecords).
func extractIDsOrFilter(record map[string]interface{}, r *http.Request) ([]interface{}, string) {
	// Check for an "id" field in the record.
	if id, ok := record["id"]; ok {
		delete(record, "id")
		return []interface{}{id}, ""
	}

	// Fall back to query parameter filter.
	return nil, queryString(r, "filter")
}

// cleanMapValues converts []byte values from database scans into strings
// for clean JSON serialization. sqlx MapScan returns []byte for many column
// types which would otherwise be base64-encoded in JSON.
func cleanMapValues(m map[string]interface{}) {
	for k, v := range m {
		if b, ok := v.([]byte); ok {
			m[k] = string(b)
		}
	}
}
