package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/contract"
)

// ContractHandler manages schema contract locking operations.
type ContractHandler struct {
	registry *connector.Registry
	store    *config.Store
}

// NewContractHandler creates a new ContractHandler.
func NewContractHandler(registry *connector.Registry, store *config.Store) *ContractHandler {
	return &ContractHandler{
		registry: registry,
		store:    store,
	}
}

// LockTable creates a schema contract by snapshotting the current live schema.
// POST /api/v1/system/contract/{serviceName}/{tableName}
func (h *ContractHandler) LockTable(w http.ResponseWriter, r *http.Request) {
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

	c, err := h.store.SaveContract(r.Context(), serviceName, tableName, *table)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to lock table: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

// LockService creates schema contracts for ALL tables in a service.
// POST /api/v1/system/contract/{serviceName}
func (h *ContractHandler) LockService(w http.ResponseWriter, r *http.Request) {
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

	var locked []contract.Contract
	for _, table := range schema.Tables {
		c, err := h.store.SaveContract(r.Context(), serviceName, table.Name, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to lock table "+table.Name+": "+err.Error())
			return
		}
		locked = append(locked, *c)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"service": serviceName,
		"locked":  len(locked),
		"tables":  locked,
	})
}

// UnlockTable removes a schema contract for a single table.
// DELETE /api/v1/system/contract/{serviceName}/{tableName}
func (h *ContractHandler) UnlockTable(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	if err := h.store.DeleteContract(r.Context(), serviceName, tableName); err != nil {
		if err == config.ErrNotFound {
			writeError(w, http.StatusNotFound, "No contract found for "+serviceName+"/"+tableName)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to unlock table: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Contract removed for " + serviceName + "/" + tableName,
	})
}

// UnlockService removes ALL schema contracts for a service.
// DELETE /api/v1/system/contract/{serviceName}
func (h *ContractHandler) UnlockService(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	n, err := h.store.DeleteServiceContracts(r.Context(), serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to unlock service: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"removed": n,
	})
}

// ListContracts returns all schema contracts for a service.
// GET /api/v1/system/contract/{serviceName}
func (h *ContractHandler) ListContracts(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	contracts, err := h.store.ListContracts(r.Context(), serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list contracts: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"service":   serviceName,
		"contracts": contracts,
	})
}

// GetContract returns a single contract with optional drift check.
// GET /api/v1/system/contract/{serviceName}/{tableName}
func (h *ContractHandler) GetContract(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	c, err := h.store.GetContract(r.Context(), serviceName, tableName)
	if err != nil {
		if err == config.ErrNotFound {
			writeError(w, http.StatusNotFound, "No contract found for "+serviceName+"/"+tableName)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get contract: "+err.Error())
		return
	}

	// If ?check=true, also compare against live schema.
	if queryBool(r, "check") {
		conn, err := h.registry.Get(serviceName)
		if err != nil {
			writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
			return
		}
		live, err := conn.IntrospectTable(r.Context(), tableName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to introspect live schema: "+err.Error())
			return
		}
		drift := contract.DiffTable(serviceName, c.Schema, *live, c.LockedAt)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"contract": c,
			"drift":    drift,
		})
		return
	}

	writeJSON(w, http.StatusOK, c)
}

// DiffService compares all locked contracts against the live schema.
// GET /api/v1/system/contract/{serviceName}/diff
func (h *ContractHandler) DiffService(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	contracts, err := h.store.ListContracts(r.Context(), serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list contracts: "+err.Error())
		return
	}

	if len(contracts) == 0 {
		writeJSON(w, http.StatusOK, contract.ServiceDriftReport{
			ServiceName: serviceName,
		})
		return
	}

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

	svc, _ := h.store.GetServiceByName(r.Context(), serviceName)
	lockMode := contract.LockModeNone
	if svc != nil {
		lockMode = contract.LockMode(svc.SchemaLock)
	}

	report := contract.DiffSchema(serviceName, contracts, schema, lockMode)
	writeJSON(w, http.StatusOK, report)
}

// PromoteTable updates a contract to match the current live schema.
// POST /api/v1/system/contract/{serviceName}/{tableName}/promote
func (h *ContractHandler) PromoteTable(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	tableName := chi.URLParam(r, "tableName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	live, err := conn.IntrospectTable(r.Context(), tableName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Table not found: "+err.Error())
		return
	}

	if err := h.store.PromoteContract(r.Context(), serviceName, tableName, *live); err != nil {
		if err == config.ErrNotFound {
			writeError(w, http.StatusNotFound, "No contract found for "+serviceName+"/"+tableName)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to promote contract: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    "Contract promoted to current schema",
		"promoted_at": time.Now().UTC(),
	})
}

// SetLockMode updates the schema_lock mode for a service.
// PUT /api/v1/system/contract/{serviceName}/mode
func (h *ContractHandler) SetLockMode(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	var body struct {
		Mode string `json:"mode"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if !contract.ValidLockMode(body.Mode) {
		writeError(w, http.StatusBadRequest, "Invalid lock mode: "+body.Mode+". Must be none, auto, or strict.")
		return
	}

	svc, err := h.store.GetServiceByName(r.Context(), serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	svc.SchemaLock = body.Mode
	if err := h.store.UpdateService(r.Context(), svc); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update service: "+err.Error())
		return
	}

	// If switching to auto or strict and no contracts exist, auto-lock all tables.
	if body.Mode != "none" {
		contracts, _ := h.store.ListContracts(r.Context(), serviceName)
		if len(contracts) == 0 {
			conn, err := h.registry.Get(serviceName)
			if err == nil {
				schema, err := conn.IntrospectSchema(r.Context())
				if err == nil {
					for _, table := range schema.Tables {
						_, _ = h.store.SaveContract(r.Context(), serviceName, table.Name, table)
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"service": serviceName,
		"mode":    body.Mode,
	})
}
