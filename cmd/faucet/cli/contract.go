package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/contract"
)

func init() {
	// These commands are registered in db.go via newDBCmd
}

func newDBLockCmd() *cobra.Command {
	var tableName string

	cmd := &cobra.Command{
		Use:   "lock <service>",
		Short: "Lock the schema contract for a service or table",
		Long: `Snapshot the current database schema as the API contract. When schema_lock
is set to "auto" or "strict", Faucet will compare the live schema against this
contract and protect downstream consumers from breaking changes.

Lock a single table or all tables in a service.`,
		Example: `  faucet db lock mydb                    # Lock all tables
  faucet db lock mydb --table users      # Lock a single table`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBLock(args[0], tableName)
		},
	}

	cmd.Flags().StringVar(&tableName, "table", "", "Lock a specific table only")
	return cmd
}

func runDBLock(serviceName, tableName string) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc, err := store.GetServiceByName(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("look up service %q: %w", serviceName, err)
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

	if err := registry.Connect(serviceName, cfg); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	conn, err := registry.Get(serviceName)
	if err != nil {
		return fmt.Errorf("get connector: %w", err)
	}

	if tableName != "" {
		// Lock a single table.
		table, err := conn.IntrospectTable(ctx, tableName)
		if err != nil {
			return fmt.Errorf("introspect table %q: %w", tableName, err)
		}
		if _, err := store.SaveContract(ctx, serviceName, tableName, *table); err != nil {
			return fmt.Errorf("save contract: %w", err)
		}
		fmt.Printf("Locked schema contract for %s/%s (%d columns)\n", serviceName, tableName, len(table.Columns))
		return nil
	}

	// Lock all tables.
	schema, err := conn.IntrospectSchema(ctx)
	if err != nil {
		return fmt.Errorf("introspect schema: %w", err)
	}

	count := 0
	for _, table := range schema.Tables {
		if _, err := store.SaveContract(ctx, serviceName, table.Name, table); err != nil {
			return fmt.Errorf("save contract for %s: %w", table.Name, err)
		}
		count++
	}

	fmt.Printf("Locked schema contracts for %d tables in %q\n", count, serviceName)
	return nil
}

func newDBUnlockCmd() *cobra.Command {
	var tableName string

	cmd := &cobra.Command{
		Use:   "unlock <service>",
		Short: "Remove schema contract locks",
		Long:  "Remove schema contract locks for a service or a specific table.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBUnlock(args[0], tableName)
		},
	}

	cmd.Flags().StringVar(&tableName, "table", "", "Unlock a specific table only")
	return cmd
}

func runDBUnlock(serviceName, tableName string) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	if tableName != "" {
		if err := store.DeleteContract(ctx, serviceName, tableName); err != nil {
			return fmt.Errorf("delete contract: %w", err)
		}
		fmt.Printf("Unlocked schema contract for %s/%s\n", serviceName, tableName)
		return nil
	}

	n, err := store.DeleteServiceContracts(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("delete contracts: %w", err)
	}
	fmt.Printf("Unlocked %d schema contracts for %q\n", n, serviceName)
	return nil
}

func newDBDiffCmd() *cobra.Command {
	var tableName string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "diff <service>",
		Short: "Show schema drift between locked contracts and live database",
		Long: `Compare locked schema contracts against the current live database schema.
Reports additive changes (safe) and breaking changes (would affect consumers).`,
		Example: `  faucet db diff mydb
  faucet db diff mydb --table users
  faucet db diff mydb --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBDiff(args[0], tableName, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&tableName, "table", "", "Diff a specific table only")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	return cmd
}

func runDBDiff(serviceName, tableName string, jsonOutput bool) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc, err := store.GetServiceByName(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("look up service %q: %w", serviceName, err)
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

	if err := registry.Connect(serviceName, cfg); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	conn, err := registry.Get(serviceName)
	if err != nil {
		return fmt.Errorf("get connector: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if tableName != "" {
		// Diff a single table.
		c, err := store.GetContract(ctx, serviceName, tableName)
		if err != nil {
			return fmt.Errorf("no contract found for %s/%s: %w", serviceName, tableName, err)
		}
		live, err := conn.IntrospectTable(ctx, tableName)
		if err != nil {
			return fmt.Errorf("introspect table %q: %w", tableName, err)
		}
		report := contract.DiffTable(serviceName, c.Schema, *live, c.LockedAt)

		if jsonOutput {
			return enc.Encode(report)
		}
		printDriftReport(report)
		return nil
	}

	// Diff all locked tables.
	contracts, err := store.ListContracts(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("list contracts: %w", err)
	}

	if len(contracts) == 0 {
		fmt.Printf("No schema contracts found for %q. Run 'faucet db lock %s' first.\n", serviceName, serviceName)
		return nil
	}

	schema, err := conn.IntrospectSchema(ctx)
	if err != nil {
		return fmt.Errorf("introspect schema: %w", err)
	}

	lockMode := contract.LockMode(svc.SchemaLock)
	report := contract.DiffSchema(serviceName, contracts, schema, lockMode)

	if jsonOutput {
		return enc.Encode(report)
	}
	printServiceDriftReport(report)
	return nil
}

func printDriftReport(r contract.DriftReport) {
	if !r.HasDrift {
		fmt.Printf("  %s: no drift\n", r.TableName)
		return
	}

	status := "DRIFT"
	if r.HasBreaking {
		status = "BREAKING"
	}
	fmt.Printf("  %s: %s (%d additive, %d breaking)\n", r.TableName, status, r.AdditiveCount, r.BreakingCount)
	for _, item := range r.Items {
		marker := "+"
		if item.Type == contract.DriftBreaking {
			marker = "!"
		}
		fmt.Printf("    %s %s\n", marker, item.Description)
	}
}

func printServiceDriftReport(r contract.ServiceDriftReport) {
	fmt.Printf("Schema Drift Report: %s (mode: %s)\n", r.ServiceName, r.LockMode)
	fmt.Printf("  %d tables locked, %d with drift, %d breaking changes\n\n", r.TotalTables, r.DriftedTables, r.BreakingCount)

	for _, t := range r.Tables {
		printDriftReport(t)
	}

	if r.DriftedTables == 0 {
		fmt.Println("  All tables match their locked contracts.")
	}
}

func newDBPromoteCmd() *cobra.Command {
	var tableName string
	var all bool

	cmd := &cobra.Command{
		Use:   "promote <service>",
		Short: "Promote schema contracts to match the current live schema",
		Long: `Update locked schema contracts to accept the current live database schema.
Use this after reviewing drift to acknowledge and accept schema changes.`,
		Example: `  faucet db promote mydb --table users   # Promote a single table
  faucet db promote mydb --all           # Promote all tables`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBPromote(args[0], tableName, all)
		},
	}

	cmd.Flags().StringVar(&tableName, "table", "", "Promote a specific table only")
	cmd.Flags().BoolVar(&all, "all", false, "Promote all locked tables")
	return cmd
}

func runDBPromote(serviceName, tableName string, all bool) error {
	if tableName == "" && !all {
		return fmt.Errorf("specify --table <name> or --all")
	}

	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	svc, err := store.GetServiceByName(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("look up service %q: %w", serviceName, err)
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

	if err := registry.Connect(serviceName, cfg); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	conn, err := registry.Get(serviceName)
	if err != nil {
		return fmt.Errorf("get connector: %w", err)
	}

	if tableName != "" {
		live, err := conn.IntrospectTable(ctx, tableName)
		if err != nil {
			return fmt.Errorf("introspect table %q: %w", tableName, err)
		}
		if err := store.PromoteContract(ctx, serviceName, tableName, *live); err != nil {
			return fmt.Errorf("promote contract: %w", err)
		}
		fmt.Printf("Promoted contract for %s/%s to current schema\n", serviceName, tableName)
		return nil
	}

	// Promote all.
	contracts, err := store.ListContracts(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("list contracts: %w", err)
	}

	promoted := 0
	for _, c := range contracts {
		live, err := conn.IntrospectTable(ctx, c.TableName)
		if err != nil {
			fmt.Printf("  Warning: could not introspect %s (table may have been dropped)\n", c.TableName)
			continue
		}
		if err := store.PromoteContract(ctx, serviceName, c.TableName, *live); err != nil {
			return fmt.Errorf("promote contract for %s: %w", c.TableName, err)
		}
		promoted++
	}

	fmt.Printf("Promoted %d contracts in %q to current schema\n", promoted, serviceName)
	return nil
}
