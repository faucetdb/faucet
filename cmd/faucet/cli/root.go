package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile    string
	appVersion string // set in Execute, used by serve for telemetry
)

// Execute creates the root command tree and runs it.
func Execute(version, commit, date string) error {
	appVersion = version
	rootCmd := newRootCmd(version, commit, date)
	return rootCmd.Execute()
}

func newRootCmd(version, commit, date string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "faucet",
		Short: "Turn any database into a secure REST API",
		Long: `Faucet: Turn any database into a secure REST API. One binary. One command. Zero configuration.

Faucet connects to your SQL databases, introspects their schemas, and automatically
generates production-ready REST APIs with filtering, pagination, RBAC, OpenAPI docs,
and a built-in MCP server for AI agents.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./faucet.yaml)")
	cmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "data directory for SQLite config (default: ~/.faucet)")

	cobra.OnInitialize(initConfig)

	// Add subcommands
	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newVersionCmd(version, commit, date))
	cmd.AddCommand(newDBCmd())
	cmd.AddCommand(newKeyCmd())
	cmd.AddCommand(newRoleCmd())
	cmd.AddCommand(newAdminCmd())
	cmd.AddCommand(newOpenAPICmd())
	cmd.AddCommand(newMCPCmd())
	cmd.AddCommand(newBenchmarkCmd())
	cmd.AddCommand(newConfigCmd())

	return cmd
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("faucet")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.faucet")
	}

	viper.SetEnvPrefix("FAUCET")
	// Map nested keys to env var names: "auth.jwt_secret" -> FAUCET_AUTH_JWT_SECRET.
	// Without this replacer AutomaticEnv would look for FAUCET_AUTH.JWT_SECRET,
	// which no shell can set, and every documented FAUCET_* variable for a
	// nested key would be silently ignored.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	// Accept both the canonical name and the historical short alias for the
	// JWT signing secret. The first non-empty variable wins.
	_ = viper.BindEnv("auth.jwt_secret", "FAUCET_AUTH_JWT_SECRET", "FAUCET_JWT_SECRET")
	viper.ReadInConfig() // Ignore error - config file is optional
}
