package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage admin users",
		Long:  "Create and list administrative users who can manage Faucet through the admin API.",
	}

	cmd.AddCommand(newAdminCreateCmd())
	cmd.AddCommand(newAdminListCmd())

	return cmd
}

// ---------- admin create ----------

func newAdminCreateCmd() *cobra.Command {
	var (
		email    string
		password string
		name     string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new admin user",
		Example: `  faucet admin create --email admin@example.com --password secret
  faucet admin create --email admin@example.com  # prompts for password`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminCreate(email, password, name)
		},
	}

	cmd.Flags().StringVar(&email, "email", "", "Admin email address (required)")
	cmd.Flags().StringVar(&password, "password", "", "Admin password (prompted if omitted)")
	cmd.Flags().StringVar(&name, "name", "", "Admin display name")
	cmd.MarkFlagRequired("email")

	return cmd
}

func runAdminCreate(email, password, name string) error {
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email address: %q", email)
	}

	// Prompt for password if not provided
	if password == "" {
		fmt.Print("Password: ")
		pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()
		password = string(pwBytes)

		fmt.Print("Confirm password: ")
		confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		fmt.Println()

		if password != string(confirmBytes) {
			return fmt.Errorf("passwords do not match")
		}
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	// TODO: open config store, hash password with bcrypt, create admin
	// store, _ := config.Open(...)
	// hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	// admin := model.Admin{Email: email, PasswordHash: string(hash), Name: name, IsActive: true}
	// err = store.CreateAdmin(admin)

	fmt.Printf("Created admin user %q\n", email)
	fmt.Println("  (placeholder: config store not yet wired)")
	return nil
}

// ---------- admin list ----------

func newAdminListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all admin users",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminList(jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func runAdminList(jsonOutput bool) error {
	// TODO: open config store, list admins
	// store, _ := config.Open(...)
	// admins, err := store.ListAdmins()

	type adminRow struct {
		Email  string `json:"email"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}

	admins := []adminRow{} // empty until config store is wired

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(admins)
	}

	if len(admins) == 0 {
		fmt.Println("No admin users configured. Use 'faucet admin create' to create one.")
		return nil
	}

	fmt.Printf("%-30s %-24s %-8s\n", "EMAIL", "NAME", "ACTIVE")
	fmt.Printf("%-30s %-24s %-8s\n", "-----", "----", "------")
	for _, a := range admins {
		active := "yes"
		if !a.Active {
			active = "no"
		}
		fmt.Printf("%-30s %-24s %-8s\n", a.Email, a.Name, active)
	}

	return nil
}
