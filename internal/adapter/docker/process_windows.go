//go:build windows

package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// configureProcess makes cancellation ask Windows to terminate the Docker CLI
// and its descendants. Docker Desktop's CLI may launch the Compose plugin as a
// separate process, so killing only cmd.Process can leave that plugin running.
func configureProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}

		// taskkill /T is scoped to this exact root PID and follows its current
		// descendant tree. If taskkill is unavailable or fails, still stop the
		// command itself rather than leave the request running indefinitely.
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = os.Getenv("WINDIR")
		}
		if systemRoot == "" {
			killErr := cmd.Process.Kill()
			return fmt.Errorf("locate taskkill: SystemRoot and WINDIR are unset; direct process kill error: %v", killErr)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		taskkill := filepath.Join(systemRoot, "System32", "taskkill.exe")
		output, treeErr := exec.CommandContext(ctx, taskkill, "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").CombinedOutput()
		if treeErr == nil {
			return nil
		}
		killErr := cmd.Process.Kill()
		if killErr == nil || errors.Is(killErr, os.ErrProcessDone) {
			return fmt.Errorf("taskkill could not terminate process tree: %w: %s", treeErr, strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("taskkill could not terminate process tree: %w: %s; direct process kill failed: %v", treeErr, strings.TrimSpace(string(output)), killErr)
	}
}
