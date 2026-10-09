package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The admin UI and the MCP endpoint share /mcp. Browsers must get the page;
// MCP clients must reach the MCP handler (which requires auth).
func TestMCPPathSharedWithUI(t *testing.T) {
	env := newTestEnv(t)

	get := func(accept string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		rr := httptest.NewRecorder()
		env.server.Router().ServeHTTP(rr, req)
		return rr
	}

	browser := get("text/html,application/xhtml+xml,*/*;q=0.8")
	if ct := browser.Header().Get("Content-Type"); browser.Code == http.StatusOK && !strings.HasPrefix(ct, "text/html") {
		t.Errorf("browser navigation: got %d %q, want the UI page", browser.Code, ct)
	}

	for _, accept := range []string{"text/event-stream", "application/json, text/event-stream", ""} {
		rr := get(accept)
		if strings.HasPrefix(rr.Header().Get("Content-Type"), "text/html") {
			t.Errorf("Accept %q: got the UI page, want the MCP handler", accept)
		}
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Accept %q: status %d, want 401 from MCP auth", accept, rr.Code)
		}
	}
}
