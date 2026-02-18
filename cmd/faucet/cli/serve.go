package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/mssql"
	"github.com/faucetdb/faucet/internal/connector/mysql"
	"github.com/faucetdb/faucet/internal/connector/postgres"
	"github.com/faucetdb/faucet/internal/connector/snowflake"
	"github.com/faucetdb/faucet/internal/server"
	"github.com/faucetdb/faucet/internal/service"
)

const banner = `
 _____ _   _   _  ___ ___ _____
|  ___/ \ | | | |/ __| __|_   _|
| |_ / _ \| |_| | (__|  _| | |
|_| /_/ \_\___,_|\___|___| |_|
`

func newServeCmd() *cobra.Command {
	var (
		port    int
		host    string
		noUI    bool
		dev     bool
		dataDir string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Faucet API server",
		Long:  "Start the HTTP server that exposes REST APIs for all configured database services.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(host, port, noUI, dev, dataDir)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "HTTP listen port")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "HTTP listen host")
	cmd.Flags().BoolVar(&noUI, "no-ui", false, "Disable the admin UI")
	cmd.Flags().BoolVar(&dev, "dev", false, "Enable development mode (verbose logging, CORS *)")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for SQLite config (default: ~/.faucet)")

	viper.BindPFlag("server.port", cmd.Flags().Lookup("port"))
	viper.BindPFlag("server.host", cmd.Flags().Lookup("host"))

	return cmd
}

func runServe(host string, port int, noUI, dev bool, dataDir string) error {
	fmt.Print(banner)
	fmt.Println()

	// Set up logger
	logLevel := slog.LevelInfo
	if dev {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	// 1. Initialize config store (SQLite)
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = home + "/.faucet"
	}
	store, err := config.NewStore(dataDir)
	if err != nil {
		return fmt.Errorf("init config store: %w", err)
	}
	defer store.Close()
	logger.Info("config store initialized", "path", dataDir)

	// 2. Initialize connector registry and register drivers
	registry := connector.NewRegistry()
	registry.RegisterDriver("postgres", func() connector.Connector { return postgres.New() })
	registry.RegisterDriver("mysql", func() connector.Connector { return mysql.New() })
	registry.RegisterDriver("mssql", func() connector.Connector { return mssql.New() })
	registry.RegisterDriver("snowflake", func() connector.Connector { return snowflake.New() })
	logger.Info("connector registry initialized", "drivers", []string{"postgres", "mysql", "mssql", "snowflake"})

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
			DSN:             svc.DSN,
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

	// 6. Build and start HTTP server
	srvCfg := server.Config{
		Host:            host,
		Port:            port,
		ShutdownTimeout: 30 * 1e9, // 30s
		CORSOrigins:     []string{"*"},
		EnableUI:        !noUI,
		MaxBodySize:     10 * 1024 * 1024,
	}

	srv := server.New(srvCfg, registry, store, authSvc, logger)

	fmt.Printf("→ Faucet v0.1.0\n")
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
