package cli

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/connector/mssql"
	"github.com/faucetdb/faucet/internal/connector/mysql"
	"github.com/faucetdb/faucet/internal/connector/postgres"
	"github.com/faucetdb/faucet/internal/connector/snowflake"
)

func newBenchmarkCmd() *cobra.Command {
	var (
		driver      string
		dsn         string
		duration    time.Duration
		concurrency int
		table       string
	)

	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark database query throughput",
		Long: `Run a load test against a database to measure query throughput and latency.
Executes concurrent SELECT queries against a specified table for the given duration.`,
		Example: `  faucet benchmark --driver postgres --dsn "postgres://localhost/mydb" --duration 30s --concurrency 50
  faucet benchmark --driver mysql --dsn "user:pass@tcp(localhost)/mydb" --table users
  faucet benchmark --driver mssql --dsn "sqlserver://user:pass@localhost?database=mydb"
  faucet benchmark --driver snowflake --dsn "user:pass@account/db/schema"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBenchmark(driver, dsn, duration, concurrency, table)
		},
	}

	cmd.Flags().StringVar(&driver, "driver", "postgres", "Database driver (postgres, mysql, mssql, snowflake)")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Connection string (required)")
	cmd.Flags().DurationVar(&duration, "duration", 30*time.Second, "Test duration")
	cmd.Flags().IntVar(&concurrency, "concurrency", 10, "Number of concurrent workers")
	cmd.Flags().StringVar(&table, "table", "", "Table to query (auto-detected if omitted)")
	cmd.MarkFlagRequired("dsn")

	return cmd
}

// sanitizeDSN redacts passwords from DSN strings for display purposes.
func sanitizeDSN(dsn string) string {
	// Mask anything between "://" user:PASSWORD@ patterns
	if idx := strings.Index(dsn, "://"); idx != -1 {
		rest := dsn[idx+3:]
		if atIdx := strings.Index(rest, "@"); atIdx != -1 {
			if colonIdx := strings.Index(rest[:atIdx], ":"); colonIdx != -1 {
				return dsn[:idx+3] + rest[:colonIdx] + ":****@" + rest[atIdx+1:]
			}
		}
	}
	return dsn
}

// printBanner prints the ASCII art banner and benchmark configuration.
func printBanner(driver, dsn string, duration time.Duration, concurrency int) {
	banner := `
 _____ _   _   _  ___ ___ _____
|  ___/ \ | | | |/ __| __|_   _|
| |_ / _ \| |_| | (__|  _| | |
|_| /_/ \_\___,_|\___|___| |_|
`
	fmt.Print(banner)
	fmt.Println("Faucet Benchmark Suite")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Target: %s @ %s\n", driver, sanitizeDSN(dsn))
	fmt.Printf("Duration: %s | Concurrency: %d\n", duration, concurrency)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// memStats captures a snapshot of memory statistics for reporting.
type memStats struct {
	HeapAlloc uint64
	Sys       uint64
}

func captureMemStats() memStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memStats{HeapAlloc: m.HeapAlloc, Sys: m.Sys}
}

func formatBytes(b uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func runBenchmark(driver, dsn string, duration time.Duration, concurrency int, table string) error {
	printBanner(driver, dsn, duration, concurrency)

	memBefore := captureMemStats()

	// Create connector based on driver
	var conn connector.Connector
	switch driver {
	case "postgres":
		conn = postgres.New()
	case "mysql":
		conn = mysql.New()
	case "mssql":
		conn = mssql.New()
	case "snowflake":
		conn = snowflake.New()
	default:
		return fmt.Errorf("unsupported driver %q (supported: postgres, mysql, mssql, snowflake)", driver)
	}

	// Connect
	fmt.Print("Connecting... ")
	cfg := connector.ConnectionConfig{
		Driver:       driver,
		DSN:          dsn,
		MaxOpenConns: concurrency + 5,
		MaxIdleConns: concurrency,
	}
	if err := conn.Connect(cfg); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Disconnect()

	ctx := context.Background()
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	fmt.Println("ok")

	// Schema introspection benchmark
	fmt.Print("Introspecting schema... ")
	introStart := time.Now()
	schema, err := conn.IntrospectSchema(ctx)
	introDuration := time.Since(introStart)
	if err != nil {
		return fmt.Errorf("schema introspection failed: %w", err)
	}
	fmt.Printf("done (%s, %d tables)\n", introDuration, len(schema.Tables))

	// Auto-detect table if not specified
	if table == "" {
		fmt.Print("Detecting tables... ")
		tables, err := conn.GetTableNames(ctx)
		if err != nil {
			return fmt.Errorf("failed to get tables: %w", err)
		}
		if len(tables) == 0 {
			return fmt.Errorf("no tables found in database")
		}
		table = tables[0]
		fmt.Printf("using %q\n", table)
	}

	// Build the benchmark query
	selectReq := connector.SelectRequest{
		Table: table,
		Limit: 10,
	}
	query, queryArgs, err := conn.BuildSelect(ctx, selectReq)
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	fmt.Printf("  query: %s\n", query)
	fmt.Println()
	fmt.Println("Running benchmark...")
	fmt.Println()

	// Run the benchmark
	var (
		totalQueries atomic.Int64
		totalErrors  atomic.Int64
		latencies    = make([]time.Duration, 0, 100000)
		latencyMu    sync.Mutex
	)

	deadline := time.Now().Add(duration)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db := conn.DB()
			for time.Now().Before(deadline) {
				start := time.Now()
				rows, err := db.QueryContext(ctx, query, queryArgs...)
				elapsed := time.Since(start)

				if err != nil {
					totalErrors.Add(1)
					continue
				}
				rows.Close()

				totalQueries.Add(1)
				latencyMu.Lock()
				latencies = append(latencies, elapsed)
				latencyMu.Unlock()
			}
		}()
	}

	wg.Wait()

	memAfter := captureMemStats()

	// Calculate results
	total := totalQueries.Load()
	errors := totalErrors.Load()
	qps := float64(total) / duration.Seconds()

	fmt.Println("Results")
	fmt.Println("-------")
	fmt.Printf("  Total queries:  %d\n", total)
	fmt.Printf("  Errors:         %d\n", errors)
	fmt.Printf("  QPS:            %.1f\n", qps)

	if len(latencies) > 0 {
		// Sort latencies for percentile calculation using sort.Slice
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})
		fmt.Printf("  Latency p50:    %s\n", latencies[len(latencies)*50/100])
		fmt.Printf("  Latency p95:    %s\n", latencies[len(latencies)*95/100])
		fmt.Printf("  Latency p99:    %s\n", latencies[len(latencies)*99/100])
		fmt.Printf("  Latency max:    %s\n", latencies[len(latencies)-1])
	}

	fmt.Println()
	fmt.Println("Schema Introspection")
	fmt.Println("--------------------")
	fmt.Printf("  Duration:       %s\n", introDuration)
	fmt.Printf("  Tables found:   %d\n", len(schema.Tables))

	fmt.Println()
	fmt.Println("Memory")
	fmt.Println("------")
	fmt.Printf("  Heap before:    %s\n", formatBytes(memBefore.HeapAlloc))
	fmt.Printf("  Heap after:     %s\n", formatBytes(memAfter.HeapAlloc))
	fmt.Printf("  RSS (sys) before: %s\n", formatBytes(memBefore.Sys))
	fmt.Printf("  RSS (sys) after:  %s\n", formatBytes(memAfter.Sys))

	return nil
}
