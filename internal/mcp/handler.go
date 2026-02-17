package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// --------------------------------------------------------------------------
// Parameter extraction helpers
// --------------------------------------------------------------------------

// requireString extracts a required string argument from the tool request.
func requireString(request mcp.CallToolRequest, key string) (string, error) {
	val, err := request.RequireString(key)
	if err != nil {
		return "", fmt.Errorf("missing required parameter %q", key)
	}
	return val, nil
}

// optionalString extracts an optional string argument from the tool request.
func optionalString(request mcp.CallToolRequest, key string) string {
	return request.GetString(key, "")
}

// optionalInt extracts an optional integer argument from the tool request.
func optionalInt(request mcp.CallToolRequest, key string, defaultVal int) int {
	return request.GetInt(key, defaultVal)
}

// optionalStringSlice extracts an optional string slice argument from the tool request.
func optionalStringSlice(request mcp.CallToolRequest, key string) []string {
	return request.GetStringSlice(key, nil)
}

// getObjectArg extracts a map[string]interface{} argument from the tool request.
// Returns nil if the key is not present or not a map.
func getObjectArg(request mcp.CallToolRequest, key string) map[string]interface{} {
	args := request.GetArguments()
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

// getObjectSliceArg extracts a []map[string]interface{} argument from the tool request.
// Returns nil if the key is not present or not the expected type.
func getObjectSliceArg(request mcp.CallToolRequest, key string) []map[string]interface{} {
	args := request.GetArguments()
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(slice))
	for _, item := range slice {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil
		}
		result = append(result, m)
	}
	return result
}

// getAnySliceArg extracts a []interface{} argument from the tool request.
// Returns nil if the key is not present.
func getAnySliceArg(request mcp.CallToolRequest, key string) []interface{} {
	args := request.GetArguments()
	if args == nil {
		return nil
	}
	raw, ok := args[key]
	if !ok {
		return nil
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	return slice
}

// --------------------------------------------------------------------------
// Response builders
// --------------------------------------------------------------------------

// successJSON marshals data to JSON and returns it as a tool result.
func successJSON(data interface{}) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}
	return mcp.NewToolResultText(string(b)), nil
}

// toolError returns a tool-level error result. Errors returned this way are
// visible to the LLM so it can self-correct; they do NOT terminate the MCP
// session.
func toolError(format string, args ...interface{}) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}

// clamp constrains val to [min, max].
func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
