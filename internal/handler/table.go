package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

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
	idsStr := queryString(r, "ids")
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

	// Apply IDs filter if provided (filters by primary key "id" column).
	if idsStr != "" {
		idParts := strings.Split(idsStr, ",")
		placeholders := make([]string, len(idParts))
		for i, id := range idParts {
			placeholders[i] = conn.ParameterPlaceholder(len(filterParams) + i + 1)
			filterParams = append(filterParams, strings.TrimSpace(id))
		}
		idClause := fmt.Sprintf("%s IN (%s)", conn.QuoteIdentifier("id"), strings.Join(placeholders, ", "))
		if filterSQL != "" {
			filterSQL = filterSQL + " AND " + idClause
		} else {
			filterSQL = idClause
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
//
// Batch modes (query parameters):
//   - (default)         halt on first error; prior inserts are committed
//   - ?rollback=true    wrap in transaction; all-or-nothing
//   - ?continue=true    insert each record independently; report mixed results
func (h *TableHandler) CreateRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	records, err := parseRecordsBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "No records provided")
		return
	}

	mode := parseBatchMode(r)

	// Continue mode: insert each record individually, collecting per-record results.
	if mode == BatchModeContinue {
		h.createRecordsContinue(w, r, conn, tableName, records, start)
		return
	}

	// Build a single multi-row INSERT for halt and rollback modes.
	insertReq := connector.InsertRequest{
		Table:   tableName,
		Records: records,
	}
	sqlStr, args, err := conn.BuildInsert(r.Context(), insertReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to build insert: "+err.Error())
		return
	}

	// Choose executor: transaction for rollback mode, raw DB otherwise.
	var exec connector.QueryExecutor
	exec = conn.DB()
	if mode == BatchModeRollback {
		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to begin transaction: "+err.Error())
			return
		}
		defer tx.Rollback()
		exec = tx

		// Execute + commit in rollback mode.
		created, err := execInsert(r.Context(), exec, conn, sqlStr, args)
		if err != nil {
			code, msg := classifyDBError(err, "Insert failed")
			writeError(w, code, msg)
			return
		}
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to commit transaction: "+err.Error())
			return
		}
		took := time.Since(start)
		writeCreateResponse(w, conn, created, records, took)
		return
	}

	// Halt mode (default): execute directly.
	created, err := execInsert(r.Context(), exec, conn, sqlStr, args)
	if err != nil {
		code, msg := classifyDBError(err, "Insert failed")
		writeError(w, code, msg)
		return
	}
	took := time.Since(start)
	writeCreateResponse(w, conn, created, records, took)
}

// createRecordsContinue inserts each record individually, collecting successes and errors.
func (h *TableHandler) createRecordsContinue(w http.ResponseWriter, r *http.Request, conn connector.Connector, tableName string, records []map[string]interface{}, start time.Time) {
	db := conn.DB()
	results := make([]interface{}, len(records))
	var errIndices []int
	succeeded := 0

	for i, rec := range records {
		singleReq := connector.InsertRequest{
			Table:   tableName,
			Records: []map[string]interface{}{rec},
		}
		sqlStr, args, err := conn.BuildInsert(r.Context(), singleReq)
		if err != nil {
			results[i] = map[string]interface{}{"error": model.ErrorDetail{Code: 500, Message: "Failed to build insert: " + err.Error()}}
			errIndices = append(errIndices, i)
			continue
		}

		if conn.SupportsReturning() {
			rows, err := db.QueryxContext(r.Context(), sqlStr, args...)
			if err != nil {
				code, msg := classifyDBError(err, "Insert failed")
				results[i] = map[string]interface{}{"error": model.ErrorDetail{Code: code, Message: msg}}
				errIndices = append(errIndices, i)
				continue
			}
			row := make(map[string]interface{})
			if rows.Next() {
				if err := rows.MapScan(row); err != nil {
					rows.Close()
					results[i] = map[string]interface{}{"error": model.ErrorDetail{Code: 500, Message: "Failed to scan result: " + err.Error()}}
					errIndices = append(errIndices, i)
					continue
				}
				cleanMapValues(row)
			}
			rows.Close()
			results[i] = row
		} else {
			_, err := db.ExecContext(r.Context(), sqlStr, args...)
			if err != nil {
				code, msg := classifyDBError(err, "Insert failed")
				results[i] = map[string]interface{}{"error": model.ErrorDetail{Code: code, Message: msg}}
				errIndices = append(errIndices, i)
				continue
			}
			results[i] = rec
		}
		succeeded++
	}

	took := time.Since(start)
	status := http.StatusCreated
	if len(errIndices) > 0 {
		status = http.StatusOK
	}
	writeJSON(w, status, model.BatchResponse{
		Resource: results,
		Meta: &model.BatchResponseMeta{
			Count:     len(records),
			Succeeded: succeeded,
			Failed:    len(errIndices),
			Errors:    errIndices,
			TookMs:    float64(took.Microseconds()) / 1000.0,
		},
	})
}

// execInsert executes an INSERT statement and returns created records (if RETURNING is supported)
// or nil (caller uses input records as fallback).
func execInsert(ctx context.Context, exec connector.QueryExecutor, conn connector.Connector, sqlStr string, args []interface{}) ([]map[string]interface{}, error) {
	if conn.SupportsReturning() {
		rows, err := exec.QueryxContext(ctx, sqlStr, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var created []map[string]interface{}
		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err != nil {
				return nil, err
			}
			cleanMapValues(row)
			created = append(created, row)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return created, nil
	}
	_, err := exec.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return nil, nil // caller uses input records
}

// writeCreateResponse writes the standard POST response.
func writeCreateResponse(w http.ResponseWriter, conn connector.Connector, created []map[string]interface{}, inputRecords []map[string]interface{}, took time.Duration) {
	if created != nil {
		writeJSON(w, http.StatusCreated, model.ListResponse{
			Resource: created,
			Meta: &model.ResponseMeta{
				Count:  len(created),
				TookMs: float64(took.Microseconds()) / 1000.0,
			},
		})
		return
	}
	writeJSON(w, http.StatusCreated, model.ListResponse{
		Resource: inputRecords,
		Meta: &model.ResponseMeta{
			Count:  len(inputRecords),
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}

// ReplaceRecords performs a full record replacement (PUT) on a table.
// PUT /api/v1/{serviceName}/_table/{tableName}
//
// Batch modes (query parameters):
//   - (default)         halt on first error; prior updates are committed
//   - ?rollback=true    wrap in transaction; all-or-nothing
//   - ?continue=true    update each record independently; report mixed results
func (h *TableHandler) ReplaceRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	records, err := parseRecordsBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if len(records) == 0 {
		writeError(w, http.StatusBadRequest, "No records provided")
		return
	}

	mode := parseBatchMode(r)

	// Choose executor: transaction for rollback mode, raw DB otherwise.
	var exec connector.QueryExecutor
	var tx *sqlx.Tx
	exec = conn.DB()

	if mode == BatchModeRollback {
		tx, err = conn.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to begin transaction: "+err.Error())
			return
		}
		defer tx.Rollback()
		exec = tx
	}

	// For continue mode, collect per-record results.
	if mode == BatchModeContinue {
		results := make([]interface{}, len(records))
		var errIndices []int
		succeeded := 0

		for i, record := range records {
			ids, filter := extractIDsOrFilter(record, r)
			result, err := execSingleUpdate(r.Context(), exec, conn, tableName, record, filter, ids)
			if err != nil {
				code, msg := classifyDBError(err, "Update failed")
				results[i] = map[string]interface{}{"error": model.ErrorDetail{Code: code, Message: msg}}
				errIndices = append(errIndices, i)
				continue
			}
			results[i] = result
			succeeded++
		}

		took := time.Since(start)
		writeJSON(w, http.StatusOK, model.BatchResponse{
			Resource: results,
			Meta: &model.BatchResponseMeta{
				Count:     len(records),
				Succeeded: succeeded,
				Failed:    len(errIndices),
				Errors:    errIndices,
				TookMs:    float64(took.Microseconds()) / 1000.0,
			},
		})
		return
	}

	// Halt and rollback modes: stop at first error.
	updated := make([]map[string]interface{}, 0)
	for _, record := range records {
		ids, filter := extractIDsOrFilter(record, r)
		result, err := execSingleUpdate(r.Context(), exec, conn, tableName, record, filter, ids)
		if err != nil {
			code, msg := classifyDBError(err, "Update failed")
			writeError(w, code, msg)
			return
		}
		updated = append(updated, result)
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to commit transaction: "+err.Error())
			return
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

// execSingleUpdate builds and executes a single UPDATE for one record, returning the result row.
func execSingleUpdate(ctx context.Context, exec connector.QueryExecutor, conn connector.Connector, tableName string, record map[string]interface{}, filter string, ids []interface{}) (map[string]interface{}, error) {
	updateReq := connector.UpdateRequest{
		Table:  tableName,
		Record: record,
		Filter: filter,
		IDs:    ids,
	}
	sqlStr, args, err := conn.BuildUpdate(ctx, updateReq)
	if err != nil {
		return nil, err
	}

	if conn.SupportsReturning() {
		rows, err := exec.QueryxContext(ctx, sqlStr, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		row := make(map[string]interface{})
		if rows.Next() {
			if err := rows.MapScan(row); err != nil {
				return nil, err
			}
			cleanMapValues(row)
		}
		return row, rows.Err()
	}

	_, err = exec.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// UpdateRecords partially updates records matching a filter or ID list.
// PATCH /api/v1/{serviceName}/_table/{tableName}
//
// Batch modes: ?rollback=true wraps the UPDATE in a transaction.
func (h *TableHandler) UpdateRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	var body map[string]interface{}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

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

	record := body
	if res, ok := body["resource"]; ok {
		if resSlice, ok := res.([]interface{}); ok && len(resSlice) > 0 {
			if first, ok := resSlice[0].(map[string]interface{}); ok {
				record = first
			}
		}
	}

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

	numSetArgs := len(record)
	allArgs := make([]interface{}, 0, len(args)+len(filterParams))
	allArgs = append(allArgs, args[:numSetArgs]...)
	allArgs = append(allArgs, filterParams...)
	allArgs = append(allArgs, args[numSetArgs:]...)

	// Choose executor: transaction for rollback mode, raw DB otherwise.
	mode := parseBatchMode(r)
	var exec connector.QueryExecutor
	var tx *sqlx.Tx
	exec = conn.DB()

	if mode == BatchModeRollback {
		tx, err = conn.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to begin transaction: "+err.Error())
			return
		}
		defer tx.Rollback()
		exec = tx
	}

	updated := make([]map[string]interface{}, 0)

	if conn.SupportsReturning() {
		rows, err := exec.QueryxContext(r.Context(), sqlStr, allArgs...)
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
		result, err := exec.ExecContext(r.Context(), sqlStr, allArgs...)
		if err != nil {
			code, msg := classifyDBError(err, "Update failed")
			writeError(w, code, msg)
			return
		}
		affected, _ := result.RowsAffected()
		updated = append(updated, map[string]interface{}{"rows_affected": affected})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to commit transaction: "+err.Error())
			return
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

// DeleteRecords removes records matching a filter or ID list.
// DELETE /api/v1/{serviceName}/_table/{tableName}
//
// Batch modes: ?rollback=true wraps the DELETE in a transaction.
func (h *TableHandler) DeleteRecords(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

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

	var ids []interface{}
	if idsStr := queryString(r, "ids"); idsStr != "" {
		for _, id := range strings.Split(idsStr, ",") {
			ids = append(ids, strings.TrimSpace(id))
		}
	}

	if len(ids) == 0 {
		var body struct {
			IDs      []interface{} `json:"ids"`
			Resource []struct {
				ID interface{} `json:"id"`
			} `json:"resource"`
		}
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

	allArgs := append(filterParams, args...)

	// Choose executor: transaction for rollback mode, raw DB otherwise.
	mode := parseBatchMode(r)
	var exec connector.QueryExecutor
	var tx *sqlx.Tx
	exec = conn.DB()

	if mode == BatchModeRollback {
		tx, err = conn.BeginTx(r.Context(), nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to begin transaction: "+err.Error())
			return
		}
		defer tx.Rollback()
		exec = tx
	}

	result, err := exec.ExecContext(r.Context(), sqlStr, allArgs...)
	if err != nil {
		code, msg := classifyDBError(err, "Delete failed")
		writeError(w, code, msg)
		return
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to commit transaction: "+err.Error())
			return
		}
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
