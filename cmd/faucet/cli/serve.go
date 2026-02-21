package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/server"
	"github.com/faucetdb/faucet/internal/service"
	"github.com/faucetdb/faucet/internal/telemetry"
)

const banner = `
 _____ _   _   _  ___ ___ _____
|  ___/ \ | | | |/ __| __|_   _|
| |_ / _ \| |_| | (__|  _| | |
|_| /_/ \_\___,_|\___|___| |_|
`

func newServeCmd() *cobra.Command {
	var (
		port       int
		host       string
		noUI       bool
		dev        bool
		foreground bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Faucet API server",
		Long: `Start the HTTP server that exposes REST APIs for all configured database services.

By default, the server starts in the background and returns control to your
terminal so you can immediately run other faucet commands (db add, key create,
etc.). Use --foreground for Docker, systemd, or other process managers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Dev mode implies foreground for live log viewing
			if dev {
				foreground = true
			}
			if foreground {
				return runServe(host, port, noUI, dev)
			}
			return runServeDaemon(host, port, noUI, dev)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "HTTP listen port")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "HTTP listen host")
	cmd.Flags().BoolVar(&noUI, "no-ui", false, "Disable the admin UI")
	cmd.Flags().BoolVar(&dev, "dev", false, "Enable development mode (verbose logging, CORS *)")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run in foreground (for Docker, systemd, etc.)")

	viper.BindPFlag("server.port", cmd.Flags().Lookup("port"))
	viper.BindPFlag("server.host", cmd.Flags().Lookup("host"))

	return cmd
}

// runServeDaemon starts the server as a background process and returns control to the terminal.
func runServeDaemon(host string, port int, noUI, dev bool) error {
	// Check if server is already running
	if pid, err := readPID(); err == nil {
		if isProcessRunning(pid) {
			return fmt.Errorf("server already running (PID %d) — use 'faucet stop' first", pid)
		}
		removePID()
	}

	fmt.Print(banner)
	fmt.Println()

	// Find our executable
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	// Build args for child process in foreground mode
	args := []string{"serve", "--foreground"}
	args = append(args, "--host", host)
	args = append(args, "-p", strconv.Itoa(port))
	if noUI {
		args = append(args, "--no-ui")
	}
	if dataDir != "" {
		args = append(args, "--data-dir", dataDir)
	}
	if cfgFile != "" {
		args = append(args, "--config", cfgFile)
	}

	// Ensure data directory exists and open log file
	dir := resolveDataDir()
	os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "faucet.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	// Start detached child process
	child := exec.Command(exe, args...)
	child.Stdout = logFile
	child.Stderr = logFile
	child.Stdin = nil
	setSysProcAttr(child)

	if err := child.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start server process: %w", err)
	}
	logFile.Close()

	pid := child.Process.Pid

	// Wait for server to become healthy
	healthAddr := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	if host != "0.0.0.0" && host != "" {
		healthAddr = fmt.Sprintf("http://%s:%d/healthz", host, port)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	healthy := false
	for i := 0; i < 50; i++ { // up to 5 seconds
		time.Sleep(100 * time.Millisecond)
		resp, err := client.Get(healthAddr)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				healthy = true
				break
			}
		}
	}

	if !healthy {
		fmt.Fprintf(os.Stderr, "warning: server may not have started correctly (PID %d)\n", pid)
		fmt.Fprintf(os.Stderr, "  check logs: %s\n\n", logPath)
	}

	fmt.Printf("  Faucet %s\n", versionString())
	fmt.Printf("  Listening on http://%s:%d\n", host, port)
	if !noUI {
		fmt.Printf("  Admin UI:   http://%s:%d/admin\n", host, port)
	}
	fmt.Printf("  OpenAPI:    http://%s:%d/openapi.json\n", host, port)
	fmt.Printf("  Health:     http://%s:%d/healthz\n", host, port)
	fmt.Println()
	fmt.Printf("  Server running in background (PID %d)\n", pid)
	fmt.Printf("  Logs: %s\n", logPath)
	fmt.Println()
	fmt.Println("Use 'faucet stop' to stop the server.")
	fmt.Println("Use 'faucet status' to check server health.")

	return nil
}

// runServe starts the server in the foreground (blocking mode).
func runServe(host string, port int, noUI, dev bool) error {
	fmt.Print(banner)
	fmt.Println()

	// Set up logger
	logLevel := slog.LevelInfo
	if dev {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	// 1. Initialize config store (SQLite)
	dir := resolveDataDir()
	store, err := config.NewStore(dir)
	if err != nil {
		return fmt.Errorf("init config store: %w", err)
	}
	defer store.Close()
	logger.Info("config store initialized", "path", dir)

	// 2. Initialize connector registry and register drivers
	registry := newRegistry()
	logger.Info("connector registry initialized", "drivers", []string{"postgres", "mysql", "mssql", "snowflake", "sqlite"})

	// 3. Load services from config and connect them
	services, err := store.ListServices(cmd_ctx())
	if err != nil {
		logger.Warn("failed to load services from config", "error", err)
	}
	for _, svc := range services {
		if !svc.IsActive {
			continue
		}
		cfg := connector.ConnectionConfig{
			Driver:          svc.Driver,
			DSN:             connector.SanitizeDSN(svc.Driver, svc.DSN),
			PrivateKeyPath:  svc.PrivateKeyPath,
			SchemaName:      svc.Schema,
			MaxOpenConns:    svc.Pool.MaxOpenConns,
			MaxIdleConns:    svc.Pool.MaxIdleConns,
			ConnMaxLifetime: svc.Pool.ConnMaxLifetime,
			ConnMaxIdleTime: svc.Pool.ConnMaxIdleTime,
		}
		if err := registry.Connect(svc.Name, cfg); err != nil {
			logger.Error("failed to connect service", "service", svc.Name, "error", err)
		} else {
			logger.Info("connected service", "service", svc.Name, "driver", svc.Driver)
		}
	}

	// 4. Initialize auth service
	jwtSecret := viper.GetString("auth.jwt_secret")
	if jwtSecret == "" {
		jwtSecret = "faucet-dev-secret-change-me"
	}
	authSvc := service.NewAuthService(store, jwtSecret)

	// 5. Check for first-run (no admin exists)
	hasAdmin, err := store.HasAnyAdmin(cmd_ctx())
	if err != nil {
		logger.Warn("failed to check for admin", "error", err)
	}
	if !hasAdmin {
		logger.Warn("no admin account found - visit /setup or run: faucet admin create")
	}

	// 6. Initialize telemetry
	tracker := telemetry.New(cmd_ctx(), store, func() telemetry.Properties {
		// Gather current state for each heartbeat
		svcs, _ := store.ListServices(cmd_ctx())
		admins, _ := store.ListAdmins(cmd_ctx())
		keys, _ := store.ListAPIKeys(cmd_ctx())
		roles, _ := store.ListRoles(cmd_ctx())

		dbTypes := make([]string, 0)
		tableCount := 0
		seen := make(map[string]bool)
		for _, svc := range svcs {
			if !svc.IsActive {
				continue
			}
			if !seen[svc.Driver] {
				dbTypes = append(dbTypes, svc.Driver)
				seen[svc.Driver] = true
			}
			// Count tables from connected services
			if conn, err := registry.Get(svc.Name); err == nil {
				if names, err := conn.GetTableNames(cmd_ctx()); err == nil {
					tableCount += len(names)
				}
			}
		}

		features := make([]string, 0)
		if !noUI {
			features = append(features, "ui")
		}
		if len(roles) > 0 {
			features = append(features, "rbac")
		}
		if len(keys) > 0 {
			features = append(features, "api_keys")
		}

		return telemetry.Properties{
			Version:   appVersion,
			GoVersion: runtime.Version(),
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			DBTypes:   dbTypes,
			Services:  len(registry.ListServices()),
			Tables:    tableCount,
			Admins:    len(admins),
			APIKeys:   len(keys),
			Roles:     len(roles),
			Features:  features,
		}
	})
	if tracker != nil {
		telemetry.PrintNotice()
		tracker.Start()
		defer tracker.Shutdown()
		logger.Info("telemetry enabled", "docs", "https://github.com/faucetdb/faucet/blob/main/TELEMETRY.md")
	} else {
		logger.Info("telemetry disabled")
	}

	// Write PID file so stop/status commands can find this process
	if err := writePID(os.Getpid()); err != nil {
		logger.Warn("failed to write PID file", "error", err)
	}
	defer removePID()

	// 7. Build and start HTTP server
	srvCfg := server.Config{
		Host:            host,
		Port:            port,
		ShutdownTimeout: 30 * 1e9, // 30s
		CORSOrigins:     []string{"*"},
		EnableUI:        !noUI,
		MaxBodySize:     10 * 1024 * 1024,
	}

	srv := server.New(srvCfg, registry, store, authSvc, logger)

	fmt.Printf("→ Faucet %s\n", versionString())
	fmt.Printf("→ Listening on http://%s:%d\n", host, port)
	if !noUI {
		fmt.Printf("→ Admin UI:   http://%s:%d/admin\n", host, port)
	}
	fmt.Printf("→ OpenAPI:    http://%s:%d/openapi.json\n", host, port)
	fmt.Printf("→ Health:     http://%s:%d/healthz\n", host, port)
	fmt.Printf("→ Connected databases: %d\n", len(registry.ListServices()))
	fmt.Println()

	return srv.ListenAndServe()
}

// cmd_ctx returns a background context for CLI initialization.
func cmd_ctx() context.Context {
	return context.Background()
}
