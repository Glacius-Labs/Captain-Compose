//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package docker

import "os/exec"

// configureProcess is a no-op on platforms without process-group support in
// os/exec. Those platforms retain CommandContext's direct-process cancellation.
func configureProcess(cmd *exec.Cmd) {}
