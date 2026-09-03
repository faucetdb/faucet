package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/model"
	"github.com/faucetdb/faucet/internal/rbac"
)

func newRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage RBAC roles",
		Long: `Create, list, and grant access to roles that define what API keys are allowed to do.

A role is a list of access rules {service, component, verbs}. API keys inherit
the rules of the role they are bound to. A role with no rules denies every
request (fail closed), so grant at least one rule with --verbs on create or
with "faucet role grant".`,
	}

	cmd.AddCommand(newRoleListCmd())
	cmd.AddCommand(newRoleCreateCmd())
	cmd.AddCommand(newRoleGrantCmd())

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

// maxAccessSummaryLen is the display width after which formatAccessSummary
// truncates the rule list.
const maxAccessSummaryLen = 40

// formatAccessSummary returns a short summary of access rules for display,
// e.g. "*:GET" or "mydb/_table/*:GET,POST; *:GET". Long summaries are
// truncated with "...".
func formatAccessSummary(access []model.RoleAccess) string {
	if len(access) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(access))
	for _, a := range access {
		parts = append(parts, formatAccessRule(a))
	}

	summary := strings.Join(parts, "; ")
	if len(summary) > maxAccessSummaryLen {
		summary = summary[:maxAccessSummaryLen-3] + "..."
	}
	return summary
}

// formatAccessRule renders a single rule as "service/component:VERBS".
// The "/component" part is omitted when the component is a wildcard so
// that the common "all services, all components" rule reads as "*:GET".
func formatAccessRule(a model.RoleAccess) string {
	service := strings.TrimSpace(a.ServiceName)
	if service == "" {
		service = rbac.Wildcard
	}
	component := strings.Trim(strings.TrimSpace(a.Component), "/")
	if component == "" || component == rbac.Wildcard {
		return service + ":" + rbac.VerbName(a.VerbMask)
	}
	return service + "/" + component + ":" + rbac.VerbName(a.VerbMask)
}

// ---------- role create ----------

func newRoleCreateCmd() *cobra.Command {
	var (
		name        string
		description string
		serviceName string
		component   string
		verbs       string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new role",
		Long: `Create a new role. Use --verbs to grant an initial access rule; without it
the role has no rules and API keys bound to it are denied (403) until you
run "faucet role grant".`,
		Example: `  faucet role create --name readonly --verbs GET --description "Read-only access to all services"
  faucet role create --name admin --verbs '*' --description "Full access"
  faucet role create --name orders-writer --service mydb --component "_table/orders" --verbs GET,POST,PATCH`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleCreate(name, description, serviceName, component, verbs, cmd.Flags().Changed("verbs"))
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Role name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Role description")
	cmd.Flags().StringVar(&serviceName, "service", rbac.Wildcard, "Service the initial rule applies to (\"*\", exact name, or \"prefix*\")")
	cmd.Flags().StringVar(&component, "component", rbac.Wildcard, "Component the initial rule applies to (\"*\", \"_table/customers\", \"_table/*\", ...)")
	cmd.Flags().StringVar(&verbs, "verbs", "", "Comma-separated verbs to grant (GET,POST,PUT,PATCH,DELETE) or \"*\" for all")
	cmd.MarkFlagRequired("name")

	return cmd
}

func runRoleCreate(name, description, serviceName, component, verbs string, verbsSet bool) error {
	var rule *model.RoleAccess
	if verbsSet {
		r, err := parseAccessRule(serviceName, component, verbs)
		if err != nil {
			return err
		}
		rule = &r
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

	fmt.Printf("Created role %q (id=%d)\n", name, role.ID)
	if description != "" {
		fmt.Printf("  description: %s\n", description)
	}

	if rule == nil {
		fmt.Fprintf(os.Stderr, "Warning: role %q has no access rules, so API keys bound to it will be denied (403).\n", name)
		fmt.Fprintf(os.Stderr, "Grant access with: faucet role grant --role %s --verbs GET   (or use the admin UI / PUT /api/v1/system/role/{id})\n", name)
		return nil
	}

	if err := store.SetRoleAccess(ctx, role.ID, []model.RoleAccess{*rule}); err != nil {
		return fmt.Errorf("set role access: %w", err)
	}
	fmt.Printf("  access: %s\n", describeAccessRule(*rule))
	return nil
}

// ---------- role grant ----------

func newRoleGrantCmd() *cobra.Command {
	var (
		roleName    string
		serviceName string
		component   string
		verbs       string
	)

	cmd := &cobra.Command{
		Use:   "grant",
		Short: "Add an access rule to an existing role",
		Long: `Append an access rule {service, component, verbs} to a role. Existing rules
are kept; the new rule is added to them.`,
		Example: `  faucet role grant --role readonly --verbs GET
  faucet role grant --role readonly --service mydb --component "_table/orders" --verbs GET,POST
  faucet role grant --role admin --verbs '*'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleGrant(roleName, serviceName, component, verbs)
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "", "Role name (required)")
	cmd.Flags().StringVar(&serviceName, "service", rbac.Wildcard, "Service the rule applies to (\"*\", exact name, or \"prefix*\")")
	cmd.Flags().StringVar(&component, "component", rbac.Wildcard, "Component the rule applies to (\"*\", \"_table/customers\", \"_table/*\", ...)")
	cmd.Flags().StringVar(&verbs, "verbs", "", "Comma-separated verbs to grant (GET,POST,PUT,PATCH,DELETE) or \"*\" for all (required)")
	cmd.MarkFlagRequired("role")
	cmd.MarkFlagRequired("verbs")

	return cmd
}

func runRoleGrant(roleName, serviceName, component, verbs string) error {
	rule, err := parseAccessRule(serviceName, component, verbs)
	if err != nil {
		return err
	}

	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	role, err := store.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("get role %q: %w", roleName, err)
	}

	access := append(role.Access, rule)
	if err := store.SetRoleAccess(ctx, role.ID, access); err != nil {
		return fmt.Errorf("set role access: %w", err)
	}

	fmt.Printf("Granted %s to role %q (id=%d)\n", describeAccessRule(rule), role.Name, role.ID)
	fmt.Printf("  rules: %d\n", len(access))
	for _, a := range access {
		fmt.Printf("    %s\n", describeAccessRule(a))
	}
	if !role.IsActive {
		fmt.Fprintf(os.Stderr, "Warning: role %q is inactive; API keys bound to it are denied until it is activated.\n", role.Name)
	}
	return nil
}

// ---------- helpers ----------

// parseAccessRule builds an API-requestor access rule from CLI flag values.
// The verbs string is validated with rbac.ParseVerbs before anything is
// written, and a mask of zero is rejected so that a rule can never be
// created that grants nothing.
func parseAccessRule(serviceName, component, verbs string) (model.RoleAccess, error) {
	mask, err := rbac.ParseVerbs(verbs)
	if err != nil {
		return model.RoleAccess{}, fmt.Errorf("invalid --verbs: %w", err)
	}
	if mask == 0 {
		return model.RoleAccess{}, fmt.Errorf("invalid --verbs %q: at least one verb is required (GET, POST, PUT, PATCH, DELETE or *)", verbs)
	}

	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = rbac.Wildcard
	}
	component = strings.Trim(strings.TrimSpace(component), "/")
	if component == "" {
		component = rbac.Wildcard
	}

	return model.RoleAccess{
		ServiceName:   serviceName,
		Component:     component,
		VerbMask:      mask,
		RequestorMask: model.RequestorAPI,
		Filters:       []model.Filter{},
		FilterOp:      "AND",
	}, nil
}

// describeAccessRule renders a rule in the human-readable form
// "GET,POST on mydb/_table/orders".
func describeAccessRule(a model.RoleAccess) string {
	service := a.ServiceName
	if service == "" {
		service = rbac.Wildcard
	}
	component := a.Component
	if component == "" {
		component = rbac.Wildcard
	}
	return fmt.Sprintf("%s on %s/%s", rbac.VerbName(a.VerbMask), service, component)
}
