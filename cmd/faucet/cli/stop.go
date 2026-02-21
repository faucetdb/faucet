package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the background Faucet server",
		Long:  "Stop a Faucet server that was started with 'faucet serve'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop()
		},
	}
}

func runStop() error {
	pid, err := readPID()
	if err != nil {
		return fmt.Errorf("no running server found (missing PID file at %s)", pidFilePath())
	}

	if !isProcessRunning(pid) {
		removePID()
		return fmt.Errorf("server (PID %d) is not running (stale PID file removed)", pid)
	}

	fmt.Printf("Stopping Faucet server (PID %d)...\n", pid)

	if err := stopProcess(pid); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 50; i++ { // up to 5 seconds
		time.Sleep(100 * time.Millisecond)
		if !isProcessRunning(pid) {
			removePID()
			fmt.Println("Server stopped.")
			return nil
		}
	}

	return fmt.Errorf("server (PID %d) did not stop within 5 seconds — it may still be draining connections", pid)
}
