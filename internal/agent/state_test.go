package agent

import (
	"path/filepath"
	"testing"

	"github.com/zyvorai/zyvor-fleet/internal/model"
)

func TestStateFilePersistsOfflineQueue(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent", "state.json")
	s, err := OpenState(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(x *LocalState) {
		x.SiteID = "site_1"
		x.AgentToken = "secret"
		x.PendingEvents = append(x.PendingEvents, model.Event{Kind: "offline"})
	}); err != nil {
		t.Fatal(err)
	}
	r, err := OpenState(p)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Snapshot()
	if got.SiteID != "site_1" || got.AgentToken != "secret" || len(got.PendingEvents) != 1 {
		t.Fatalf("unexpected persisted state %+v", got)
	}
}
