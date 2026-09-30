package mqtt

import "os"

// Windows does not support fsync of directory handles through os.File.Sync.
func syncDirectory(root *os.Root) error { return nil }
