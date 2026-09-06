// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/fleet/internal/auth"
	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/store"
)

type testEnv struct {
	t          *testing.T
	server     *httptest.Server
	client     *http.Client
	store      *store.Store
	enrollment string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("test-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	enroll := "zf_enroll_test"
	if err := st.Update(func(s *model.State) error {
		s.Users = append(s.Users, model.User{ID: "u1", Email: "admin@example.test", Name: "Admin", Role: model.RoleAdmin, PasswordHash: hash, CreatedAt: time.Now().UTC()})
		s.EnrollmentTokens = append(s.EnrollmentTokens, model.EnrollmentToken{ID: "e1", Name: "test", TokenHash: auth.SHA256Token(enroll), CreatedAt: time.Now().UTC(), MaxUses: 2})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sm, err := auth.NewSessionManager([]byte(strings.Repeat("s", 32)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(st, sm, Config{SiteTimeout: time.Minute}).Handler())
	jar, _ := cookiejar.New(nil)
	return &testEnv{t: t, server: srv, client: &http.Client{Jar: jar}, store: st, enrollment: enroll}
}
func (e *testEnv) close() { e.server.Close() }
func (e *testEnv) req(method, path string, body any, authMarker bool) (int, []byte) {
	e.t.Helper()
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			e.t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, e.server.URL+path, &b)
	if err != nil {
		e.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authMarker {
		req.Header.Set("X-Zyvor-Request", "1")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp.StatusCode, out.Bytes()
}
func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", string(b), err)
	}
	return v
}

func TestLoginAuthAndStaticUI(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	resp, err := e.client.Get(e.server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Security-Policy"); ct == "" {
		t.Fatal("missing CSP")
	}
	code, _ := e.req("GET", "/api/v1/dashboard", nil, false)
	if code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
	code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false)
	if code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
	code, _ = e.req("GET", "/api/v1/dashboard", nil, false)
	if code != 200 {
		t.Fatalf("dashboard %d", code)
	}
}

func TestAgentEnrollmentRevisionAndWaveRollout(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false)
	if code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
	// Create desired-state revision through the authenticated API.
	code, b = e.req("POST", "/api/v1/revisions", map[string]any{"name": "r1", "notes": "test", "workloads": []map[string]any{{"kind": "systemd", "name": "chronyd", "state": "running"}}}, true)
	if code != 201 {
		t.Fatalf("revision %d: %s", code, b)
	}
	rev := decode[model.Revision](t, b)
	// Enroll two sites with one reusable test token.
	type registration struct {
		SiteID     string `json:"siteId"`
		AgentToken string `json:"agentToken"`
	}
	register := func(name string) registration {
		reqBody, _ := json.Marshal(map[string]any{"name": name, "region": "lab", "agentVersion": "test", "inventory": map[string]any{"hostname": name, "os": "linux", "arch": "amd64", "cpuCount": 4}})
		req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/agent/register", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.enrollment)
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		if resp.StatusCode != 201 {
			t.Fatalf("register %d %s", resp.StatusCode, buf.String())
		}
		return decode[registration](t, buf.Bytes())
	}
	a := register("site-a")
	bb := register("site-b")
	code, b = e.req("POST", "/api/v1/rollouts", map[string]any{"name": "wave", "revisionId": rev.ID, "strategy": "waves", "waveSize": 1, "siteIds": []string{a.SiteID, bb.SiteID}}, true)
	if code != 201 {
		t.Fatalf("rollout %d: %s", code, b)
	}
	// Only the first site should receive the revision in wave 1.
	syncSite := func(reg registration) map[string]any {
		req, _ := http.NewRequest("GET", e.server.URL+"/api/v1/agent/sync", nil)
		req.Header.Set("Authorization", "Bearer "+reg.AgentToken)
		req.Header.Set("X-Zyvor-Site-ID", reg.SiteID)
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if got := syncSite(a)["desiredRevision"]; got != rev.ID {
		t.Fatalf("first wave got %v", got)
	}
	if got := syncSite(bb)["desiredRevision"]; got != "" {
		t.Fatalf("second site activated early: %v", got)
	}
	// First site reports applied revision; next wave should activate.
	heartbeat := func(reg registration, applied string) {
		payload, _ := json.Marshal(map[string]any{"inventory": map[string]any{"hostname": "x", "os": "linux", "arch": "amd64", "cpuCount": 4}, "agentVersion": "test", "appliedRevision": applied, "queuedEvents": 0, "autonomyMode": false, "events": []any{}})
		req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/agent/heartbeat", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+reg.AgentToken)
		req.Header.Set("X-Zyvor-Site-ID", reg.SiteID)
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("heartbeat %d", resp.StatusCode)
		}
	}
	heartbeat(a, rev.ID)
	if got := syncSite(bb)["desiredRevision"]; got != rev.ID {
		t.Fatalf("second wave not activated: %v", got)
	}
	heartbeat(bb, rev.ID)
	code, b = e.req("GET", "/api/v1/rollouts", nil, false)
	if code != 200 {
		t.Fatal(code)
	}
	rollouts := decode[[]model.Rollout](t, b)
	if len(rollouts) != 1 || rollouts[0].Status != "completed" {
		t.Fatalf("rollout not complete: %+v", rollouts)
	}
}

func TestMutatingCookieAPIRequiresSameOriginMarker(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	code, _ := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false)
	if code != 200 {
		t.Fatal(code)
	}
	code, _ = e.req("POST", "/api/v1/revisions", map[string]any{"name": "bad", "workloads": []any{}}, false)
	if code != 403 {
		t.Fatalf("expected 403, got %d", code)
	}
}

type testRegistration struct {
	SiteID     string `json:"siteId"`
	AgentToken string `json:"agentToken"`
}

func loginTestAdmin(t *testing.T, e *testEnv) {
	t.Helper()
	code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false)
	if code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
}

func registerSite(t *testing.T, e *testEnv, name string, labels map[string]string) testRegistration {
	t.Helper()
	_ = e.store.Update(func(st *model.State) error {
		for i := range st.EnrollmentTokens {
			st.EnrollmentTokens[i].MaxUses = 100
		}
		return nil
	})
	payload, _ := json.Marshal(map[string]any{
		"name": name, "region": "lab", "labels": labels, "agentVersion": "test",
		"inventory": map[string]any{"hostname": name, "os": "linux", "arch": "amd64", "cpuCount": 4},
	})
	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/agent/register", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.enrollment)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode != 201 {
		t.Fatalf("register %d: %s", resp.StatusCode, buf.String())
	}
	return decode[testRegistration](t, buf.Bytes())
}

func createTestRevision(t *testing.T, e *testEnv, name string) model.Revision {
	t.Helper()
	code, b := e.req("POST", "/api/v1/revisions", map[string]any{
		"name":      name,
		"workloads": []map[string]any{{"kind": "k3s", "name": "edge-config", "state": "present", "manifest": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edge-config\n"}},
	}, true)
	if code != 201 {
		t.Fatalf("revision %d: %s", code, b)
	}
	return decode[model.Revision](t, b)
}

func syncRegistration(t *testing.T, e *testEnv, reg testRegistration) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", e.server.URL+"/api/v1/agent/sync", nil)
	req.Header.Set("Authorization", "Bearer "+reg.AgentToken)
	req.Header.Set("X-Zyvor-Site-ID", reg.SiteID)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("sync status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func heartbeatRegistration(t *testing.T, e *testEnv, reg testRegistration, applied, failed, failure string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"inventory":    map[string]any{"hostname": "x", "os": "linux", "arch": "amd64", "cpuCount": 4},
		"agentVersion": "test", "appliedRevision": applied, "failedRevision": failed, "revisionError": failure,
		"queuedEvents": 0, "autonomyMode": false, "workloadHealth": []any{}, "events": []any{},
	})
	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/agent/heartbeat", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+reg.AgentToken)
	req.Header.Set("X-Zyvor-Site-ID", reg.SiteID)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("heartbeat status %d", resp.StatusCode)
	}
}

func TestSiteGroupsDynamicSelectorsAndRolloutPlan(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	loginTestAdmin(t, e)
	a := registerSite(t, e, "prod-a", map[string]string{"env": "prod", "zone": "west"})
	b := registerSite(t, e, "prod-b", map[string]string{"env": "prod", "zone": "east"})
	_ = registerSite(t, e, "dev-a", map[string]string{"env": "dev"})

	code, body := e.req("POST", "/api/v1/site-groups", map[string]any{"name": "Production", "selector": map[string]string{"env": "prod"}}, true)
	if code != 201 {
		t.Fatalf("group %d: %s", code, body)
	}
	created := decode[map[string]any](t, body)
	group := created["group"].(map[string]any)
	groupID := group["id"].(string)

	code, body = e.req("POST", "/api/v1/rollouts/plan", map[string]any{"groupId": groupID, "waveSize": 1}, true)
	if code != 200 {
		t.Fatalf("plan %d: %s", code, body)
	}
	plan := decode[map[string]any](t, body)
	if int(plan["total"].(float64)) != 2 || int(plan["waves"].(float64)) != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	ids := plan["siteIds"].([]any)
	got := map[string]bool{}
	for _, id := range ids {
		got[id.(string)] = true
	}
	if !got[a.SiteID] || !got[b.SiteID] {
		t.Fatalf("selector missed production sites: %+v", got)
	}
}

func TestRolloutApprovalPauseResumeAndAbort(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	loginTestAdmin(t, e)
	rev := createTestRevision(t, e, "safe-release")
	a := registerSite(t, e, "site-a", map[string]string{"env": "prod"})
	b := registerSite(t, e, "site-b", map[string]string{"env": "prod"})

	code, body := e.req("POST", "/api/v1/rollouts", map[string]any{
		"name": "approval rollout", "revisionId": rev.ID, "waveSize": 1,
		"siteIds": []string{a.SiteID, b.SiteID}, "approvalRequired": true,
	}, true)
	if code != 201 {
		t.Fatalf("rollout %d: %s", code, body)
	}
	ro := decode[model.Rollout](t, body)
	if ro.Status != "pending_approval" {
		t.Fatalf("expected pending approval, got %s", ro.Status)
	}
	if got := syncRegistration(t, e, a)["desiredRevision"]; got != "" {
		t.Fatalf("revision activated before approval: %v", got)
	}

	code, body = e.req("POST", "/api/v1/rollouts/"+ro.ID+"/approve", nil, true)
	if code != 200 {
		t.Fatalf("approve %d: %s", code, body)
	}
	if got := syncRegistration(t, e, a)["desiredRevision"]; got != rev.ID {
		t.Fatalf("first wave not activated: %v", got)
	}
	code, body = e.req("POST", "/api/v1/rollouts/"+ro.ID+"/pause", nil, true)
	if code != 200 {
		t.Fatalf("pause %d: %s", code, body)
	}
	heartbeatRegistration(t, e, a, rev.ID, "", "")
	if got := syncRegistration(t, e, b)["desiredRevision"]; got != "" {
		t.Fatalf("paused rollout advanced: %v", got)
	}
	code, body = e.req("POST", "/api/v1/rollouts/"+ro.ID+"/resume", nil, true)
	if code != 200 {
		t.Fatalf("resume %d: %s", code, body)
	}
	if got := syncRegistration(t, e, b)["desiredRevision"]; got != rev.ID {
		t.Fatalf("resumed rollout did not advance: %v", got)
	}
	code, body = e.req("DELETE", "/api/v1/sites/"+b.SiteID, nil, true)
	if code != http.StatusConflict {
		t.Fatalf("active rollout site deletion should conflict, got %d: %s", code, body)
	}
	code, body = e.req("POST", "/api/v1/rollouts/"+ro.ID+"/abort", nil, true)
	if code != 200 {
		t.Fatalf("abort %d: %s", code, body)
	}
	aborted := decode[model.Rollout](t, body)
	if aborted.Status != "aborted" {
		t.Fatalf("expected aborted, got %s", aborted.Status)
	}
	code, body = e.req("DELETE", "/api/v1/sites/"+b.SiteID, nil, true)
	if code != 200 {
		t.Fatalf("site deletion after abort %d: %s", code, body)
	}
}

func TestRolloutFailureAutoRollback(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	loginTestAdmin(t, e)
	previous := createTestRevision(t, e, "previous")
	next := createTestRevision(t, e, "next")
	reg := registerSite(t, e, "critical-site", map[string]string{"tier": "critical"})
	if err := e.store.Update(func(st *model.State) error {
		for i := range st.Sites {
			if st.Sites[i].ID == reg.SiteID {
				st.Sites[i].DesiredRevision = previous.ID
				st.Sites[i].AppliedRevision = previous.ID
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, body := e.req("POST", "/api/v1/rollouts", map[string]any{
		"name": "guarded", "revisionId": next.ID, "waveSize": 1, "siteIds": []string{reg.SiteID},
		"maxFailures": 0, "autoRollback": true,
	}, true)
	if code != 201 {
		t.Fatalf("rollout %d: %s", code, body)
	}
	ro := decode[model.Rollout](t, body)
	heartbeatRegistration(t, e, reg, previous.ID, next.ID, "health probe failed")
	code, body = e.req("GET", "/api/v1/rollouts", nil, false)
	if code != 200 {
		t.Fatal(code)
	}
	rollouts := decode[[]model.Rollout](t, body)
	var got model.Rollout
	for _, item := range rollouts {
		if item.ID == ro.ID {
			got = item
		}
	}
	if got.Status != "failed" || !got.RolledBack || len(got.FailedSites) != 1 {
		t.Fatalf("rollout not failed+rolled back: %+v", got)
	}
	if desired := syncRegistration(t, e, reg)["desiredRevision"]; desired != previous.ID {
		t.Fatalf("site did not roll back to previous revision: %v", desired)
	}
	code, body = e.req("POST", "/api/v1/rollouts/"+ro.ID+"/retry", nil, true)
	if code != 200 {
		t.Fatalf("retry %d: %s", code, body)
	}
	retried := decode[model.Rollout](t, body)
	if retried.Status != "running" || retried.RolledBack {
		t.Fatalf("retry did not reset rollout: %+v", retried)
	}
	if desired := syncRegistration(t, e, reg)["desiredRevision"]; desired != next.ID {
		t.Fatalf("retry did not reactivate target revision: %v", desired)
	}
}

func TestMetricsExposeFleetState(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	resp, err := e.client.Get(e.server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	_, _ = b.ReadFrom(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(b.String(), "zyvor_fleet_sites") {
		t.Fatalf("metrics unavailable: %d %s", resp.StatusCode, b.String())
	}
}

func TestPasswordChangeRevokesExistingSession(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	loginTestAdmin(t, e)
	code, body := e.req("PATCH", "/api/v1/users/u1", map[string]any{"password": "new-test-admin-password"}, true)
	if code != 200 {
		t.Fatalf("password update %d: %s", code, body)
	}
	code, _ = e.req("GET", "/api/v1/dashboard", nil, false)
	if code != 401 {
		t.Fatalf("expected revoked session, got %d", code)
	}
}

func TestStrictWaveIntermission(t *testing.T) {
	t0 := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	st := model.State{
		Sites: []model.Site{{ID: "a"}, {ID: "b"}},
		Rollouts: []model.Rollout{{
			ID: "ro1", Name: "strict waves", RevisionID: "rev1", Status: "running",
			SiteIDs: []string{"a", "b"}, WaveSize: 1, PauseSeconds: 10,
		}},
	}
	reconcileRolloutsAt(&st, t0)
	if st.Sites[0].DesiredRevision != "rev1" || st.Sites[1].DesiredRevision != "" {
		t.Fatalf("first wave incorrect: %+v", st.Sites)
	}
	st.Sites[0].AppliedRevision = "rev1"
	reconcileRolloutsAt(&st, t0.Add(time.Second))
	if st.Rollouts[0].NextWaveAt == nil {
		t.Fatal("expected inter-wave pause deadline")
	}
	reconcileRolloutsAt(&st, t0.Add(9*time.Second))
	if st.Sites[1].DesiredRevision != "" {
		t.Fatal("second wave activated before pause elapsed")
	}
	reconcileRolloutsAt(&st, t0.Add(12*time.Second))
	if st.Sites[1].DesiredRevision != "rev1" || st.Rollouts[0].CurrentWave != 2 {
		t.Fatalf("second wave did not activate after pause: site=%+v rollout=%+v", st.Sites[1], st.Rollouts[0])
	}
}

func TestRevisionRejectsContainerOptionInjection(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	loginTestAdmin(t, e)
	code, body := e.req("POST", "/api/v1/revisions", map[string]any{
		"name":      "unsafe image",
		"workloads": []map[string]any{{"kind": "container", "name": "edge-api", "state": "running", "image": "--privileged"}},
	}, true)
	if code != http.StatusBadRequest {
		t.Fatalf("unsafe container image accepted: %d %s", code, body)
	}
}
