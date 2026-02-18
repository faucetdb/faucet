package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "key",
		Aliases: []string{"apikey"},
		Short:   "Manage API keys",
		Long:    "Create, list, and revoke API keys used to authenticate against the Faucet REST API.",
	}

	cmd.AddCommand(newKeyCreateCmd())
	cmd.AddCommand(newKeyListCmd())
	cmd.AddCommand(newKeyRevokeCmd())

	return cmd
}

// ---------- key create ----------

func newKeyCreateCmd() *cobra.Command {
	var (
		role  string
		label string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new API key",
		Long:  "Generate a new API key bound to a role. The raw key is shown once and cannot be retrieved again.",
		Example: `  faucet key create --role readonly --label "CI pipeline"
  faucet key create --role admin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeyCreate(role, label)
		},
	}

	cmd.Flags().StringVar(&role, "role", "", "Role to bind the key to (required)")
	cmd.Flags().StringVar(&label, "label", "", "Human-readable label for the key")
	cmd.MarkFlagRequired("role")

	return cmd
}

func runKeyCreate(role, label string) error {
	// TODO: open config store, look up role by name, generate key, store hash
	// store, _ := config.Open(...)
	// roleObj, err := store.GetRoleByName(role)
	// rawKey := crypto.GenerateAPIKey()
	// hash := sha256hex(rawKey)
	// prefix := rawKey[:8]
	// apiKey := model.APIKey{KeyHash: hash, KeyPrefix: prefix, Label: label, RoleID: roleObj.ID, IsActive: true}
	// store.CreateAPIKey(apiKey)

	fmt.Println("API Key created:")
	fmt.Println()
	fmt.Printf("  Key:   faucet_%s... (placeholder)\n", "xxxxxxxxxxxx")
	fmt.Printf("  Role:  %s\n", role)
	if label != "" {
		fmt.Printf("  Label: %s\n", label)
	}
	fmt.Println()
	fmt.Println("  Save this key now - it cannot be retrieved again.")
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}

// ---------- key list ----------

func newKeyListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeyList(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runKeyList(jsonOutput bool) error {
	// TODO: open config store, list keys
	// store, _ := config.Open(...)
	// keys, err := store.ListAPIKeys()

	type keyRow struct {
		Prefix string `json:"prefix"`
		Role   string `json:"role"`
		Label  string `json:"label"`
		Active bool   `json:"active"`
	}

	keys := []keyRow{} // empty until config store is wired

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(keys)
	}

	if len(keys) == 0 {
		fmt.Println("No API keys configured. Use 'faucet key create' to create one.")
		return nil
	}

	fmt.Printf("%-12s %-16s %-24s %-8s\n", "PREFIX", "ROLE", "LABEL", "ACTIVE")
	fmt.Printf("%-12s %-16s %-24s %-8s\n", "------", "----", "-----", "------")
	for _, k := range keys {
		active := "yes"
		if !k.Active {
			active = "no"
		}
		fmt.Printf("%-12s %-16s %-24s %-8s\n", k.Prefix, k.Role, k.Label, active)
	}

	return nil
}

// ---------- key revoke ----------

func newKeyRevokeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke <prefix>",
		Short: "Revoke an API key by its prefix",
		Long:  "Deactivate an API key, preventing any further authenticated requests using that key.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeyRevoke(args[0])
		},
	}

	return cmd
}

func runKeyRevoke(prefix string) error {
	// TODO: open config store, find key by prefix, set is_active = false
	// store, _ := config.Open(...)
	// err := store.RevokeAPIKey(prefix)

	fmt.Printf("Revoked API key with prefix %q\n", prefix)
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}
