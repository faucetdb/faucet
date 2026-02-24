package model

// ListResponse is the standard envelope for list endpoints, wrapping results
// in a "resource" array with optional pagination metadata.
type ListResponse struct {
	Resource []map[string]interface{} `json:"resource"`
	Meta     *ResponseMeta            `json:"meta,omitempty"`
}

// ResponseMeta contains pagination and timing information for list responses.
type ResponseMeta struct {
	Count      int     `json:"count"`
	Total      *int64  `json:"total,omitempty"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	NextCursor string  `json:"next_cursor,omitempty"`
	TookMs     float64 `json:"took_ms"`
}

// BatchResponse is the envelope for batch operations that may have mixed results.
// Used when ?continue=true produces partial successes.
type BatchResponse struct {
	Resource []interface{}      `json:"resource"`
	Meta     *BatchResponseMeta `json:"meta,omitempty"`
}

// BatchResponseMeta extends ResponseMeta with batch operation tracking fields.
type BatchResponseMeta struct {
	Count     int     `json:"count"`
	Succeeded int     `json:"succeeded"`
	Failed    int     `json:"failed"`
	Errors    []int   `json:"errors,omitempty"` // indices of failed operations
	TookMs    float64 `json:"took_ms"`
}

// ErrorResponse is the standard envelope for error responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the structured error information returned by the API.
type ErrorDetail struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context,omitempty"`
}
