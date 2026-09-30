//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package docker

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcess isolates the command in its own process group so context
// cancellation can stop Docker CLI and any Compose plugin it started.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}
