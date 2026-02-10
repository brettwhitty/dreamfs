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

type SyncState struct {
	Files map[string]FileState `json:"files"`
	mu    sync.RWMutex
}

func NewSyncState() *SyncState {
	return &SyncState{
		Files: make(map[string]FileState),
	}
}

func GetStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wiki-sync", "state.json"), nil
}

func LoadState() (*SyncState, error) {
	path, err := GetStatePath()
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

func (s *SyncState) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := GetStatePath()
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

func (s *SyncState) Get(relPath string) (FileState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.Files[relPath]
	return val, ok
}

func (s *SyncState) Update(relPath, rev, checksum string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Files[relPath] = FileState{
		LastRev:      rev,
		LastChecksum: checksum,
		LastSyncedAt: time.Now(),
	}
}
