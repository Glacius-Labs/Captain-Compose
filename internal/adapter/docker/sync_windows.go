package docker

import "os"

// Windows does not support fsync of directory handles through os.File.Sync.
func syncDirectory(_ *os.Root, _ string) error { return nil }
