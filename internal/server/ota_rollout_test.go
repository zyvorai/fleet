// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestOTACanaryWavePausesAndCompletes(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	if code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false); code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}
	assignment := []byte(`{"job_id":"job","device_id":"pending","release":{"key_id":"k","payload":"YQ==","signature":"YQ=="},"not_before":"2026-01-01T00:00:00Z","deadline":"2030-01-01T00:00:00Z","auto_reboot":false}`)

	// Failure threshold pauses before the rest of the fleet is a success.
	code, b := e.req("POST", "/api/v1/ota/devices", map[string]any{"deviceId": "gw-fail", "name": "fail", "labels": map[string]string{"env": "lab"}}, true)
	if code != 201 {
		t.Fatalf("device %d %s", code, b)
	}
	failTok := tokenOf(t, b)
	code, b = e.req("POST", "/api/v1/ota/rollouts", map[string]any{
		"name": "pause", "deviceIds": []string{"gw-fail"}, "assignment": json.RawMessage(assignment),
		"canaryCount": 1, "waveSize": 1, "maxFailures": 1,
	}, true)
	if code != 201 {
		t.Fatalf("rollout %d %s", code, b)
	}
	postOTAEvent(t, e, "gw-fail", failTok, 1, "failed", "")
	code, b = e.req("GET", "/api/v1/ota/rollouts", nil, true)
	if code != 200 || !bytes.Contains(b, []byte("failure threshold")) {
		t.Fatalf("expected pause, %d %s", code, b)
	}
	code, b = e.req("GET", "/api/v1/ota/devices/gw-fail/recovery", nil, true)
	if code != 200 {
		t.Fatal(string(b))
	}

	const n = 100
	ids := make([]string, n)
	toks := make([]string, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("gw-%03d", i)
		ids[i] = id
		code, b = e.req("POST", "/api/v1/ota/devices", map[string]any{
			"deviceId": id, "name": id, "labels": map[string]string{"env": "edge"},
			"hardwareRevision": "r1", "slot": "A", "version": "1",
		}, true)
		if code != 201 {
			t.Fatalf("device %s %d %s", id, code, b)
		}
		toks[i] = tokenOf(t, b)
	}
	code, b = e.req("POST", "/api/v1/ota/cohorts", map[string]any{"name": "edge", "selector": map[string]string{"env": "edge"}}, true)
	if code != 201 {
		t.Fatalf("cohort %d %s", code, b)
	}
	var cohort struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &cohort); err != nil {
		t.Fatal(err)
	}
	code, b = e.req("POST", "/api/v1/ota/rollouts", map[string]any{
		"name": "tonight", "cohortId": cohort.ID, "assignment": json.RawMessage(assignment),
		"canaryCount": 2, "waveSize": 10, "maxFailures": 5, "maxRollbacks": 5, "maxOffline": 50,
		"windowStart": "00:00", "windowEnd": "23:59", "timezone": "UTC",
	}, true)
	if code != 201 || !bytes.Contains(b, []byte("10,000 remote gateways")) {
		t.Fatalf("wave rollout %d %s", code, b)
	}
	// Commit canaries, then the rest. Each post advances the wave.
	for i := 0; i < n; i++ {
		postOTAEvent(t, e, ids[i], toks[i], 1, "committed", "")
	}
	code, b = e.req("GET", "/api/v1/ota/rollouts", nil, true)
	if code != 200 || !bytes.Contains(b, []byte(`"status":"completed"`)) {
		t.Fatalf("expected a completed wave, %s", b)
	}
	code, b = e.req("GET", "/api/v1/ota/devices/gw-000/timeline", nil, true)
	if code != 200 || !bytes.Contains(b, []byte("committed")) {
		t.Fatalf("timeline %s", b)
	}
	postOTAEvent(t, e, ids[0], toks[0], 2, "needs_recovery", "agent restarted during installation")
	code, b = e.req("GET", "/api/v1/ota/devices/gw-000/recovery", nil, true)
	if code != 200 || !bytes.Contains(b, []byte("newly signed")) {
		t.Fatalf("guidance %s", b)
	}
	_ = time.UTC
}

func tokenOf(t *testing.T, b []byte) string {
	t.Helper()
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &created); err != nil || created.Token == "" {
		t.Fatalf("token: %s", b)
	}
	return created.Token
}

func postOTAEvent(t *testing.T, e *testEnv, id, token string, seq int, state, errText string) {
	t.Helper()
	body := fmt.Sprintf(`[{"sequence":%d,"job_id":"job","state":%q,"error":%q,"time":"2026-09-21T12:00:00Z"}]`, seq, state, errText)
	req, err := http.NewRequest(http.MethodPost, e.server.URL+"/v1/devices/"+id+"/events", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("event %s: %d %s", id, resp.StatusCode, buf.String())
	}
}
