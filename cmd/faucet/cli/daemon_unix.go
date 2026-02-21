//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// setSysProcAttr configures the child process to run in a new session,
// detached from the parent's terminal.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// isProcessRunning checks whether a process with the given PID is alive.
func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// stopProcess sends SIGTERM to the process for graceful shutdown.
func stopProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
