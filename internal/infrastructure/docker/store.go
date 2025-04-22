package docker

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type store struct {
	mu          sync.RWMutex
	dir         string
	deployments map[string]deployment.Deployment
}

func newStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	s := &store{
		dir:         dir,
		deployments: make(map[string]deployment.Deployment),
	}

	if err := s.init(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *store) Add(d deployment.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deployments[d.Name] = d

	path := s.composeFilePath(d.Name)
	return os.WriteFile(path, d.Content, 0644)
}

func (s *store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.deployments, name)

	path := s.composeFilePath(name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (s *store) Get(name string) (deployment.Deployment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	d, ok := s.deployments[name]
	return d, ok
}

func (s *store) All() []deployment.Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]deployment.Deployment, 0, len(s.deployments))
	for _, d := range s.deployments {
		out = append(out, d)
	}
	return out
}

func (s *store) composeFilePath(name string) string {
	return filepath.Join(s.dir, name+".yml")
}

func (s *store) init() error {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.yml"))
	if err != nil {
		return err
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue // Skip corrupted/broken files, but continue recovery
		}
		name := strings.TrimSuffix(filepath.Base(file), ".yml")

		s.deployments[name] = deployment.Deployment{
			Name:    name,
			Content: content,
		}
	}

	return nil
}
