package runtimeadapter

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyvorai/fleet/internal/model"
)

func TestK3sManifestReconcile(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{K3sManifestDir: dir}
	spec := model.RuntimeSpec{Kind: "k3s", Name: "edge-api", State: "running", Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: edge-api\n"}
	if err := m.Reconcile(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "zyvor-fleet-edge-api.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ConfigMap") {
		t.Fatal("manifest mismatch")
	}
	spec.State = "stopped"
	if err := m.Reconcile(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "zyvor-fleet-edge-api.yaml")); !os.IsNotExist(err) {
		t.Fatal("manifest not removed")
	}
}

func TestRejectUnsafeName(t *testing.T) {
	m := &Manager{K3sManifestDir: t.TempDir()}
	err := m.Reconcile(context.Background(), model.RuntimeSpec{Kind: "k3s", Name: "../../x", State: "running", Manifest: "x"})
	if err == nil {
		t.Fatal("unsafe name accepted")
	}
}

func TestHTTPHealthProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := &Manager{K3sManifestDir: dir}
	spec := model.RuntimeSpec{
		Kind: "k3s", Name: "healthy-app", State: "present",
		Manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: healthy-app\n",
		Health:   &model.HealthProbe{Type: "http", URL: srv.URL, ExpectedStatus: http.StatusNoContent, TimeoutSeconds: 2},
	}
	if err := m.Reconcile(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	health := m.Check(context.Background(), spec)
	if !health.Healthy {
		t.Fatalf("expected healthy probe: %+v", health)
	}
	spec.Health.ExpectedStatus = http.StatusOK
	health = m.Check(context.Background(), spec)
	if health.Healthy || !strings.Contains(health.Message, "expected 200") {
		t.Fatalf("expected failed status probe: %+v", health)
	}
}

func TestTCPHealthProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	if err := checkProbe(context.Background(), model.HealthProbe{Type: "tcp", Address: ln.Addr().String(), TimeoutSeconds: 2}); err != nil {
		t.Fatalf("tcp probe failed: %v", err)
	}
}

func TestContainerSpecDigestTracksConfigurationDrift(t *testing.T) {
	base := model.RuntimeSpec{Kind: "container", Name: "edge-api", State: "running", Image: "example/api:v1", Args: []string{"serve"}, Env: map[string]string{"MODE": "prod"}, Ports: []string{"8080:8080"}}
	a := containerSpecDigest(base)
	copy := base
	copy.Env = map[string]string{"MODE": "canary"}
	if b := containerSpecDigest(copy); a == b {
		t.Fatal("environment drift did not change managed spec digest")
	}
	copy = base
	copy.Health = &model.HealthProbe{Type: "http", URL: "http://127.0.0.1:8080/health"}
	if b := containerSpecDigest(copy); a != b {
		t.Fatal("health-only changes should not replace a container")
	}
}
