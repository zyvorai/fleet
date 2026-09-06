package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/zyvorai/fleet/internal/model"
)

type Store struct {
	mu   sync.RWMutex
	path string
	data model.State
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store path is required")
	}
	s := &Store{path: path, data: model.State{SchemaVersion: 1}}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("decode state: %w", err)
		}
		if s.data.SchemaVersion == 0 {
			s.data.SchemaVersion = 1
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read state: %w", err)
	}
	return s, nil
}

func (s *Store) Snapshot() model.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// JSON round-trip provides a simple deep copy for maps/slices and keeps callers
	// from mutating store-owned data.
	b, _ := json.Marshal(s.data)
	var out model.State
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Store) Update(fn func(*model.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloneBytes, _ := json.Marshal(s.data)
	var next model.State
	if err := json.Unmarshal(cloneBytes, &next); err != nil {
		return err
	}
	if err := fn(&next); err != nil {
		return err
	}
	if next.SchemaVersion == 0 {
		next.SchemaVersion = 1
	}
	nextBytes, _ := json.Marshal(next)
	if bytes.Equal(cloneBytes, nextBytes) {
		return nil
	}
	if err := s.persistLocked(next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *Store) persistLocked(data model.State) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open temporary state: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
