//go:build windows

package operator

// Windows does not expose portable directory fsync through os.File.Sync.
func syncRequestDirectory(string) error { return nil }
