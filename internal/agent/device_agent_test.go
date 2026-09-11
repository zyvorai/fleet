// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zyvorai/fleet/internal/model"
)

func TestEnrichWithDeviceAgent_Disabled(t *testing.T) {
	c := NewClient("http://127.0.0.1:0")
	inv := model.Inventory{Hostname: "edge-1"}
	got := c.enrichWithDeviceAgent(context.Background(), inv)
	if got.Hostname != "edge-1" || len(got.Metadata) != 0 {
		t.Fatalf("expected inventory unchanged when DeviceAgentURL is empty, got %+v", got)
	}
}

func TestEnrichWithDeviceAgent_MergesMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integrations/fleet/inventory" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"metadata":{"zyvor.device.serial":"MW-0001","zyvor.hardware.profile":"minewing-reference-arm64"}}`))
	}))
	defer srv.Close()

	c := NewClient("http://127.0.0.1:0")
	c.DeviceAgentURL = srv.URL
	c.DeviceAgentHTTP = srv.Client()

	inv := model.Inventory{Hostname: "edge-1", Metadata: map[string]string{"existing": "kept"}}
	got := c.enrichWithDeviceAgent(context.Background(), inv)

	if got.Metadata["existing"] != "kept" {
		t.Fatalf("expected pre-existing metadata preserved, got %+v", got.Metadata)
	}
	if got.Metadata["zyvor.device.serial"] != "MW-0001" {
		t.Fatalf("expected device-agent serial merged, got %+v", got.Metadata)
	}
	if got.Metadata["zyvor.hardware.profile"] != "minewing-reference-arm64" {
		t.Fatalf("expected device-agent hardware profile merged, got %+v", got.Metadata)
	}
	if got.Metadata["zyvor.device_agent.reachable"] != "true" {
		t.Fatalf("expected reachable marker set, got %+v", got.Metadata)
	}
	// Fields with a different taxonomy than Fleet's own must never be overlaid.
	if got.Hostname != "edge-1" {
		t.Fatalf("hostname must not be overlaid by device-agent projection, got %q", got.Hostname)
	}
}

func TestEnrichWithDeviceAgent_SendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"metadata":{}}`))
	}))
	defer srv.Close()

	c := NewClient("http://127.0.0.1:0")
	c.DeviceAgentURL = srv.URL
	c.DeviceAgentToken = "secret-token"
	c.DeviceAgentHTTP = srv.Client()

	c.enrichWithDeviceAgent(context.Background(), model.Inventory{})

	if gotAuth != "Bearer secret-token" {
		t.Fatalf("expected bearer token forwarded, got Authorization header %q", gotAuth)
	}
}

func TestEnrichWithDeviceAgent_UnreachableSkipsSilently(t *testing.T) {
	c := NewClient("http://127.0.0.1:0")
	c.DeviceAgentURL = "http://127.0.0.1:1" // reserved, connection refused
	inv := model.Inventory{Hostname: "edge-1"}
	got := c.enrichWithDeviceAgent(context.Background(), inv)
	if got.Hostname != "edge-1" || len(got.Metadata) != 0 {
		t.Fatalf("expected inventory unchanged when device agent is unreachable, got %+v", got)
	}
}

func TestEnrichWithDeviceAgent_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient("http://127.0.0.1:0")
	c.DeviceAgentURL = srv.URL
	c.DeviceAgentHTTP = srv.Client()

	inv := model.Inventory{Hostname: "edge-1"}
	got := c.enrichWithDeviceAgent(context.Background(), inv)
	if len(got.Metadata) != 0 {
		t.Fatalf("expected no metadata merged on non-2xx response, got %+v", got.Metadata)
	}
}
