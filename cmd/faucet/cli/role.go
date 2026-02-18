package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	// TODO: open config store, list roles
	// store, _ := config.Open(...)
	// roles, err := store.ListRoles()

	type roleRow struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Active      bool   `json:"active"`
	}

	roles := []roleRow{} // empty until config store is wired

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(roles)
	}

	if len(roles) == 0 {
		fmt.Println("No roles configured. Use 'faucet role create' to create one.")
		return nil
	}

	fmt.Printf("%-20s %-40s %-8s\n", "NAME", "DESCRIPTION", "ACTIVE")
	fmt.Printf("%-20s %-40s %-8s\n", "----", "-----------", "------")
	for _, r := range roles {
		active := "yes"
		if !r.Active {
			active = "no"
		}
		fmt.Printf("%-20s %-40s %-8s\n", r.Name, r.Description, active)
	}

	return nil
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
	// TODO: open config store, create role
	// store, _ := config.Open(...)
	// role := model.Role{Name: name, Description: description, IsActive: true}
	// err := store.CreateRole(role)

	fmt.Printf("Created role %q\n", name)
	if description != "" {
		fmt.Printf("  description: %s\n", description)
	}
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}
