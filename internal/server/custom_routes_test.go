package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCustomSPAHandler(t *testing.T) {
	// Create a temporary directory to act as the static files root (dist folder)
	tmpDir, err := os.MkdirTemp("", "spa-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock index.html file
	indexPath := filepath.Join(tmpDir, "index.html")
	indexContent := "<html><body>SPA Root Index</body></html>"
	if err := os.WriteFile(indexPath, []byte(indexContent), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}

	// Create a mock assets subdirectory and an asset file
	assetsDir := filepath.Join(tmpDir, "assets")
	if err := os.Mkdir(assetsDir, 0755); err != nil {
		t.Fatalf("failed to create assets dir: %v", err)
	}
	assetPath := filepath.Join(assetsDir, "app.js")
	assetContent := "console.log('SPA asset loaded');"
	if err := os.WriteFile(assetPath, []byte(assetContent), 0644); err != nil {
		t.Fatalf("failed to write app.js: %v", err)
	}

	// Initialize the handler with "/_" as the base prefix
	basePrefix := "/_"
	handler := customSPAHandler(tmpDir, basePrefix)

	// Define test cases
	tests := []struct {
		name           string
		targetURL      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Serve existing static asset",
			targetURL:      "/_/assets/app.js",
			expectedStatus: http.StatusOK,
			expectedBody:   assetContent,
		},
		{
			name:           "Fallback to index.html for client-side route",
			targetURL:      "/_/dashboard/settings",
			expectedStatus: http.StatusOK,
			expectedBody:   indexContent,
		},
		{
			name:           "Fallback to index.html for base root path",
			targetURL:      "/_/",
			expectedStatus: http.StatusOK,
			expectedBody:   indexContent,
		},
	}

	// Run tests
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.targetURL, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rec.Code)
			}

			if rec.Body.String() != tc.expectedBody {
				t.Errorf("expected body %q, got %q", tc.expectedBody, rec.Body.String())
			}
		})
	}
}
