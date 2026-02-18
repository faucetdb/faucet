package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/connector"
	"github.com/faucetdb/faucet/internal/openapi"
)

func newOpenAPICmd() *cobra.Command {
	var (
		all        bool
		outputFile string
		baseURL    string
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
			return runOpenAPI(serviceName, all, outputFile, baseURL)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Generate combined spec for all services")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write spec to file instead of stdout")
	cmd.Flags().StringVar(&baseURL, "base-url", "http://localhost:8080", "Base URL for the API server")

	return cmd
}

func runOpenAPI(serviceName string, all bool, outputFile, baseURL string) error {
	if serviceName == "" && !all {
		return fmt.Errorf("specify a service name or use --all")
	}

	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()
	registry := newRegistry()
	defer registry.CloseAll()

	var specJSON []byte

	if all {
		services, err := store.ListServices(ctx)
		if err != nil {
			return fmt.Errorf("list services: %w", err)
		}
		if len(services) == 0 {
			return fmt.Errorf("no services configured")
		}

		var serviceSpecs []openapi.ServiceSpec
		for _, svc := range services {
			if !svc.IsActive {
				continue
			}

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
			if err := registry.Connect(svc.Name, cfg); err != nil {
				return fmt.Errorf("connect service %q: %w", svc.Name, err)
			}

			conn, err := registry.Get(svc.Name)
			if err != nil {
				return fmt.Errorf("get connector for %q: %w", svc.Name, err)
			}

			schema, err := conn.IntrospectSchema(ctx)
			if err != nil {
				return fmt.Errorf("introspect schema for %q: %w", svc.Name, err)
			}

			serviceSpecs = append(serviceSpecs, openapi.ServiceSpec{
				Name:   svc.Name,
				Label:  svc.Label,
				Driver: svc.Driver,
				Schema: schema,
			})
		}

		doc := openapi.GenerateCombinedSpec(serviceSpecs, baseURL)
		specJSON, err = json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal openapi spec: %w", err)
		}
	} else {
		svc, err := store.GetServiceByName(ctx, serviceName)
		if err != nil {
			return fmt.Errorf("look up service %q: %w", serviceName, err)
		}

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
		if err := registry.Connect(svc.Name, cfg); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		conn, err := registry.Get(svc.Name)
		if err != nil {
			return fmt.Errorf("get connector: %w", err)
		}

		schema, err := conn.IntrospectSchema(ctx)
		if err != nil {
			return fmt.Errorf("introspect schema: %w", err)
		}

		doc := openapi.GenerateServiceSpec(svc.Name, svc.Label, svc.Driver, baseURL, schema)
		specJSON, err = json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal openapi spec: %w", err)
		}
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, specJSON, 0644); err != nil {
			return fmt.Errorf("write file %q: %w", outputFile, err)
		}
		fmt.Printf("OpenAPI spec written to %s\n", outputFile)
	} else {
		fmt.Println(string(specJSON))
	}

	return nil
}
