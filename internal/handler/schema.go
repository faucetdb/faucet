package handler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/contract"
	"github.com/faucetdb/faucet/internal/model"
)

// SchemaHandler handles schema introspection and DDL operations.
type SchemaHandler struct {
	registry *connector.Registry
	store    *config.Store
}

// NewSchemaHandler creates a new SchemaHandler.
func NewSchemaHandler(registry *connector.Registry, store *config.Store) *SchemaHandler {
	return &SchemaHandler{
		registry: registry,
		store:    store,
	}
}

// ListTables returns the full schema for a service's database, including all
// tables, views, stored procedures, and functions.
//
// When schema locking is enabled (auto or strict), the response includes a
// "contract" field with drift status for each locked table.
// GET /api/v1/{serviceName}/_schema
func (h *SchemaHandler) ListTables(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	schema, err := conn.IntrospectSchema(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to introspect schema: "+err.Error())
		return
	}

	// Auto-lock: if service is in auto or strict mode and no contracts exist yet,
	// create them on first introspection.
	svc, _ := h.store.GetServiceByName(r.Context(), serviceName)
	if svc != nil && (svc.SchemaLock == "auto" || svc.SchemaLock == "strict") {
		contracts, _ := h.store.ListContracts(r.Context(), serviceName)
		if len(contracts) == 0 && len(schema.Tables) > 0 {
			for _, table := range schema.Tables {
				_, _ = h.store.SaveContract(r.Context(), serviceName, table.Name, table)
			}
		}
	}

	writeJSON(w, http.StatusOK, schema)
}

// GetTableSchema returns the detailed schema for a single table or view.
// GET /api/v1/{serviceName}/_schema/{tableName}
func (h *SchemaHandler) GetTableSchema(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	table, err := conn.IntrospectTable(r.Context(), tableName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Table not found: "+err.Error())
		return
	}

	// If contract exists, include drift info in headers.
	c, err := h.store.GetContract(r.Context(), serviceName, tableName)
	if err == nil {
		drift := contract.DiffTable(serviceName, c.Schema, *table, c.LockedAt)
		if drift.HasBreaking {
			w.Header().Set("X-Schema-Drift", "breaking")
		} else if drift.HasDrift {
			w.Header().Set("X-Schema-Drift", "additive")
		} else {
			w.Header().Set("X-Schema-Drift", "none")
		}
	}

	writeJSON(w, http.StatusOK, table)
}

// CreateTable creates a new table in the service's database from a table
// schema definition provided in the request body.
// POST /api/v1/{serviceName}/_schema
func (h *SchemaHandler) CreateTable(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Check if service is read-only.
	svc, err := h.store.GetServiceByName(r.Context(), serviceName)
	if err == nil && svc.ReadOnly {
		writeError(w, http.StatusForbidden, "Service is read-only")
		return
	}

	var def model.TableSchema
	if err := readJSON(r, &def); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if def.Name == "" {
		writeError(w, http.StatusBadRequest, "Table name is required")
		return
	}

	if len(def.Columns) == 0 {
		writeError(w, http.StatusBadRequest, "At least one column is required")
		return
	}

	if err := conn.CreateTable(r.Context(), def); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create table: "+err.Error())
		return
	}

	// Return the schema of the newly created table.
	created, err := conn.IntrospectTable(r.Context(), def.Name)
	if err != nil {
		writeJSON(w, http.StatusCreated, def)
		return
	}

	// Auto-lock new tables when in auto or strict mode.
	if svc != nil && (svc.SchemaLock == "auto" || svc.SchemaLock == "strict") {
		_, _ = h.store.SaveContract(r.Context(), serviceName, created.Name, *created)
	}

	writeJSON(w, http.StatusCreated, created)
}

// AlterTable modifies an existing table's schema. The request body should
// contain an array of schema changes (add_column, drop_column, rename_column,
// modify_column).
//
// When schema locking is in "strict" mode, AlterTable is blocked entirely.
// When in "auto" mode, only breaking changes (drop_column, rename_column,
// modify_column) are blocked; additive changes (add_column) pass through.
// PUT /api/v1/{serviceName}/_schema/{tableName}
func (h *SchemaHandler) AlterTable(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Check if service is read-only.
	svc, err := h.store.GetServiceByName(r.Context(), serviceName)
	if err == nil && svc.ReadOnly {
		writeError(w, http.StatusForbidden, "Service is read-only")
		return
	}

	// Accept the request body as a schema change list.
	var changes []connector.SchemaChange

	var envelope struct {
		Changes []connector.SchemaChange `json:"changes"`
	}
	if err := readJSON(r, &envelope); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if len(envelope.Changes) > 0 {
		changes = envelope.Changes
	}

	if len(changes) == 0 {
		writeError(w, http.StatusBadRequest, "No schema changes provided")
		return
	}

	// Validate each change.
	for i, ch := range changes {
		switch ch.Type {
		case "add_column", "drop_column", "rename_column", "modify_column":
			// valid
		default:
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid change type at index %d: %s", i, ch.Type))
			return
		}
		if ch.Column == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Column name required for change at index %d", i))
			return
		}
	}

	// Contract locking enforcement.
	if svc != nil && svc.SchemaLock != "" && svc.SchemaLock != "none" {
		_, contractErr := h.store.GetContract(r.Context(), serviceName, tableName)
		if contractErr == nil {
			if svc.SchemaLock == "strict" {
				writeError(w, http.StatusConflict,
					"Schema is locked in strict mode. Run 'faucet db promote' or change mode to apply changes.")
				return
			}
			// Auto mode: block breaking changes.
			for _, ch := range changes {
				if ch.Type == "drop_column" || ch.Type == "rename_column" || ch.Type == "modify_column" {
					writeError(w, http.StatusConflict, fmt.Sprintf(
						"Breaking schema change blocked: %s on column %q. Schema is locked in auto mode. "+
							"Run 'faucet db promote %s --table %s' to accept, or unlock the table first.",
						ch.Type, ch.Column, serviceName, tableName))
					return
				}
			}
		}
	}

	if err := conn.AlterTable(r.Context(), tableName, changes); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to alter table: "+err.Error())
		return
	}

	// Return the updated table schema.
	updated, err := conn.IntrospectTable(r.Context(), tableName)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "Table altered successfully",
		})
		return
	}

	// In auto mode, auto-promote additive changes to keep the contract in sync.
	if svc != nil && svc.SchemaLock == "auto" {
		_ = h.store.PromoteContract(r.Context(), serviceName, tableName, *updated)
	}

	writeJSON(w, http.StatusOK, updated)
}

// DropTable removes a table from the service's database.
//
// When schema locking is enabled, dropping a locked table is blocked.
// DELETE /api/v1/{serviceName}/_schema/{tableName}
func (h *SchemaHandler) DropTable(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Check if service is read-only.
	svc, err := h.store.GetServiceByName(r.Context(), serviceName)
	if err == nil && svc.ReadOnly {
		writeError(w, http.StatusForbidden, "Service is read-only")
		return
	}

	// Block dropping locked tables.
	if svc != nil && svc.SchemaLock != "" && svc.SchemaLock != "none" {
		_, contractErr := h.store.GetContract(r.Context(), serviceName, tableName)
		if contractErr == nil {
			writeError(w, http.StatusConflict, fmt.Sprintf(
				"Cannot drop locked table %q. Run 'faucet db unlock %s --table %s' first.",
				tableName, serviceName, tableName))
			return
		}
	}

	if err := conn.DropTable(r.Context(), tableName); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to drop table: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Table '" + tableName + "' dropped successfully",
	})
}
