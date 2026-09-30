//go:build !windows

package docker

import "os"

func syncDirectory(root *os.Root, path string) error {
	dir, err := root.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
