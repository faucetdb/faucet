package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/handler"
	"github.com/faucetdb/faucet/internal/server/middleware"
	"github.com/faucetdb/faucet/internal/service"
	"github.com/faucetdb/faucet/internal/ui"
)

// Config holds the HTTP server configuration.
type Config struct {
	Host            string
	Port            int
	ShutdownTimeout time.Duration
	CORSOrigins     []string
	EnableUI        bool
	MaxBodySize     int64 // bytes
}

// DefaultConfig returns a Config with sensible production defaults.
func DefaultConfig() Config {
	return Config{
		Host:            "0.0.0.0",
		Port:            8080,
		ShutdownTimeout: 30 * time.Second,
		CORSOrigins:     []string{"*"},
		EnableUI:        true,
		MaxBodySize:     10 * 1024 * 1024, // 10MB
	}
}

// Server is the top-level HTTP server for Faucet. It owns the Chi router,
// the connector registry, configuration store, and authentication service.
type Server struct {
	cfg        Config
	router     chi.Router
	registry   *connector.Registry
	store      *config.Store
	authSvc    *service.AuthService
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates a new Server, wires up all routes and middleware, and returns
// it ready to listen. Call ListenAndServe to start accepting connections.
func New(cfg Config, registry *connector.Registry, store *config.Store, authSvc *service.AuthService, logger *slog.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		registry: registry,
		store:    store,
		authSvc:  authSvc,
		logger:   logger,
	}
	s.setupRouter()
	return s
}

func (s *Server) setupRouter() {
	r := chi.NewRouter()

	// --- Global middleware ---
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(s.logger))
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Requested-With"},
		ExposedHeaders:   []string{"X-Total-Count", "X-Request-ID", "Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(chimw.Compress(5))

	// --- Health checks (no auth required) ---
	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// --- OpenAPI combined spec (no auth required) ---
	r.Get("/openapi.json", handler.NewOpenAPIHandler(s.registry, s.store).ServeCombinedSpec)

	// --- API routes ---
	r.Route("/api/v1", func(r chi.Router) {

		// System APIs (admin management)
		r.Route("/system", func(r chi.Router) {
			sysHandler := handler.NewSystemHandler(s.store, s.authSvc)

			// Session endpoints are unauthenticated (login) or self-authenticated (logout)
			r.Post("/admin/session", sysHandler.Login)
			r.Delete("/admin/session", sysHandler.Logout)

			// All other system endpoints require admin authentication
			r.Group(func(r chi.Router) {
				r.Use(middleware.Authenticate(s.authSvc))
				r.Use(middleware.RequireAdmin())

				// Service management
				r.Get("/service", sysHandler.ListServices)
				r.Post("/service", sysHandler.CreateService)
				r.Get("/service/{serviceName}", sysHandler.GetService)
				r.Put("/service/{serviceName}", sysHandler.UpdateService)
				r.Delete("/service/{serviceName}", sysHandler.DeleteService)

				// Role management
				r.Get("/role", sysHandler.ListRoles)
				r.Post("/role", sysHandler.CreateRole)
				r.Get("/role/{roleId}", sysHandler.GetRole)
				r.Put("/role/{roleId}", sysHandler.UpdateRole)
				r.Delete("/role/{roleId}", sysHandler.DeleteRole)

				// Admin management
				r.Get("/admin", sysHandler.ListAdmins)
				r.Post("/admin", sysHandler.CreateAdmin)

				// API key management
				r.Get("/api-key", sysHandler.ListAPIKeys)
				r.Post("/api-key", sysHandler.CreateAPIKey)
				r.Delete("/api-key/{keyId}", sysHandler.RevokeAPIKey)
			})
		})

		// Dynamic database service APIs
		r.Route("/{serviceName}", func(r chi.Router) {
			r.Use(middleware.Authenticate(s.authSvc))

			tableHandler := handler.NewTableHandler(s.registry, s.store)
			schemaHandler := handler.NewSchemaHandler(s.registry, s.store)
			procHandler := handler.NewProcHandler(s.registry)
			openAPIHandler := handler.NewOpenAPIHandler(s.registry, s.store)

			// Schema introspection and DDL
			r.Get("/_schema", schemaHandler.ListTables)
			r.Get("/_schema/{tableName}", schemaHandler.GetTableSchema)
			r.Post("/_schema", schemaHandler.CreateTable)
			r.Put("/_schema/{tableName}", schemaHandler.AlterTable)
			r.Delete("/_schema/{tableName}", schemaHandler.DropTable)

			// Table CRUD
			r.Get("/_table", tableHandler.ListTableNames)
			r.Get("/_table/{tableName}", tableHandler.QueryRecords)
			r.Post("/_table/{tableName}", tableHandler.CreateRecords)
			r.Put("/_table/{tableName}", tableHandler.ReplaceRecords)
			r.Patch("/_table/{tableName}", tableHandler.UpdateRecords)
			r.Delete("/_table/{tableName}", tableHandler.DeleteRecords)

			// Stored procedures
			r.Get("/_proc", procHandler.ListProcedures)
			r.Post("/_proc/{procName}", procHandler.CallProcedure)

			// Per-service OpenAPI spec
			r.Get("/_doc", openAPIHandler.ServeServiceSpec)
		})
	})

	// --- Embedded admin UI ---
	if s.cfg.EnableUI {
		// Serve the embedded SPA. The dist/ directory is produced by
		// `cd ui && npm run build` and embedded via go:embed in internal/ui.
		distFS, err := fs.Sub(ui.Dist, "dist")
		if err != nil {
			s.logger.Error("failed to create sub filesystem for UI", "error", err)
		} else {
			fileServer := http.FileServer(http.FS(distFS))
			// Serve static assets directly
			r.Handle("/assets/*", fileServer)
			r.Get("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
				fileServer.ServeHTTP(w, r)
			})
			// SPA fallback: serve index.html for all UI routes
			spaHandler := func(w http.ResponseWriter, r *http.Request) {
				f, err := distFS.Open("index.html")
				if err != nil {
					http.Error(w, "UI not available", http.StatusNotFound)
					return
				}
				defer f.Close()
				stat, _ := f.Stat()
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				http.ServeContent(w, r, "index.html", stat.ModTime(), f.(io.ReadSeeker))
			}
			r.Get("/admin", spaHandler)
			r.Get("/admin/*", spaHandler)
			r.Get("/setup", spaHandler)
			r.Get("/services", spaHandler)
			r.Get("/schema", spaHandler)
			r.Get("/api-explorer", spaHandler)
			r.Get("/roles", spaHandler)
			r.Get("/api-keys", spaHandler)
			r.Get("/settings", spaHandler)
			// Root serves the SPA
			r.Get("/", spaHandler)
		}
	}

	s.router = r
}

// handleHealthz is a liveness probe. Returns 200 if the process is running.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleReadyz is a readiness probe. Returns 200 when all database connectors
// are reachable, or 503 if any connector is unhealthy.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	httpStatus := http.StatusOK
	checks := make(map[string]string)

	// Check all active connectors
	for _, name := range s.registry.ListServices() {
		conn, err := s.registry.Get(name)
		if err != nil {
			checks[name] = "error: " + err.Error()
			status = "degraded"
			continue
		}
		if err := conn.Ping(r.Context()); err != nil {
			checks[name] = "error: " + err.Error()
			status = "degraded"
		} else {
			checks[name] = "ok"
		}
	}

	if status != "ok" {
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"checks": checks,
	})
}

// ListenAndServe starts the HTTP server and blocks until a SIGINT or SIGTERM
// is received. It then performs a graceful shutdown, draining in-flight
// requests before closing all database connections.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Listen for shutdown signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start server in background goroutine
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("server starting", "addr", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-errCh:
		return fmt.Errorf("server listen: %w", err)
	case <-ctx.Done():
		s.logger.Info("shutdown signal received, draining connections...")
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	// Close all database connections
	s.registry.CloseAll()
	s.logger.Info("server stopped")
	return nil
}

// Router returns the underlying Chi router, useful for testing.
func (s *Server) Router() chi.Router {
	return s.router
}

// ServeHTTP implements http.Handler, delegating to the router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
