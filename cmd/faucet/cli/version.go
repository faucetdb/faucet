package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd(version, commit, date string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := map[string]string{
				"version":    version,
				"commit":     commit,
				"built":      date,
				"go_version": runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
			}

			if jsonOutput {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "faucet %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "  commit:  %s\n", commit)
			fmt.Fprintf(cmd.OutOrStdout(), "  built:   %s\n", date)
			fmt.Fprintf(cmd.OutOrStdout(), "  go:      %s\n", runtime.Version())
			fmt.Fprintf(cmd.OutOrStdout(), "  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output version info as JSON")

	return cmd
}
