package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/model"
)

func newRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage RBAC roles",
		Long:  "Create and list roles that define what API keys are allowed to do.",
	}

	cmd.AddCommand(newRoleListCmd())
	cmd.AddCommand(newRoleCreateCmd())

	return cmd
}

// ---------- role list ----------

func newRoleListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all roles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleList(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runRoleList(jsonOutput bool) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()
	roles, err := store.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}

	if jsonOutput {
		type roleRow struct {
			Name        string             `json:"name"`
			Description string             `json:"description"`
			Active      bool               `json:"active"`
			Access      []model.RoleAccess `json:"access"`
		}
		rows := make([]roleRow, len(roles))
		for i, r := range roles {
			rows[i] = roleRow{
				Name:        r.Name,
				Description: r.Description,
				Active:      r.IsActive,
				Access:      r.Access,
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(roles) == 0 {
		fmt.Println("No roles configured. Use 'faucet role create' to create one.")
		return nil
	}

	fmt.Printf("%-20s %-40s %-8s %-8s\n", "NAME", "DESCRIPTION", "ACTIVE", "RULES")
	fmt.Printf("%-20s %-40s %-8s %-8s\n", "----", "-----------", "------", "-----")
	for _, r := range roles {
		active := "yes"
		if !r.IsActive {
			active = "no"
		}
		desc := r.Description
		if len(desc) > 38 {
			desc = desc[:35] + "..."
		}
		fmt.Printf("%-20s %-40s %-8s %-8s\n", r.Name, desc, active, formatAccessSummary(r.Access))
	}

	return nil
}

// formatAccessSummary returns a short summary of access rules for display.
func formatAccessSummary(access []model.RoleAccess) string {
	if len(access) == 0 {
		return "none"
	}

	// Collect unique services
	services := make(map[string]bool)
	for _, a := range access {
		if a.ServiceName != "" {
			services[a.ServiceName] = true
		}
	}

	parts := make([]string, 0, len(services))
	for s := range services {
		parts = append(parts, s)
	}

	if len(parts) == 0 {
		return fmt.Sprintf("%d rule(s)", len(access))
	}
	return fmt.Sprintf("%d rule(s): %s", len(access), strings.Join(parts, ", "))
}

// ---------- role create ----------

func newRoleCreateCmd() *cobra.Command {
	var (
		name        string
		description string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new role",
		Example: `  faucet role create --name readonly --description "Read-only access to all services"
  faucet role create --name admin --description "Full access"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleCreate(name, description)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Role name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Role description")
	cmd.MarkFlagRequired("name")

	return cmd
}

func runRoleCreate(name, description string) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	role := &model.Role{
		Name:        name,
		Description: description,
		IsActive:    true,
	}

	if err := store.CreateRole(ctx, role); err != nil {
		return fmt.Errorf("create role: %w", err)
	}

	fmt.Printf("Created role %q (id=%d)\n", name, role.ID)
	if description != "" {
		fmt.Printf("  description: %s\n", description)
	}
	return nil
}
