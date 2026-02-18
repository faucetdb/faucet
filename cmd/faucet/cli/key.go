package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/faucetdb/faucet/internal/config"
	"github.com/faucetdb/faucet/internal/model"
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

func runKeyCreate(roleName, label string) error {
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Look up role by name (iterate all roles since store has no GetRoleByName)
	roles, err := store.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}

	var matchedRole *model.Role
	for i := range roles {
		if roles[i].Name == roleName {
			matchedRole = &roles[i]
			break
		}
	}
	if matchedRole == nil {
		return fmt.Errorf("role %q not found", roleName)
	}

	// Generate 32 random bytes, hex encode, prefix with "faucet_"
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Errorf("generate random key: %w", err)
	}
	rawKey := "faucet_" + hex.EncodeToString(randomBytes)

	// Hash the key for storage
	keyHash := config.HashAPIKey(rawKey)

	// Use first 15 chars as prefix (faucet_ + 8 hex chars)
	keyPrefix := rawKey[:15]

	apiKey := &model.APIKey{
		KeyHash:  keyHash,
		KeyPrefix: keyPrefix,
		Label:    label,
		RoleID:   matchedRole.ID,
		IsActive: true,
	}

	if err := store.CreateAPIKey(ctx, apiKey); err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	fmt.Println("API Key created:")
	fmt.Println()
	fmt.Printf("  Key:   %s\n", rawKey)
	fmt.Printf("  Role:  %s\n", roleName)
	if label != "" {
		fmt.Printf("  Label: %s\n", label)
	}
	fmt.Println()
	fmt.Println("  Save this key now - it cannot be retrieved again.")
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
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	keys, err := store.ListAPIKeys(ctx)
	if err != nil {
		return fmt.Errorf("list api keys: %w", err)
	}

	// Build a role ID -> name map for display
	roles, err := store.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}
	roleNames := make(map[int64]string, len(roles))
	for _, r := range roles {
		roleNames[r.ID] = r.Name
	}

	type keyRow struct {
		Prefix string `json:"prefix"`
		Role   string `json:"role"`
		Label  string `json:"label"`
		Active bool   `json:"active"`
	}

	rows := make([]keyRow, len(keys))
	for i, k := range keys {
		rn := roleNames[k.RoleID]
		if rn == "" {
			rn = fmt.Sprintf("role:%d", k.RoleID)
		}
		rows[i] = keyRow{
			Prefix: k.KeyPrefix,
			Role:   rn,
			Label:  k.Label,
			Active: k.IsActive,
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(rows) == 0 {
		fmt.Println("No API keys configured. Use 'faucet key create' to create one.")
		return nil
	}

	fmt.Printf("%-16s %-16s %-24s %-8s\n", "PREFIX", "ROLE", "LABEL", "ACTIVE")
	fmt.Printf("%-16s %-16s %-24s %-8s\n", "------", "----", "-----", "------")
	for _, k := range rows {
		active := "yes"
		if !k.Active {
			active = "no"
		}
		fmt.Printf("%-16s %-16s %-24s %-8s\n", k.Prefix, k.Role, k.Label, active)
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
	store, err := openConfigStore()
	if err != nil {
		return fmt.Errorf("open config store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	keys, err := store.ListAPIKeys(ctx)
	if err != nil {
		return fmt.Errorf("list api keys: %w", err)
	}

	// Find key whose prefix starts with the given prefix
	var matchedKey *model.APIKey
	for i := range keys {
		if strings.HasPrefix(keys[i].KeyPrefix, prefix) || keys[i].KeyPrefix == prefix {
			matchedKey = &keys[i]
			break
		}
	}
	if matchedKey == nil {
		return fmt.Errorf("no API key found with prefix %q", prefix)
	}

	if err := store.RevokeAPIKey(ctx, matchedKey.ID); err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}

	fmt.Printf("Revoked API key with prefix %q\n", matchedKey.KeyPrefix)
	return nil
}
