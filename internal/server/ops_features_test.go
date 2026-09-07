// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/ops"
)

func apiReq(t *testing.T, client *http.Client, base, token, method, path string, body any) (int, []byte) {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, base+path, &b)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp.StatusCode, out.Bytes()
}

func registerOpsSite(t *testing.T, e *testEnv, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name": name, "region": "lab", "agentVersion": "test",
		"inventory": map[string]any{"hostname": name, "os": "linux", "arch": "amd64", "cpuCount": 4},
	})
	req, _ := http.NewRequest(http.MethodPost, e.server.URL+"/api/v1/agent/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.enrollment)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		SiteID string `json:"siteId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register returned %d", resp.StatusCode)
	}
	return out.SiteID
}

func TestAPITokenScopesAndNoCSRFCookieRequirement(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	if code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false); code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
	code, b := e.req("POST", "/api/v1/api-tokens", map[string]any{
		"name": "ci", "role": "operator", "scopes": []string{"read", "rollouts:write"}, "expiresHours": 24,
	}, true)
	if code != 201 {
		t.Fatalf("create API token %d: %s", code, b)
	}
	created := decode[struct {
		Token string `json:"token"`
	}](t, b)

	client := &http.Client{Timeout: 2 * time.Second}
	if code, b = apiReq(t, client, e.server.URL, created.Token, "GET", "/api/v1/dashboard", nil); code != 200 {
		t.Fatalf("token read %d: %s", code, b)
	}
	if code, b = apiReq(t, client, e.server.URL, created.Token, "POST", "/api/v1/revisions", map[string]any{
		"name": "from-ci", "workloads": []map[string]any{{"kind": "systemd", "name": "chronyd", "state": "running"}},
	}); code != 201 {
		t.Fatalf("token rollout write %d: %s", code, b)
	}
	if code, _ = apiReq(t, client, e.server.URL, created.Token, "POST", "/api/v1/api-tokens", map[string]any{"name": "escalate"}); code != 403 {
		t.Fatalf("expected admin endpoint to be denied, got %d", code)
	}
}

func TestMaintenanceSitesAreExcludedUnlessExplicitlyForced(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	if code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false); code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
	a := registerOpsSite(t, e, "maint-a")
	bsite := registerOpsSite(t, e, "maint-b")
	if code, b := e.req("PATCH", "/api/v1/sites/"+a, map[string]any{"maintenance": true, "maintenanceReason": "generator service"}, true); code != 200 {
		t.Fatalf("maintenance patch %d: %s", code, b)
	}
	code, b := e.req("POST", "/api/v1/revisions", map[string]any{
		"name": "r-maint", "workloads": []map[string]any{{"kind": "systemd", "name": "chronyd", "state": "running"}},
	}, true)
	if code != 201 {
		t.Fatalf("revision %d: %s", code, b)
	}
	rev := decode[model.Revision](t, b)
	type plan struct {
		Total               int      `json:"total"`
		MaintenanceExcluded []string `json:"maintenanceExcluded"`
	}
	code, b = e.req("POST", "/api/v1/rollouts/plan", map[string]any{"revisionId": rev.ID, "siteIds": []string{a, bsite}}, true)
	if code != 200 {
		t.Fatalf("plan %d: %s", code, b)
	}
	p := decode[plan](t, b)
	if p.Total != 1 || len(p.MaintenanceExcluded) != 1 || p.MaintenanceExcluded[0] != a {
		t.Fatalf("unexpected maintenance filtering: %+v", p)
	}
	code, b = e.req("POST", "/api/v1/rollouts/plan", map[string]any{"revisionId": rev.ID, "siteIds": []string{a, bsite}, "includeMaintenance": true}, true)
	if code != 200 || decode[plan](t, b).Total != 2 {
		t.Fatalf("forced maintenance plan %d: %s", code, b)
	}
}

func TestSignedWebhookAndMutationAudit(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	if code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false); code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
	type delivery struct {
		body []byte
		sig  string
	}
	deliveries := make(chan delivery, 2)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		deliveries <- delivery{body: buf.Bytes(), sig: r.Header.Get("X-Zyvor-Fleet-Signature")}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()
	secret := strings.Repeat("w", 32)
	code, b := e.req("POST", "/api/v1/webhooks", map[string]any{
		"name": "ops", "url": sink.URL, "secret": secret, "eventKinds": []string{"api_token.*"},
	}, true)
	if code != 201 {
		t.Fatalf("webhook create %d: %s", code, b)
	}
	time.Sleep(50 * time.Millisecond) // let the creation-triggered dispatcher release its lock
	code, b = e.req("POST", "/api/v1/api-tokens", map[string]any{"name": "webhook-trigger", "role": "viewer", "scopes": []string{"read"}}, true)
	if code != 201 {
		t.Fatalf("token create %d: %s", code, b)
	}
	select {
	case got := <-deliveries:
		if want := ops.SignWebhook(secret, got.body); got.sig != want {
			t.Fatalf("bad webhook signature: got %q want %q", got.sig, want)
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(got.body, &envelope); err != nil || envelope.Type != "api_token.created" {
			t.Fatalf("unexpected webhook payload: %s", got.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}

	code, b = e.req("GET", "/api/v1/audit?limit=50", nil, false)
	if code != 200 {
		t.Fatalf("audit list %d: %s", code, b)
	}
	audit := decode[[]model.AuditRecord](t, b)
	found := false
	for _, record := range audit {
		if record.Path == "/api/v1/api-tokens" && record.Method == "POST" && record.Actor == "admin@example.test" && record.Status == 201 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected API-token mutation in audit log: %+v", audit)
	}
}
