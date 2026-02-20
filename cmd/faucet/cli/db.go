package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/model"
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
		name           string
		driver         string
		dsn            string
		label          string
		schema         string
		privateKeyPath string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a database connection",
		Long: `Add a new database service connection. Provide flags for non-interactive use,
or omit them to be prompted interactively.

Supported drivers: postgres, mysql, mssql, snowflake, sqlite`,
		Example: `  faucet db add --name mydb --driver postgres --dsn "postgres://user:pass@localhost/mydb"
  faucet db add --name analytics --driver snowflake --dsn "USER@org-account/DB/SCHEMA" --private-key-path /path/to/key.p8
  faucet db add  # interactive mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBAdd(name, driver, dsn, label, schema, privateKeyPath)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Service name (unique identifier)")
	cmd.Flags().StringVar(&driver, "driver", "", "Database driver (postgres, mysql, mssql, snowflake, sqlite)")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Data source name / connection string")
	cmd.Flags().StringVar(&label, "label", "", "Human-readable label (defaults to name)")
	cmd.Flags().StringVar(&schema, "schema", "", "Database schema to expose (default depends on driver)")
	cmd.Flags().StringVar(&privateKeyPath, "private-key-path", "", "Path to private key file (for Snowflake key-pair auth)")

	return cmd
}

func runDBAdd(name, driver, dsn, label, schema, privateKeyPath string) error {
	// Interactive prompts when flags are missing
	if name == "" {
		fmt.Print("Service name: ")
		fmt.Scanln(&name)
	}
	if driver == "" {
		fmt.Print("Driver (postgres, mysql, mssql, snowflake, sqlite): ")
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
		"postgres": true, "mysql": true, "mssql": true, "snowflake": true, "sqlite": true,
	}
	if !supportedDrivers[driver] {
		return fmt.Errorf("unsupported driver %q; supported: postgres, mysql, mssql, snowflake, sqlite", driver)
	}

	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc := &model.ServiceConfig{
		Name:           name,
		Label:          label,
		Driver:         driver,
		DSN:            dsn,
		PrivateKeyPath: privateKeyPath,
		Schema:         schema,
		IsActive:       true,
		Pool:           model.DefaultPoolConfig(),
	}

	if err := store.CreateService(ctx, svc); err != nil {
		return fmt.Errorf("create service: %w", err)
	}

	fmt.Printf("Added service %q (driver=%s, id=%d)\n", name, driver, svc.ID)
	return nil
}

// ---------- db list ----------

func newDBListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all registered database services",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBList(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runDBList(jsonOutput bool) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()
	services, err := store.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	if jsonOutput {
		type serviceRow struct {
			Name   string `json:"name"`
			Driver string `json:"driver"`
			Label  string `json:"label"`
			Schema string `json:"schema"`
			Active bool   `json:"active"`
		}
		rows := make([]serviceRow, len(services))
		for i, s := range services {
			rows[i] = serviceRow{
				Name:   s.Name,
				Driver: s.Driver,
				Label:  s.Label,
				Schema: s.Schema,
				Active: s.IsActive,
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(services) == 0 {
		fmt.Println("No services configured. Use 'faucet db add' to add one.")
		return nil
	}

	fmt.Printf("%-20s %-12s %-8s\n", "NAME", "DRIVER", "ACTIVE")
	fmt.Printf("%-20s %-12s %-8s\n", "----", "------", "------")
	for _, s := range services {
		active := "yes"
		if !s.IsActive {
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
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc, err := store.GetServiceByName(ctx, name)
	if err != nil {
		return fmt.Errorf("look up service %q: %w", name, err)
	}

	if err := store.DeleteService(ctx, svc.ID); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}

	fmt.Printf("Removed service %q\n", name)
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
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc, err := store.GetServiceByName(ctx, name)
	if err != nil {
		return fmt.Errorf("look up service %q: %w", name, err)
	}

	registry := newRegistry()
	defer registry.CloseAll()

	cfg := connector.ConnectionConfig{
		Driver:          svc.Driver,
		DSN:             svc.DSN,
		PrivateKeyPath:  svc.PrivateKeyPath,
		SchemaName:      svc.Schema,
		MaxOpenConns:    svc.Pool.MaxOpenConns,
		MaxIdleConns:    svc.Pool.MaxIdleConns,
		ConnMaxLifetime: svc.Pool.ConnMaxLifetime,
		ConnMaxIdleTime: svc.Pool.ConnMaxIdleTime,
	}

	fmt.Printf("Testing connection %q (driver=%s)...\n", name, svc.Driver)

	if err := registry.Connect(name, cfg); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	conn, err := registry.Get(name)
	if err != nil {
		return fmt.Errorf("get connector: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	fmt.Println("Connection successful.")
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
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc, err := store.GetServiceByName(ctx, name)
	if err != nil {
		return fmt.Errorf("look up service %q: %w", name, err)
	}

	registry := newRegistry()
	defer registry.CloseAll()

	cfg := connector.ConnectionConfig{
		Driver:          svc.Driver,
		DSN:             svc.DSN,
		PrivateKeyPath:  svc.PrivateKeyPath,
		SchemaName:      svc.Schema,
		MaxOpenConns:    svc.Pool.MaxOpenConns,
		MaxIdleConns:    svc.Pool.MaxIdleConns,
		ConnMaxLifetime: svc.Pool.ConnMaxLifetime,
		ConnMaxIdleTime: svc.Pool.ConnMaxIdleTime,
	}

	if err := registry.Connect(name, cfg); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	conn, err := registry.Get(name)
	if err != nil {
		return fmt.Errorf("get connector: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if tableName != "" {
		table, err := conn.IntrospectTable(ctx, tableName)
		if err != nil {
			return fmt.Errorf("introspect table %q: %w", tableName, err)
		}
		return enc.Encode(table)
	}

	schema, err := conn.IntrospectSchema(ctx)
	if err != nil {
		return fmt.Errorf("introspect schema: %w", err)
	}
	return enc.Encode(schema)
}
