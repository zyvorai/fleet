// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/runtimeadapter"
)

// newTestRunner builds a Runner against a fake control-plane server, with the
// runtime adapter's k3s manifest dir pointed at a scratch directory so
// reconciliation is deterministic and filesystem-only (no systemctl/docker
// dependency, which may not exist in the test environment).
func newTestRunner(t *testing.T, serverURL string) (*Runner, *StateFile) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "agent.json")
	st, err := OpenState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Server:          serverURL,
		Name:            "test-site",
		Region:          "test",
		EnrollmentToken: "enroll-token",
		Interval:        time.Second,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	r := NewRunner(cfg, st)
	r.runtime = &runtimeadapter.Manager{K3sManifestDir: t.TempDir()}
	return r, st
}

func hasEvent(events []model.Event, kind string) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func countEvents(events []model.Event, kind string) int {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func TestEnsureEnrollment_Success(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/register" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(Registration{SiteID: "site_123", AgentToken: "tok_abc", HeartbeatSeconds: 30})
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	if err := r.ensureEnrollment(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	if snap.SiteID != "site_123" || snap.AgentToken != "tok_abc" {
		t.Fatalf("state not updated from registration: %+v", snap)
	}
	if gotAuth != "Bearer enroll-token" {
		t.Fatalf("expected enrollment token sent as bearer auth, got %q", gotAuth)
	}
}

func TestEnsureEnrollment_AlreadyEnrolledSkipsNetwork(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(500)
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	if err := st.Update(func(s *LocalState) { s.SiteID = "existing"; s.AgentToken = "tok" }); err != nil {
		t.Fatal(err)
	}
	if err := r.ensureEnrollment(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("expected no HTTP call when the agent is already enrolled")
	}
}

func TestEnsureEnrollment_MissingTokenErrors(t *testing.T) {
	r, _ := newTestRunner(t, "http://unused.invalid")
	r.cfg.EnrollmentToken = ""
	if err := r.ensureEnrollment(context.Background()); err == nil {
		t.Fatal("expected an error when no enrollment token is configured for first start")
	}
}

// TestCycle_OfflineAutonomy is the product's core promise: when the control
// plane is unreachable, the agent must keep reconciling its last-known
// desired state locally rather than freezing until the WAN returns.
func TestCycle_OfflineAutonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	manifestDir := r.runtime.K3sManifestDir
	rev := model.Revision{ID: "rev1", Name: "test-rev", Workloads: []model.RuntimeSpec{
		{Kind: "k3s", Name: "app", State: "running", Manifest: "apiVersion: v1\nkind: Pod"},
	}}
	if err := st.Update(func(s *LocalState) {
		s.SiteID, s.AgentToken = "site1", "tok"
		s.CachedRevision = &rev
	}); err != nil {
		t.Fatal(err)
	}

	r.cycle(context.Background())

	snap := st.Snapshot()
	if !snap.ConnectivityLost {
		t.Fatal("expected ConnectivityLost=true after a failed sync")
	}
	if !hasEvent(snap.PendingEvents, "controlplane.unreachable") {
		t.Fatal("expected a controlplane.unreachable event to be queued")
	}
	if hasEvent(snap.PendingEvents, "revision.applied") {
		t.Fatal("routine offline reconciliation should not announce revision.applied (announce=false)")
	}
	if snap.AppliedRevision != "rev1" {
		t.Fatalf("expected the cached revision to be reconciled and applied locally while offline, got AppliedRevision=%q", snap.AppliedRevision)
	}
	if _, err := os.Stat(filepath.Join(manifestDir, "zyvor-fleet-app.yaml")); err != nil {
		t.Fatalf("expected the k3s manifest to be written to disk despite the control plane being unreachable: %v", err)
	}
}

// TestCycle_ReconnectAnnouncesRestoration verifies both that ConnectivityLost
// clears on a successful sync and that the reconnection is announced. The
// announcement is checked via the heartbeat request body, not the local
// PendingEvents afterward: a successful heartbeat in the same cycle
// immediately flushes whatever was just queued (that's the delivery
// mechanism), so the local queue is correctly empty again by the time
// cycle() returns.
func TestCycle_ReconnectAnnouncesRestoration(t *testing.T) {
	var heartbeatBody struct {
		Events []model.Event `json:"events"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync"):
			_ = json.NewEncoder(w).Encode(SyncResponse{})
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			_ = json.NewDecoder(r.Body).Decode(&heartbeatBody)
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	if err := st.Update(func(s *LocalState) {
		s.SiteID, s.AgentToken = "site1", "tok"
		s.ConnectivityLost = true
	}); err != nil {
		t.Fatal(err)
	}

	r.cycle(context.Background())

	snap := st.Snapshot()
	if snap.ConnectivityLost {
		t.Fatal("expected ConnectivityLost to clear after a successful sync")
	}
	if !hasEvent(heartbeatBody.Events, "controlplane.reconnected") {
		t.Fatalf("expected a controlplane.reconnected event in the heartbeat payload, got %+v", heartbeatBody.Events)
	}
	if len(snap.PendingEvents) != 0 {
		t.Fatalf("expected the local queue to be flushed after a successful heartbeat delivered the event, got %+v", snap.PendingEvents)
	}
}

// TestCycle_RevisionMismatchGuardFailsClosed verifies the safety check in
// cycle(): if the server's desired revision ID doesn't match the body the
// agent has cached, the agent must refuse to apply it rather than trust a
// partial/corrupt response.
func TestCycle_RevisionMismatchGuardFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync"):
			_ = json.NewEncoder(w).Encode(SyncResponse{DesiredRevision: "rev-server-says"})
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	cached := model.Revision{ID: "rev-cached-stale", Workloads: []model.RuntimeSpec{
		{Kind: "k3s", Name: "app", State: "running", Manifest: "x"},
	}}
	if err := st.Update(func(s *LocalState) {
		s.SiteID, s.AgentToken = "site1", "tok"
		s.CachedRevision = &cached
	}); err != nil {
		t.Fatal(err)
	}

	r.cycle(context.Background())

	snap := st.Snapshot()
	if snap.AppliedRevision != "" {
		t.Fatalf("expected no reconciliation when the cached revision ID doesn't match the server's desired revision, got AppliedRevision=%q", snap.AppliedRevision)
	}
}

func TestCycle_AcksUnsupportedCommandAsFailed(t *testing.T) {
	var ackBody struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync"):
			_ = json.NewEncoder(w).Encode(SyncResponse{Commands: []model.Command{{ID: "cmd1", Type: "reboot"}}})
		case strings.HasSuffix(r.URL.Path, "/ack"):
			_ = json.NewDecoder(r.Body).Decode(&ackBody)
			w.WriteHeader(200)
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	if err := st.Update(func(s *LocalState) { s.SiteID, s.AgentToken = "site1", "tok" }); err != nil {
		t.Fatal(err)
	}

	r.cycle(context.Background())

	if ackBody.Status != "failed" {
		t.Fatalf("expected an unrecognized command type to be acked as failed, got status=%q error=%q", ackBody.Status, ackBody.Error)
	}
}

// TestCycle_HeartbeatFailureKeepsPendingEvents verifies no queued events are
// dropped when the heartbeat that would flush them fails to reach the server.
func TestCycle_HeartbeatFailureKeepsPendingEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync"):
			_ = json.NewEncoder(w).Encode(SyncResponse{})
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	r, st := newTestRunner(t, srv.URL)
	if err := st.Update(func(s *LocalState) {
		s.SiteID, s.AgentToken = "site1", "tok"
		s.PendingEvents = []model.Event{{Kind: "queued-before-failure"}}
	}); err != nil {
		t.Fatal(err)
	}

	r.cycle(context.Background())

	snap := st.Snapshot()
	if len(snap.PendingEvents) == 0 {
		t.Fatal("expected pending events to survive a failed heartbeat instead of being silently dropped")
	}
}

func TestReconcileRevision_SuccessAppliesAndAnnounces(t *testing.T) {
	r, st := newTestRunner(t, "http://unused.invalid")
	rev := model.Revision{ID: "rev1", Name: "test", Workloads: []model.RuntimeSpec{
		{Kind: "k3s", Name: "app", State: "running", Manifest: "content"},
	}}

	ok := r.reconcileRevision(context.Background(), rev, true)

	if !ok {
		t.Fatal("expected reconciliation to succeed")
	}
	snap := st.Snapshot()
	if snap.AppliedRevision != "rev1" {
		t.Fatalf("expected AppliedRevision=rev1, got %q", snap.AppliedRevision)
	}
	if snap.FailedRevision != "" {
		t.Fatalf("expected no FailedRevision, got %q", snap.FailedRevision)
	}
	if !hasEvent(snap.PendingEvents, "revision.applied") {
		t.Fatal("expected a revision.applied event when announce=true")
	}
}

// TestReconcileRevision_FailureDedupesRepeatedAnnouncements verifies the
// guard at runner.go's reconcileRevision: repeating the exact same failure
// must not spam a new event every cycle (the drift-controller reconciles
// every cycle, not just on change).
func TestReconcileRevision_FailureDedupesRepeatedAnnouncements(t *testing.T) {
	r, st := newTestRunner(t, "http://unused.invalid")
	rev := model.Revision{ID: "rev-bad", Name: "broken", Workloads: []model.RuntimeSpec{
		{Kind: "k3s", Name: "app", State: "running", Manifest: ""}, // empty manifest is rejected deterministically
	}}

	if ok := r.reconcileRevision(context.Background(), rev, true); ok {
		t.Fatal("expected reconciliation to fail for an empty k3s manifest")
	}
	snap := st.Snapshot()
	if snap.FailedRevision != "rev-bad" {
		t.Fatalf("expected FailedRevision=rev-bad, got %q", snap.FailedRevision)
	}
	if n := countEvents(snap.PendingEvents, "revision.failed"); n != 1 {
		t.Fatalf("expected exactly 1 revision.failed event, got %d", n)
	}

	if ok := r.reconcileRevision(context.Background(), rev, true); ok {
		t.Fatal("expected the second identical reconciliation attempt to fail too")
	}
	snap2 := st.Snapshot()
	if n := countEvents(snap2.PendingEvents, "revision.failed"); n != 1 {
		t.Fatalf("expected no duplicate revision.failed event on a repeated identical failure, got %d total", n)
	}
}

// TestQueueEvent_CapsAt2000AndKeepsMostRecent seeds the queue just under the
// cap directly (bypassing queueEvent's individual fsync-per-call cost) and
// only exercises queueEvent itself for the handful of calls that actually
// cross the 2000 boundary, instead of calling it 2000+ times.
func TestQueueEvent_CapsAt2000AndKeepsMostRecent(t *testing.T) {
	r, st := newTestRunner(t, "http://unused.invalid")
	seed := make([]model.Event, 1998)
	for i := range seed {
		seed[i] = model.Event{Kind: "seed", Message: fmt.Sprintf("seed-%d", i)}
	}
	if err := st.Update(func(s *LocalState) { s.PendingEvents = seed }); err != nil {
		t.Fatal(err)
	}

	const extra = 5
	for i := 0; i < extra; i++ {
		r.queueEvent("test", "info", fmt.Sprintf("event-%d", i))
	}

	snap := st.Snapshot()
	if len(snap.PendingEvents) != 2000 {
		t.Fatalf("expected pending events capped at 2000, got %d", len(snap.PendingEvents))
	}
	last := snap.PendingEvents[len(snap.PendingEvents)-1]
	if last.Message != fmt.Sprintf("event-%d", extra-1) {
		t.Fatalf("expected the most recent event to be retained when trimming, got %q", last.Message)
	}
}
