package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Faucet configuration",
		Long:  "Initialize a default configuration file or display the current effective configuration.",
	}

	cmd.AddCommand(newConfigInitCmd())
	cmd.AddCommand(newConfigShowCmd())

	return cmd
}

// ---------- config init ----------

func newConfigInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default faucet.yaml configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config file")

	return cmd
}

const defaultConfig = `# Faucet Configuration
# https://github.com/faucetdb/faucet

server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 15s
  write_timeout: 30s
  cors:
    allowed_origins:
      - "*"

# Database service connections
# Add databases with 'faucet db add' or list them here:
services: []
  # - name: mydb
  #   driver: postgres
  #   dsn: postgres://user:pass@localhost:5432/mydb?sslmode=disable
  #   schema: public
  #   read_only: false

# SQLite config store (holds API keys, roles, admins)
config_db: faucet.db

# Authentication
auth:
  jwt_secret: ""  # Set via FAUCET_AUTH_JWT_SECRET env var
  api_key_header: X-Faucet-Api-Key

# Rate limiting
rate_limit:
  enabled: false
  requests_per_second: 100
  burst: 200

# Logging
log:
  level: info    # debug, info, warn, error
  format: text   # text or json

# MCP server
mcp:
  enabled: false
  transport: stdio
`

func runConfigInit(force bool) error {
	path := "faucet.yaml"

	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
	}

	if err := os.WriteFile(path, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created %s\n", path)
	fmt.Println("Edit the file to add your database connections, then run 'faucet serve'.")
	return nil
}

// ---------- config show ----------

func newConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the current effective configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow()
		},
	}

	return cmd
}

func runConfigShow() error {
	// Ensure config is loaded
	initConfig()

	configFile := viper.ConfigFileUsed()
	if configFile != "" {
		fmt.Printf("Config file: %s\n", configFile)
	} else {
		fmt.Println("Config file: (none found, using defaults)")
	}
	fmt.Println()

	// Print all settings
	settings := viper.AllSettings()
	if len(settings) == 0 {
		fmt.Println("No configuration settings loaded.")
		fmt.Println("Run 'faucet config init' to create a default configuration file.")
		return nil
	}

	for key, value := range settings {
		fmt.Printf("  %s: %v\n", key, value)
	}

	return nil
}
