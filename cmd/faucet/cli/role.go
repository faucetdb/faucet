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
		access      []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new role",
		Long: `Create a role. A role grants nothing until it has access rules, so pass
--access one or more times as SERVICE:COMPONENT:VERBS, where SERVICE and
COMPONENT may be * and VERBS is a comma-separated list of GET, POST, PUT,
PATCH, DELETE or * for all.`,
		Example: `  faucet role create --name readonly --access "*:*:GET"
  faucet role create --name orders --access "mydb:_table/orders:GET,POST,PATCH"
  faucet role create --name admin --access "*:*:*"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleCreate(name, description, access)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Role name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Role description")
	cmd.Flags().StringArrayVar(&access, "access", nil, "Access rule SERVICE:COMPONENT:VERBS (repeatable)")
	cmd.MarkFlagRequired("name")

	return cmd
}

func runRoleCreate(name, description string, accessSpecs []string) error {
	rules := make([]model.RoleAccess, 0, len(accessSpecs))
	for _, spec := range accessSpecs {
		rule, err := parseAccessSpec(spec)
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}

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
	if len(rules) > 0 {
		if err := store.SetRoleAccess(ctx, role.ID, rules); err != nil {
			return fmt.Errorf("set role access: %w", err)
		}
	}

	fmt.Printf("Created role %q (id=%d)\n", name, role.ID)
	if description != "" {
		fmt.Printf("  description: %s\n", description)
	}
	for _, r := range rules {
		fmt.Printf("  access: %s:%s:%s\n", r.ServiceName, r.Component, verbNames(r.VerbMask))
	}
	if len(rules) == 0 {
		fmt.Println("  warning: no access rules; keys bound to this role will be denied until rules are added (--access or the admin UI)")
	}
	return nil
}

// parseAccessSpec parses SERVICE:COMPONENT:VERBS into an access rule.
func parseAccessSpec(spec string) (model.RoleAccess, error) {
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return model.RoleAccess{}, fmt.Errorf("invalid --access %q: want SERVICE:COMPONENT:VERBS, e.g. \"*:_table/*:GET\"", spec)
	}
	mask := 0
	for _, v := range strings.Split(parts[2], ",") {
		v = strings.ToUpper(strings.TrimSpace(v))
		if v == "*" {
			mask = model.VerbAll
			continue
		}
		bit := model.VerbFromMethod(v)
		if bit == 0 {
			return model.RoleAccess{}, fmt.Errorf("invalid --access %q: unknown verb %q (use GET, POST, PUT, PATCH, DELETE or *)", spec, v)
		}
		mask |= bit
	}
	return model.RoleAccess{
		ServiceName: parts[0],
		Component:   parts[1],
		VerbMask:    mask,
		// Defaults mirror the role_access column defaults used by the API.
		RequestorMask: model.RequestorAPI,
		Filters:       []model.Filter{},
		FilterOp:      "AND",
	}, nil
}

// verbNames renders a verb mask as a comma-separated method list.
func verbNames(mask int) string {
	if mask == model.VerbAll {
		return "*"
	}
	var names []string
	for _, v := range []int{model.VerbGet, model.VerbPost, model.VerbPut, model.VerbPatch, model.VerbDelete} {
		if mask&v != 0 {
			names = append(names, model.VerbName(v))
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
}
