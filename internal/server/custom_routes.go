package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

/* This example shows how you can setup a route to serve your custom SPA.
* You may add other custom routes here. The routes are added after Faucet owns routes.
 */

func (s *Server) setupCustomRoutes() {
	s.router.Get("/_/*", customSPAHandler("./frontend/dist", "/_"))
}

// customSPAHandler serves static files or falls back to index.html for client-side routes
func customSPAHandler(staticDir string, basePrefix string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticDir))

	return func(w http.ResponseWriter, r *http.Request) {
		// Normalize the path by removing the prefix if it exists (e.g., "/_/")
		p := r.URL.Path
		if basePrefix != "" && strings.HasPrefix(p, basePrefix) {
			p = strings.TrimPrefix(p, basePrefix)
		}

		// Build the full path to the file inside the distribution directory
		fullPath := filepath.Join(staticDir, filepath.Clean(p))

		// Check if the file exists and is not a directory
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			// Serve the static file (JS, CSS, images, etc.)
			http.StripPrefix(basePrefix, fileServer).ServeHTTP(w, r)
			return
		}

		// If the file does not exist, respond with index.html so the SPA can handle the route
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	}
}
