// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestOTAContractAssignmentAndEventAck(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	if code, b := e.req("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.test", "password": "test-admin-password"}, false); code != 200 {
		t.Fatalf("login %d: %s", code, b)
	}

	code, b := e.req("POST", "/api/v1/ota/devices", map[string]string{
		"deviceId": "minewing-gw1-lab-001",
		"name":     "lab gateway",
	}, true)
	if code != 201 {
		t.Fatalf("create device %d: %s", code, b)
	}
	created := decode[map[string]any](t, b)
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatal("missing device token")
	}

	// Unauthorized device id must not succeed even with a valid token for another path.
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/v1/devices/unknown/assignment", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown device status=%d", resp.StatusCode)
	}

	// No assignment → 204
	req, _ = http.NewRequest(http.MethodGet, e.server.URL+"/v1/devices/minewing-gw1-lab-001/assignment", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("empty assignment status=%d", resp.StatusCode)
	}

	assignment := map[string]any{
		"job_id":      "fleet-job-1",
		"device_id":   "minewing-gw1-lab-001",
		"auto_reboot": true,
		"not_before":  time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		"deadline":    time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
		"release": map[string]any{
			"key_id":    "production-1",
			"payload":   "e30=",
			"signature": "AA==",
		},
	}
	code, b = e.req("PUT", "/api/v1/ota/devices/minewing-gw1-lab-001/assignment", assignment, true)
	if code != 200 {
		t.Fatalf("put assignment %d: %s", code, b)
	}

	req, _ = http.NewRequest(http.MethodGet, e.server.URL+"/v1/devices/minewing-gw1-lab-001/assignment", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get assignment %d", resp.StatusCode)
	}
	got := decode[map[string]any](t, readAll(t, resp))
	if got["job_id"] != "fleet-job-1" {
		t.Fatalf("assignment=%v", got)
	}

	events := []map[string]any{
		{"sequence": 1, "job_id": "fleet-job-1", "state": "accepted", "time": time.Now().UTC().Format(time.RFC3339Nano)},
		{"sequence": 2, "job_id": "fleet-job-1", "state": "downloading", "time": time.Now().UTC().Format(time.RFC3339Nano)},
	}
	body, _ := json.Marshal(events)
	req, _ = http.NewRequest(http.MethodPost, e.server.URL+"/v1/devices/minewing-gw1-lab-001/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("post events %d", resp.StatusCode)
	}
	ack := decode[map[string]any](t, readAll(t, resp))
	if int(ack["sequence"].(float64)) != 2 {
		t.Fatalf("ack=%v", ack)
	}

	// Idempotent replay
	req, _ = http.NewRequest(http.MethodPost, e.server.URL+"/v1/devices/minewing-gw1-lab-001/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("replay %d", resp.StatusCode)
	}

	code, b = e.req("GET", "/api/v1/ota/devices/minewing-gw1-lab-001/events", nil, false)
	if code != 200 {
		t.Fatalf("list events %d: %s", code, b)
	}
	listed := decode[map[string]any](t, b)
	if int(listed["ackedSequence"].(float64)) != 2 {
		t.Fatalf("listed=%v", listed)
	}
}

func TestOTAContractRejectsUnboundToken(t *testing.T) {
	e := newTestEnv(t)
	defer e.close()
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/v1/devices/any/assignment", nil)
	req.Header.Set("Authorization", "Bearer totally-fake")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func readAll(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return buf.Bytes()
}
