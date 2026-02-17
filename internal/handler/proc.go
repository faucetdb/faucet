package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
)

// ProcHandler handles stored procedure listing and execution.
type ProcHandler struct {
	registry *connector.Registry
}

// NewProcHandler creates a new ProcHandler.
func NewProcHandler(registry *connector.Registry) *ProcHandler {
	return &ProcHandler{
		registry: registry,
	}
}

// ListProcedures returns all stored procedures and functions for a service.
// GET /api/v2/{serviceName}/_proc
func (h *ProcHandler) ListProcedures(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")
	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	procs, err := conn.GetStoredProcedures(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list procedures: "+err.Error())
		return
	}

	// Convert to resource format.
	resources := make([]map[string]interface{}, 0, len(procs))
	for _, p := range procs {
		resources = append(resources, map[string]interface{}{
			"name":        p.Name,
			"type":        p.Type,
			"return_type": p.ReturnType,
			"parameters":  p.Parameters,
		})
	}

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: resources,
		Meta: &model.ResponseMeta{
			Count: len(resources),
		},
	})
}

// CallProcedure executes a stored procedure with the provided parameters and
// returns the result set.
// POST /api/v2/{serviceName}/_proc/{procName}
func (h *ProcHandler) CallProcedure(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	serviceName := chi.URLParam(r, "serviceName")
	procName := chi.URLParam(r, "procName")

	conn, err := h.registry.Get(serviceName)
	if err != nil {
		writeError(w, http.StatusNotFound, "Service not found: "+serviceName)
		return
	}

	// Parse parameters from the request body. An empty body is acceptable
	// for procedures with no parameters.
	var params map[string]interface{}
	if r.Body != nil && r.ContentLength != 0 {
		if err := readJSON(r, &params); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
			return
		}
	}
	if params == nil {
		params = make(map[string]interface{})
	}

	// Also merge query parameters into the params map so that simple
	// procedure calls can be made via GET-style query strings.
	for key, values := range r.URL.Query() {
		if _, exists := params[key]; !exists && len(values) > 0 {
			params[key] = values[0]
		}
	}

	results, err := conn.CallProcedure(r.Context(), procName, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Procedure call failed: "+err.Error())
		return
	}

	// Clean up []byte values for JSON serialization.
	for _, row := range results {
		cleanMapValues(row)
	}

	took := time.Since(start)

	writeJSON(w, http.StatusOK, model.ListResponse{
		Resource: results,
		Meta: &model.ResponseMeta{
			Count:  len(results),
			TookMs: float64(took.Microseconds()) / 1000.0,
		},
	})
}
