package cli

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check if the Faucet server is running",
		Long:  "Check the status of the Faucet server, including process state and HTTP health.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	pid, err := readPID()
	if err != nil {
		fmt.Println("Server is not running (no PID file found).")
		return nil
	}

	if !isProcessRunning(pid) {
		removePID()
		fmt.Println("Server is not running (stale PID file removed).")
		return nil
	}

	// Server process is alive — check HTTP health
	port := viper.GetInt("server.port")
	if port == 0 {
		port = 8080
	}
	host := viper.GetString("server.host")
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	healthAddr := fmt.Sprintf("http://%s:%d/healthz", host, port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthAddr)

	if err != nil {
		fmt.Printf("Server process is running (PID %d) but not responding to HTTP.\n", pid)
		fmt.Printf("  Logs: %s\n", logFilePath())
		return nil
	}
	resp.Body.Close()

	fmt.Printf("Server is running (PID %d)\n", pid)
	fmt.Printf("  Health:  %s (%d)\n", healthAddr, resp.StatusCode)
	fmt.Printf("  Logs:    %s\n", logFilePath())
	return nil
}
