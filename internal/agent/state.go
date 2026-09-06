package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zyvorai/fleet/internal/model"
)

type LocalState struct {
	SiteID           string                 `json:"siteId,omitempty"`
	AgentToken       string                 `json:"agentToken,omitempty"`
	AppliedRevision  string                 `json:"appliedRevision,omitempty"`
	DesiredRevision  string                 `json:"desiredRevision,omitempty"`
	CachedRevision   *model.Revision        `json:"cachedRevision,omitempty"`
	FailedRevision   string                 `json:"failedRevision,omitempty"`
	RevisionError    string                 `json:"revisionError,omitempty"`
	WorkloadHealth   []model.WorkloadHealth `json:"workloadHealth,omitempty"`
	ConnectivityLost bool                   `json:"connectivityLost,omitempty"`
	PendingEvents    []model.Event          `json:"pendingEvents,omitempty"`
	LastSync         time.Time              `json:"lastSync,omitempty"`
}

type StateFile struct {
	mu   sync.Mutex
	path string
	data LocalState
}

func OpenState(path string) (*StateFile, error) {
	s := &StateFile{path: path}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}
func (s *StateFile) Snapshot() LocalState {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.data)
	var out LocalState
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *StateFile) Update(fn func(*LocalState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
	return s.save()
}
func (s *StateFile) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
