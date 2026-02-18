package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "db",
		Aliases: []string{"service", "database"},
		Short:   "Manage database connections",
		Long:    "Add, remove, test, and inspect database service connections.",
	}

	cmd.AddCommand(newDBAddCmd())
	cmd.AddCommand(newDBListCmd())
	cmd.AddCommand(newDBRemoveCmd())
	cmd.AddCommand(newDBTestCmd())
	cmd.AddCommand(newDBSchemaCmd())

	return cmd
}

// ---------- db add ----------

func newDBAddCmd() *cobra.Command {
	var (
		name   string
		driver string
		dsn    string
		label  string
		schema string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a database connection",
		Long: `Add a new database service connection. Provide flags for non-interactive use,
or omit them to be prompted interactively.

Supported drivers: postgres, mysql, mssql, snowflake`,
		Example: `  faucet db add --name mydb --driver postgres --dsn "postgres://user:pass@localhost/mydb"
  faucet db add  # interactive mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBAdd(name, driver, dsn, label, schema)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Service name (unique identifier)")
	cmd.Flags().StringVar(&driver, "driver", "", "Database driver (postgres, mysql, mssql, snowflake)")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Data source name / connection string")
	cmd.Flags().StringVar(&label, "label", "", "Human-readable label (defaults to name)")
	cmd.Flags().StringVar(&schema, "schema", "", "Database schema to expose (default depends on driver)")

	return cmd
}

func runDBAdd(name, driver, dsn, label, schema string) error {
	// Interactive prompts when flags are missing
	if name == "" {
		fmt.Print("Service name: ")
		fmt.Scanln(&name)
	}
	if driver == "" {
		fmt.Print("Driver (postgres, mysql, mssql, snowflake): ")
		fmt.Scanln(&driver)
	}
	if dsn == "" {
		fmt.Print("DSN (connection string): ")
		fmt.Scanln(&dsn)
	}
	if label == "" {
		label = name
	}

	// Validate required fields
	if name == "" || driver == "" || dsn == "" {
		return fmt.Errorf("name, driver, and dsn are required")
	}

	supportedDrivers := map[string]bool{
		"postgres": true, "mysql": true, "mssql": true, "snowflake": true,
	}
	if !supportedDrivers[driver] {
		return fmt.Errorf("unsupported driver %q; supported: postgres, mysql, mssql, snowflake", driver)
	}

	// TODO: open config store, insert service config
	// store, err := config.Open(...)
	// svc := model.ServiceConfig{Name: name, Driver: driver, DSN: dsn, Label: label, Schema: schema, IsActive: true}
	// err = store.CreateService(svc)
	fmt.Printf("Added service %q (driver=%s)\n", name, driver)
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}

// ---------- db list ----------

func newDBListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered database services",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBList(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runDBList(jsonOutput bool) error {
	// TODO: open config store, list services
	// store, _ := config.Open(...)
	// services, err := store.ListServices()

	// Placeholder output
	type serviceRow struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
		Active bool   `json:"active"`
	}

	services := []serviceRow{} // empty until config store is wired

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(services)
	}

	if len(services) == 0 {
		fmt.Println("No services configured. Use 'faucet db add' to add one.")
		return nil
	}

	fmt.Printf("%-20s %-12s %-8s\n", "NAME", "DRIVER", "ACTIVE")
	fmt.Printf("%-20s %-12s %-8s\n", "----", "------", "------")
	for _, s := range services {
		active := "yes"
		if !s.Active {
			active = "no"
		}
		fmt.Printf("%-20s %-12s %-8s\n", s.Name, s.Driver, active)
	}

	return nil
}

// ---------- db remove ----------

func newDBRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove a database service",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBRemove(args[0])
		},
	}

	return cmd
}

func runDBRemove(name string) error {
	// TODO: open config store, delete service
	// store, _ := config.Open(...)
	// err := store.DeleteService(name)

	fmt.Printf("Removed service %q\n", name)
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}

// ---------- db test ----------

func newDBTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <name>",
		Short: "Test a database connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBTest(args[0])
		},
	}

	return cmd
}

func runDBTest(name string) error {
	// TODO: open config store, look up service config, create connector, ping
	// store, _ := config.Open(...)
	// svc, err := store.GetService(name)
	// registry := connector.NewRegistry()
	// registerDrivers(registry)
	// err = registry.Connect(name, connector.ConnectionConfig{Driver: svc.Driver, DSN: svc.DSN})
	// conn, _ := registry.Get(name)
	// err = conn.Ping(context.Background())

	fmt.Printf("Testing connection %q...\n", name)
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}

// ---------- db schema ----------

func newDBSchemaCmd() *cobra.Command {
	var tableName string

	cmd := &cobra.Command{
		Use:   "schema <name>",
		Short: "Print database schema as JSON",
		Long:  "Introspect the database schema and print it as JSON. Optionally filter to a single table.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBSchema(args[0], tableName)
		},
	}

	cmd.Flags().StringVar(&tableName, "table", "", "Show schema for a single table only")

	return cmd
}

func runDBSchema(name, tableName string) error {
	// TODO: open config store, connect, introspect
	// store, _ := config.Open(...)
	// svc, err := store.GetService(name)
	// registry := connector.NewRegistry()
	// registerDrivers(registry)
	// registry.Connect(name, connector.ConnectionConfig{Driver: svc.Driver, DSN: svc.DSN})
	// conn, _ := registry.Get(name)
	//
	// if tableName != "" {
	//     schema, err := conn.IntrospectTable(context.Background(), tableName)
	// } else {
	//     schema, err := conn.IntrospectSchema(context.Background())
	// }

	fmt.Printf("Schema for service %q", name)
	if tableName != "" {
		fmt.Printf(" (table: %s)", tableName)
	}
	fmt.Println()
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}
