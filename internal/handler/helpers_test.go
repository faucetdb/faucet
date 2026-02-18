package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// queryInt tests
// ---------------------------------------------------------------------------

func TestQueryInt(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		key        string
		defaultVal int
		want       int
	}{
		{"returns default for missing param", "/test", "limit", 25, 25},
		{"parses integer param", "/test?limit=100", "limit", 25, 100},
		{"returns default for non-integer", "/test?limit=abc", "limit", 25, 25},
		{"parses zero", "/test?offset=0", "offset", 10, 0},
		{"parses negative", "/test?offset=-5", "offset", 0, -5},
		{"returns default for empty value", "/test?limit=", "limit", 25, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.url, nil)
			got := queryInt(r, tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("queryInt(%q, %d) = %d, want %d", tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// queryBool tests
// ---------------------------------------------------------------------------

func TestQueryBool(t *testing.T) {
	tests := []struct {
		name string
		url  string
		key  string
		want bool
	}{
		{"true for 'true'", "/test?include_count=true", "include_count", true},
		{"true for '1'", "/test?include_count=1", "include_count", true},
		{"false for 'false'", "/test?include_count=false", "include_count", false},
		{"false for missing", "/test", "include_count", false},
		{"false for '0'", "/test?include_count=0", "include_count", false},
		{"false for empty", "/test?include_count=", "include_count", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.url, nil)
			got := queryBool(r, tt.key)
			if got != tt.want {
				t.Errorf("queryBool(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// queryString tests
// ---------------------------------------------------------------------------

func TestQueryString(t *testing.T) {
	tests := []struct {
		name string
		url  string
		key  string
		want string
	}{
		{"returns value", "/test?filter=age>21", "filter", "age>21"},
		{"returns empty for missing", "/test", "filter", ""},
		{"returns empty string for empty", "/test?filter=", "filter", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.url, nil)
			got := queryString(r, tt.key)
			if got != tt.want {
				t.Errorf("queryString(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// clampInt tests
// ---------------------------------------------------------------------------

func TestClampInt(t *testing.T) {
	tests := []struct {
		name       string
		val        int
		min        int
		max        int
		want       int
	}{
		{"within range", 50, 0, 100, 50},
		{"at min", 0, 0, 100, 0},
		{"at max", 100, 0, 100, 100},
		{"below min clamps to min", -5, 0, 100, 0},
		{"above max clamps to max", 500, 0, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampInt(tt.val, tt.min, tt.max)
			if got != tt.want {
				t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.val, tt.min, tt.max, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// stringsToResources tests
// ---------------------------------------------------------------------------

func TestStringsToResources(t *testing.T) {
	t.Run("converts strings to resource maps", func(t *testing.T) {
		result := stringsToResources("name", []string{"users", "orders", "products"})
		if len(result) != 3 {
			t.Fatalf("expected 3 resources, got %d", len(result))
		}
		for i, expected := range []string{"users", "orders", "products"} {
			if result[i]["name"] != expected {
				t.Errorf("resource[%d][name] = %v, want %s", i, result[i]["name"], expected)
			}
		}
	})

	t.Run("empty input", func(t *testing.T) {
		result := stringsToResources("name", nil)
		if len(result) != 0 {
			t.Errorf("expected 0 resources, got %d", len(result))
		}
	})
}

// ---------------------------------------------------------------------------
// cleanMapValues tests
// ---------------------------------------------------------------------------

func TestCleanMapValues(t *testing.T) {
	t.Run("converts byte slices to strings", func(t *testing.T) {
		m := map[string]interface{}{
			"name":  []byte("Alice"),
			"email": []byte("alice@example.com"),
			"id":    42,
		}
		cleanMapValues(m)

		if m["name"] != "Alice" {
			t.Errorf("expected name 'Alice', got %v", m["name"])
		}
		if m["email"] != "alice@example.com" {
			t.Errorf("expected email 'alice@example.com', got %v", m["email"])
		}
		if m["id"] != 42 {
			t.Errorf("expected id 42, got %v", m["id"])
		}
	})

	t.Run("no-op for non-byte values", func(t *testing.T) {
		m := map[string]interface{}{
			"count":  100,
			"active": true,
			"name":   "Bob",
		}
		cleanMapValues(m)
		if m["count"] != 100 || m["active"] != true || m["name"] != "Bob" {
			t.Error("non-byte values should be unchanged")
		}
	})
}

// ---------------------------------------------------------------------------
// writeError tests
// ---------------------------------------------------------------------------

func TestWriteError(t *testing.T) {
	t.Run("writes JSON error response", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeError(w, http.StatusBadRequest, "Invalid input")

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, `"code":400`) {
			t.Errorf("expected code 400 in body: %s", body)
		}
		if !strings.Contains(body, `"message":"Invalid input"`) {
			t.Errorf("expected message in body: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// writeJSON tests
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Run("writes JSON with correct content type", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeJSON(w, http.StatusOK, map[string]string{"hello": "world"})

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, `"hello":"world"`) {
			t.Errorf("expected JSON body, got: %s", body)
		}
	})
}

// ---------------------------------------------------------------------------
// jsonSchemaType tests
// ---------------------------------------------------------------------------

func TestJsonSchemaType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"integer", "integer"},
		{"number", "number"},
		{"boolean", "boolean"},
		{"string", "string"},
		{"string(date-time)", "string"},
		{"string(date)", "string"},
		{"string(byte)", "string"},
		{"object", "object"},
		{"array", "array"},
		{"unknown", "string"},
		{"", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := jsonSchemaType(tt.input)
			if got != tt.want {
				t.Errorf("jsonSchemaType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseRecordsBody tests
// ---------------------------------------------------------------------------

func TestParseRecordsBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		wantErr bool
	}{
		{
			name: "single object",
			body: `{"name": "Alice", "email": "alice@example.com"}`,
			want: 1,
		},
		{
			name: "resource envelope",
			body: `{"resource": [{"name": "Alice"}, {"name": "Bob"}]}`,
			want: 2,
		},
		{
			name: "array of objects",
			body: `[{"name": "Alice"}, {"name": "Bob"}, {"name": "Charlie"}]`,
			want: 3,
		},
		{
			name:    "invalid JSON",
			body:    `{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty object",
			body:    `{}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/test", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")

			records, err := parseRecordsBody(r)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(records) != tt.want {
				t.Errorf("expected %d records, got %d", tt.want, len(records))
			}
		})
	}
}
