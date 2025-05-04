package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
	"github.com/google/uuid"
)

type store struct {
	tempDirs []string
}

func newStore() *store {
	return &store{
		tempDirs: make([]string, 0),
	}
}

func (s *store) save(d deployment.Deployment, payload []byte) (string, error) {
	id := uuid.NewString()
	dirName := fmt.Sprintf("%s-%s", d.Name, id)

	dir, err := os.MkdirTemp("", dirName)
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	composeFile := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(composeFile, payload, 0600); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("failed to write compose file: %w", err)
	}

	s.tempDirs = append(s.tempDirs, dir)

	return composeFile, nil
}

func (s *store) cleanup() {
	for _, dir := range s.tempDirs {
		_ = os.RemoveAll(dir)
	}
	s.tempDirs = nil
}
