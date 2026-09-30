package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

// The entrypoint holds an OS file lock. OpenRoot confines manifest access.
type store struct{ root *os.Root }

func newStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &store{root: r}, nil
}

func (s *store) path(name string) (string, error) {
	if err := deployment.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(name, "compose.json"), nil
}

func (s *store) save(d deployment.Deployment, payload []byte) (string, error) {
	p, err := s.path(d.Name)
	if err != nil {
		return "", err
	}
	if err := s.root.MkdirAll(d.Name, 0700); err != nil {
		return "", err
	}
	if err := syncDirectory(s.root, "."); err != nil {
		return "", fmt.Errorf("sync deployment store directory: %w", err)
	}
	f, err := s.root.OpenFile(p+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(payload)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := s.root.Rename(p+".tmp", p); err != nil {
		return "", err
	}
	if err := syncDirectory(s.root, d.Name); err != nil {
		return "", fmt.Errorf("sync deployment manifest directory: %w", err)
	}
	return filepath.Join(s.root.Name(), p), nil
}

func (s *store) remove(name string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	if err := s.root.Remove(p); err != nil {
		return err
	}
	if err := syncDirectory(s.root, name); err != nil {
		return fmt.Errorf("sync removed manifest directory: %w", err)
	}
	return nil
}

func (s *store) read(name string) ([]byte, error) {
	p, err := s.path(name)
	if err != nil {
		return nil, err
	}
	b, err := s.root.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read managed deployment %q: %w", name, err)
	}
	return b, nil
}
