package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newOpenAPICmd() *cobra.Command {
	var (
		all        bool
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "openapi [service]",
		Short: "Generate OpenAPI specification",
		Long: `Generate an OpenAPI 3.1 specification for one or all database services.
The spec includes all tables, columns, relationships, and supported operations.`,
		Example: `  faucet openapi mydb              # spec for a single service
  faucet openapi --all             # combined spec for all services
  faucet openapi mydb -o spec.json # write to file`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := ""
			if len(args) > 0 {
				serviceName = args[0]
			}
			return runOpenAPI(serviceName, all, outputFile)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Generate combined spec for all services")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write spec to file instead of stdout")

	return cmd
}

func runOpenAPI(serviceName string, all bool, outputFile string) error {
	if serviceName == "" && !all {
		return fmt.Errorf("specify a service name or use --all")
	}

	// TODO: open config store, connect to services, introspect schemas, generate OpenAPI
	// store, _ := config.Open(...)
	// registry := connector.NewRegistry()
	// registerDrivers(registry)
	//
	// if all {
	//     services, _ := store.ListServices()
	//     for _, svc := range services { ... connect and introspect ... }
	//     spec := openapi.GenerateCombined(schemas)
	// } else {
	//     svc, _ := store.GetService(serviceName)
	//     registry.Connect(serviceName, ...)
	//     conn, _ := registry.Get(serviceName)
	//     schema, _ := conn.IntrospectSchema(context.Background())
	//     spec := openapi.Generate(serviceName, schema)
	// }
	//
	// jsonBytes, _ := json.MarshalIndent(spec, "", "  ")
	// if outputFile != "" {
	//     os.WriteFile(outputFile, jsonBytes, 0644)
	// } else {
	//     fmt.Println(string(jsonBytes))
	// }

	if all {
		fmt.Println("Generating combined OpenAPI spec for all services...")
	} else {
		fmt.Printf("Generating OpenAPI spec for service %q...\n", serviceName)
	}
	if outputFile != "" {
		fmt.Printf("  output: %s\n", outputFile)
	}
	fmt.Println("  (placeholder: OpenAPI generator not yet wired)")
	return nil
}
