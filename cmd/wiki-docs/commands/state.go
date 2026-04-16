package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileState struct {
	LastRev      string    `json:"last_rev"`
	LastChecksum string    `json:"last_checksum"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

// SyncState manages the local persistence of document staging metadata.
// It tracks revisions and body checksums to allow precise auditing of
// staged documents even when their YAML frontmatter is stripped.
type SyncState struct {
	Files map[string]FileState `json:"files"`
	mu    sync.RWMutex
}

// NewSyncState initializes a fresh staging metadata state.
func NewSyncState() *SyncState {
	return &SyncState{
		Files: make(map[string]FileState),
	}
}

// GetStatePath returns the absolute path to the staging metadata file,
// ensuring it is stored within the .config directory of the provided repo root.
func GetStatePath(repoRoot string) (string, error) {
	if repoRoot == "" {
		return "", filepath.ErrBadPattern
	}
	return filepath.Join(repoRoot, ".config", "wiki-docs", "state.json"), nil
}

// LoadState retrieves the staging metadata from the repository's .config directory.
// It returns an initialized state even if the file does not yet exist.
func LoadState(repoRoot string) (*SyncState, error) {
	path, err := GetStatePath(repoRoot)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewSyncState(), nil
	}
	if err != nil {
		return nil, err
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Files == nil {
		state.Files = make(map[string]FileState)
	}
	return &state, nil
}

// Save persists the current sync state to the repository's .config directory.
func (s *SyncState) Save(repoRoot string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := GetStatePath(repoRoot)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Get retrieves the recorded file state for a repository-relative path.
func (s *SyncState) Get(relPath string) (FileState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.Files[relPath]
	return val, ok
}

// Update records the sync metadata for a repository-relative path.
func (s *SyncState) Update(relPath, rev, checksum string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Files[relPath] = FileState{
		LastRev:      rev,
		LastChecksum: checksum,
		LastSyncedAt: time.Now(),
	}
}
