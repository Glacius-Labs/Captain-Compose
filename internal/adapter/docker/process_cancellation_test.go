//go:build darwin || linux || windows

package docker

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	processTestModeEnv = "CAPTAIN_COMPOSE_PROCESS_TEST_MODE"
	processTestFileEnv = "CAPTAIN_COMPOSE_PROCESS_TEST_FILE"
)

// This test is also the child process entry point used by TestProcessTreeCancellation.
func TestProcessTreeCancellationHelper(t *testing.T) {
	switch os.Getenv(processTestModeEnv) {
	case "heartbeat":
		path := os.Getenv(processTestFileEnv)
		for n := 1; ; n++ {
			if err := os.WriteFile(path, []byte(strconv.Itoa(n)), 0600); err != nil {
				os.Exit(2)
			}
			time.Sleep(25 * time.Millisecond)
		}
	case "parent":
		path := os.Getenv(processTestFileEnv)
		executable, err := os.Executable()
		if err != nil {
			panic(err)
		}
		child := exec.Command(executable, "-test.run=^TestProcessTreeCancellationHelper$")
		child.Env = processTestEnv(os.Environ(), "heartbeat", path)
		child.Stdout = os.Stderr
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if value, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(value)) != "" {
				fmt.Fprintf(os.Stdout, "READY %d\n", child.Process.Pid)
				for {
					time.Sleep(time.Hour)
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		panic("heartbeat child did not start")
	}
}

// TestProcessTreeCancellation proves a descendant was alive before cancellation
// and stopped updating afterward. It exercises the real OS cancellation path.
func TestProcessTreeCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heartbeat")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestProcessTreeCancellationHelper$")
	cmd.Env = processTestEnv(os.Environ(), "parent", path)
	configureProcess(cmd)
	cancelResult := make(chan error, 1)
	configuredCancel := cmd.Cancel
	cmd.Cancel = func() error {
		err := configuredCancel()
		cancelResult <- err
		return err
	}
	cmd.WaitDelay = 3 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var child *os.Process
	defer func() {
		cancel()
		_ = cmd.Process.Kill()
		if child != nil {
			_ = child.Kill()
			_ = child.Release()
		}
	}()

	ready := make(chan int, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) == 2 && fields[0] == "READY" {
				pid, err := strconv.Atoi(fields[1])
				if err == nil {
					ready <- pid
					return
				}
			}
			if strings.HasPrefix(scanner.Text(), "READY") {
				ready <- 0
				return
			}
		}
		ready <- 0
	}()
	select {
	case childPID := <-ready:
		if childPID == 0 {
			t.Fatal("helper exited before descendant became active")
		}
		child, err = os.FindProcess(childPID)
		if err != nil {
			t.Fatalf("open descendant process handle: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for active descendant")
	}

	if before := readHeartbeat(t, path); before == "" {
		t.Fatal("descendant was not active before cancellation")
	}
	cancel()
	waitErr := cmd.Wait()
	var cancelErr error
	select {
	case cancelErr = <-cancelResult:
	default:
	}
	if waitErr == nil {
		t.Fatal("canceled helper unexpectedly exited successfully")
	}
	// Windows taskkill walks and terminates a tree asynchronously. Measure only
	// after the root process has fully exited so normal shutdown latency is not
	// mistaken for a surviving descendant.
	afterShutdown := readHeartbeat(t, path)
	time.Sleep(250 * time.Millisecond)
	after := readHeartbeat(t, path)
	if after != afterShutdown {
		t.Fatalf("descendant continued after cancellation completed: heartbeat changed from %q to %q (wait error: %v, cancel error: %v)", afterShutdown, after, waitErr, cancelErr)
	}
}

func processTestEnv(env []string, mode, path string) []string {
	result := make([]string, 0, len(env)+2)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, processTestModeEnv) || strings.EqualFold(key, processTestFileEnv) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, processTestModeEnv+"="+mode, processTestFileEnv+"="+path)
}

func readHeartbeat(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read descendant heartbeat: %v", err)
	}
	return strings.TrimSpace(string(data))
}
