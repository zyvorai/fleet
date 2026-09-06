// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zyvorai/fleet/internal/model"
)

func TestStorePersistsAtomicallyAndSnapshotsAreIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(st *model.State) error {
		st.Sites = append(st.Sites, model.Site{ID: "site_1", Name: "one", Labels: map[string]string{"tier": "edge"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state not persisted: %v", err)
	}
	snap := s.Snapshot()
	snap.Sites[0].Name = "mutated"
	snap.Sites[0].Labels["tier"] = "bad"
	snap2 := s.Snapshot()
	if snap2.Sites[0].Name != "one" || snap2.Sites[0].Labels["tier"] != "edge" {
		t.Fatal("snapshot mutated store")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().Sites[0].Name; got != "one" {
		t.Fatalf("reopen: got %q", got)
	}
}

func TestUpdateRollbackOnError(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	errSentinel := os.ErrPermission
	err := s.Update(func(st *model.State) error { st.Sites = append(st.Sites, model.Site{ID: "x"}); return errSentinel })
	if err != errSentinel {
		t.Fatalf("got %v", err)
	}
	if len(s.Snapshot().Sites) != 0 {
		t.Fatal("failed transaction leaked")
	}
}
