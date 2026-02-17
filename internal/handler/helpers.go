package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/faucetdb/faucet/internal/model"
)

// writeJSON serializes v as JSON and writes it to the response with the given
// HTTP status code. The Content-Type header is set to application/json.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a structured error response using the standard error
// envelope. The optional ctx map provides additional context fields.
func writeError(w http.ResponseWriter, code int, message string, ctx ...map[string]interface{}) {
	var ctxMap map[string]interface{}
	if len(ctx) > 0 {
		ctxMap = ctx[0]
	}
	writeJSON(w, code, model.ErrorResponse{
		Error: model.ErrorDetail{
			Code:    code,
			Message: message,
			Context: ctxMap,
		},
	})
}

// readJSON decodes the request body as JSON into v. The body is closed after
// decoding regardless of success or failure.
func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// queryInt extracts an integer query parameter, returning defaultVal if the
// parameter is missing or cannot be parsed.
func queryInt(r *http.Request, key string, defaultVal int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

// queryString extracts a string query parameter.
func queryString(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

// queryBool extracts a boolean query parameter. Returns false if the parameter
// is missing or not "true"/"1".
func queryBool(r *http.Request, key string) bool {
	val := r.URL.Query().Get(key)
	return val == "true" || val == "1"
}

// stringsToResources converts a list of strings into the DreamFactory-style
// resource array format: [{"key": "value1"}, {"key": "value2"}, ...].
func stringsToResources(key string, values []string) []map[string]interface{} {
	out := make([]map[string]interface{}, len(values))
	for i, v := range values {
		out[i] = map[string]interface{}{key: v}
	}
	return out
}

// clampInt constrains val to be within [min, max].
func clampInt(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
