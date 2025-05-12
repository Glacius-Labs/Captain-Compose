package docker

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/stretchr/testify/require"
)

func TestStore_Save(t *testing.T) {
	s := newStore()
	d := deployment.Deployment{Name: "unit-test"}
	payload := []byte("version: '3'\nservices:\n  hello:\n    image: busybox")

	composePath, err := s.save(d, payload)
	require.NoError(t, err)
	require.FileExists(t, composePath)

	content, err := os.ReadFile(composePath)
	require.NoError(t, err)
	require.Equal(t, payload, content)

	require.Len(t, s.tempDirs, 1)
	require.DirExists(t, filepath.Dir(composePath))
}

func TestStore_Cleanup(t *testing.T) {
	s := newStore()
	d := deployment.Deployment{Name: "unit-test-cleanup"}
	payload := []byte("version: '3'")

	composePath, err := s.save(d, payload)
	require.NoError(t, err)

	dir := filepath.Dir(composePath)
	require.FileExists(t, composePath)
	require.DirExists(t, dir)

	s.cleanup()

	require.NoFileExists(t, composePath)
	require.NoDirExists(t, dir)
	require.Empty(t, s.tempDirs)
}

func TestStore_ConcurrentSaves(t *testing.T) {
	s := newStore()
	payload := []byte("version: '3'\nservices:\n  test:\n    image: busybox")

	var wg sync.WaitGroup
	count := 10

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := deployment.Deployment{Name: "concurrent-test"}
			path, err := s.save(d, payload)
			require.NoError(t, err)
			require.FileExists(t, path)
		}(i)
	}

	wg.Wait()
	require.Len(t, s.tempDirs, count)

	s.cleanup()
	require.Empty(t, s.tempDirs)
}
