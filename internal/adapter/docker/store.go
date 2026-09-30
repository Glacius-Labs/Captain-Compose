package docker

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

// The entrypoint holds an OS file lock. OpenRoot confines manifest access.
type store struct{ root *os.Root }

const historyLimit = 10

type storedRevision struct {
	Revision string    `json:"revision"`
	Compose  []byte    `json:"compose"`
	Created  time.Time `json:"created"`
}

// deploymentState records intent separately from the last successful revision.
// Compose bytes are private deployment data and are never returned by queries.
type deploymentState struct {
	Version              int              `json:"version"`
	Name                 string           `json:"name"`
	DesiredRevision      string           `json:"desired_revision,omitempty"`
	DesiredCreatedAt     time.Time        `json:"desired_created_at,omitempty"`
	SuccessfulRevision   string           `json:"successful_revision,omitempty"`
	Compose              []byte           `json:"compose,omitempty"`
	Phase                string           `json:"phase"`
	LastRequestID        string           `json:"last_request_id,omitempty"`
	LastAction           string           `json:"last_action,omitempty"`
	HasExpectedRevision  bool             `json:"has_expected_revision,omitempty"`
	LastExpectedRevision string           `json:"last_expected_revision,omitempty"`
	History              []storedRevision `json:"history,omitempty"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

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
	_ = s.root.Remove(p + ".tmp")
	f, err := s.root.OpenFile(p+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
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
	state, err := s.readStateFile(name)
	if err == nil {
		if state.Phase == "removed" || len(state.Compose) == 0 {
			return nil, os.ErrNotExist
		}
		return state.Compose, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return s.readLegacy(name)
}

func (s *store) readLegacy(name string) ([]byte, error) {
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

func (s *store) statePath(name string) (string, error) {
	if err := deployment.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(name, "state.json"), nil
}

func (s *store) readState(name string) (deploymentState, error) {
	state, err := s.readStateFile(name)
	if err == nil {
		return state, nil
	}
	if !os.IsNotExist(err) {
		return deploymentState{}, err
	}
	// 1.0 stored only compose.json. Keep it readable and migrate on next write.
	legacy, legacyErr := s.readLegacy(name)
	if legacyErr != nil {
		return deploymentState{}, legacyErr
	}
	rev := revisionOf(legacy)
	now := time.Now().UTC()
	return deploymentState{Version: 2, Name: name, DesiredRevision: rev, DesiredCreatedAt: now, Compose: legacy, Phase: "legacy", UpdatedAt: now}, nil
}

func (s *store) readStateFile(name string) (deploymentState, error) {
	p, err := s.statePath(name)
	if err != nil {
		return deploymentState{}, err
	}
	b, err := s.root.ReadFile(p)
	if err == nil {
		var state deploymentState
		if err := json.Unmarshal(b, &state); err != nil {
			return deploymentState{}, fmt.Errorf("read deployment metadata")
		}
		if err := validateStoredState(state, name); err != nil {
			return deploymentState{}, err
		}
		return state, nil
	}
	if !os.IsNotExist(err) {
		return deploymentState{}, fmt.Errorf("read deployment metadata: %w", err)
	}
	return deploymentState{}, os.ErrNotExist
}

func validPhase(phase string) bool {
	switch phase {
	case "absent", "legacy", "applying", "active", "failed", "removing", "removed":
		return true
	}
	return false
}

func validRevision(revision string) bool {
	if len(revision) != 64 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil && strings.ToLower(revision) == revision
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boundHistory(history []storedRevision, successful string, hasCurrent bool) []storedRevision {
	unique := make([]storedRevision, 0, len(history))
	seen := make(map[string]bool, len(history))
	for _, item := range history {
		if !seen[item.Revision] {
			unique = append(unique, item)
			seen[item.Revision] = true
		}
	}
	limit := historyLimit - boolInt(hasCurrent)
	for len(unique) > limit {
		remove := -1
		for i, item := range unique {
			if item.Revision != successful {
				remove = i
				break
			}
		}
		if remove < 0 {
			remove = 0
		}
		unique = append(unique[:remove], unique[remove+1:]...)
	}
	return unique
}

func validateStoredState(state deploymentState, name string) error {
	if state.Version != 2 || state.Name != name || !validPhase(state.Phase) {
		return fmt.Errorf("unsupported deployment metadata")
	}
	if len(state.History) > historyLimit {
		return fmt.Errorf("deployment history exceeds supported limit")
	}
	if len(state.History)+boolInt(len(state.Compose) > 0) > historyLimit {
		return fmt.Errorf("deployment revision history exceeds supported limit")
	}
	if (len(state.Compose) > 0 && state.DesiredRevision != revisionOf(state.Compose)) || (len(state.Compose) == 0 && state.DesiredRevision != "") {
		return fmt.Errorf("deployment revision metadata is inconsistent")
	}
	if state.DesiredRevision != "" && !validRevision(state.DesiredRevision) {
		return fmt.Errorf("invalid desired deployment revision")
	}
	if state.SuccessfulRevision != "" && !validRevision(state.SuccessfulRevision) {
		return fmt.Errorf("invalid successful deployment revision")
	}
	if state.SuccessfulRevision != "" && state.SuccessfulRevision != state.DesiredRevision {
		found := false
		for _, item := range state.History {
			if item.Revision == state.SuccessfulRevision {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("successful deployment revision content is not retained")
		}
	}
	if (state.Phase == "removed" || state.Phase == "absent") && (state.DesiredRevision != "" || len(state.Compose) != 0) {
		return fmt.Errorf("deployment phase conflicts with stored manifest")
	}
	if state.LastAction != "" && state.LastAction != "create" && state.LastAction != "remove" && state.LastAction != "revert" {
		return fmt.Errorf("invalid deployment operation metadata")
	}
	if (state.LastRequestID != "" && state.LastAction == "") || (!state.HasExpectedRevision && state.LastExpectedRevision != "") || (state.HasExpectedRevision && state.LastAction != "create" && state.LastExpectedRevision == "") {
		return fmt.Errorf("inconsistent request metadata")
	}
	if state.LastExpectedRevision != "" && !validRevision(state.LastExpectedRevision) {
		return fmt.Errorf("invalid expected deployment revision metadata")
	}
	for _, item := range state.History {
		if !validRevision(item.Revision) || len(item.Compose) == 0 || item.Revision != revisionOf(item.Compose) {
			return fmt.Errorf("invalid deployment history entry")
		}
	}
	return nil
}

func (s *store) saveState(state deploymentState) error {
	p, err := s.statePath(state.Name)
	if err != nil {
		return err
	}
	if err := s.root.MkdirAll(state.Name, 0700); err != nil {
		return err
	}
	if err := syncDirectory(s.root, "."); err != nil {
		return fmt.Errorf("sync deployment store directory: %w", err)
	}
	state.Version = 2
	state.History = boundHistory(state.History, state.SuccessfulRevision, len(state.Compose) > 0)
	if err := validateStoredState(state, state.Name); err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_ = s.root.Remove(p + ".tmp")
	f, err := s.root.OpenFile(p+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := s.root.Rename(p+".tmp", p); err != nil {
		return err
	}
	if err := syncDirectory(s.root, state.Name); err != nil {
		return fmt.Errorf("sync deployment metadata directory: %w", err)
	}
	legacyPath, _ := s.path(state.Name)
	if err := s.root.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove migrated 1.0 manifest: %w", err)
	}
	if err := syncDirectory(s.root, state.Name); err != nil {
		return fmt.Errorf("sync migrated deployment directory: %w", err)
	}
	return nil
}
