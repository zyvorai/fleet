// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package ops

import (
	"net/http"
	"testing"
)

func TestScopeAllows(t *testing.T) {
	if !ScopeAllows([]string{"read"}, http.MethodGet, "/api/v1/dashboard") {
		t.Fatal("read scope should permit GET")
	}
	if ScopeAllows([]string{"read"}, http.MethodPost, "/api/v1/rollouts") {
		t.Fatal("read scope must not permit rollout mutation")
	}
	if !ScopeAllows([]string{"rollouts:write"}, http.MethodPost, "/api/v1/rollouts") {
		t.Fatal("rollouts:write should permit rollout mutation")
	}
	if ScopeAllows([]string{"rollouts:write"}, http.MethodPost, "/api/v1/api-tokens") {
		t.Fatal("rollouts:write must not permit admin endpoints")
	}
	if !ScopeAllows([]string{"admin"}, http.MethodDelete, "/api/v1/webhooks/w1") {
		t.Fatal("admin scope should permit all operations")
	}
}

func TestWebhookSignature(t *testing.T) {
	got := SignWebhook("secret", []byte(`{"hello":"fleet"}`))
	want := "sha256=382b63c756f5192b3094315e14ad6257daf24e193e6d1d1135877e2ca18fa0e5"
	if got != want {
		t.Fatalf("signature mismatch: got %s want %s", got, want)
	}
}

func TestMatchEventKind(t *testing.T) {
	if !MatchEventKind(nil, "rollout.started") {
		t.Fatal("empty filter should match")
	}
	if !MatchEventKind([]string{"rollout.*"}, "rollout.failed") {
		t.Fatal("prefix wildcard should match")
	}
	if MatchEventKind([]string{"site.*"}, "rollout.failed") {
		t.Fatal("wrong prefix matched")
	}
}
