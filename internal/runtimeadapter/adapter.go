package runtimeadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/zyvorai/fleet/internal/model"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)

type Manager struct {
	K3sManifestDir string
	AllowQEMU      bool
}

func (m *Manager) Check(ctx context.Context, spec model.RuntimeSpec) model.WorkloadHealth {
	result := model.WorkloadHealth{Name: spec.Name, Kind: spec.Kind, CheckedAt: time.Now().UTC()}
	err := m.checkRuntime(ctx, spec)
	if err == nil && spec.Health != nil && spec.State != "stopped" {
		err = checkProbe(ctx, *spec.Health)
	}
	result.Healthy = err == nil
	if err != nil {
		result.Message = err.Error()
	}
	return result
}

func (m *Manager) checkRuntime(ctx context.Context, spec model.RuntimeSpec) error {
	if !safeName.MatchString(spec.Name) {
		return fmt.Errorf("unsafe workload name %q", spec.Name)
	}
	switch spec.Kind {
	case "systemd":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return errors.New("systemctl not found")
		}
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", spec.Name)
		err := cmd.Run()
		if spec.State == "stopped" {
			if err == nil {
				return errors.New("service is still active")
			}
			return nil
		}
		if err != nil {
			return errors.New("service is not active")
		}
		return nil
	case "container":
		engine := ""
		for _, candidate := range []string{"podman", "docker"} {
			if _, err := exec.LookPath(candidate); err == nil {
				engine = candidate
				break
			}
		}
		if engine == "" {
			return errors.New("podman or docker is required")
		}
		cmd := exec.CommandContext(ctx, engine, "inspect", "--format", "{{.State.Running}}", spec.Name)
		out, err := cmd.Output()
		if spec.State == "stopped" {
			if err != nil {
				return nil
			}
			if strings.TrimSpace(string(out)) == "true" {
				return errors.New("container is still running")
			}
			return nil
		}
		if err != nil || strings.TrimSpace(string(out)) != "true" {
			return errors.New("container is not running")
		}
		return nil
	case "k3s":
		path := filepath.Join(m.K3sManifestDir, "zyvor-fleet-"+spec.Name+".yaml")
		_, err := os.Stat(path)
		if spec.State == "stopped" {
			if err == nil {
				return errors.New("manifest still present")
			}
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err != nil {
			return fmt.Errorf("manifest missing: %w", err)
		}
		return nil
	case "qemu":
		pidfile := filepath.Join(os.TempDir(), "zyvor-fleet-"+spec.Name+".pid")
		_, err := os.Stat(pidfile)
		if spec.State == "stopped" {
			if err == nil {
				return errors.New("qemu pid file still present")
			}
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err != nil {
			return fmt.Errorf("qemu pid file missing: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported runtime %q", spec.Kind)
	}
}

func checkProbe(ctx context.Context, probe model.HealthProbe) error {
	timeout := time.Duration(probe.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 5 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch probe.Type {
	case "", "runtime":
		return nil
	case "http":
		if probe.URL == "" {
			return errors.New("http health probe requires url")
		}
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, probe.URL, nil)
		if err != nil {
			return err
		}
		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("http probe: %w", err)
		}
		defer resp.Body.Close()
		expected := probe.ExpectedStatus
		if expected == 0 {
			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				return fmt.Errorf("http probe returned %d", resp.StatusCode)
			}
			return nil
		}
		if resp.StatusCode != expected {
			return fmt.Errorf("http probe returned %d, expected %d", resp.StatusCode, expected)
		}
		return nil
	case "tcp":
		if probe.Address == "" {
			return errors.New("tcp health probe requires address")
		}
		dialer := net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(probeCtx, "tcp", probe.Address)
		if err != nil {
			return fmt.Errorf("tcp probe: %w", err)
		}
		_ = conn.Close()
		return nil
	default:
		return fmt.Errorf("unsupported health probe type %q", probe.Type)
	}
}

func New() *Manager {
	dir := os.Getenv("ZYVOR_FLEET_K3S_MANIFEST_DIR")
	if dir == "" {
		dir = "/var/lib/rancher/k3s/server/manifests"
	}
	return &Manager{K3sManifestDir: dir, AllowQEMU: os.Getenv("ZYVOR_FLEET_ALLOW_QEMU") == "1"}
}

func (m *Manager) Reconcile(ctx context.Context, spec model.RuntimeSpec) error {
	if !safeName.MatchString(spec.Name) {
		return fmt.Errorf("unsafe workload name %q", spec.Name)
	}
	switch spec.Kind {
	case "systemd":
		return reconcileSystemd(ctx, spec)
	case "container":
		return reconcileContainer(ctx, spec)
	case "k3s":
		return m.reconcileK3s(spec)
	case "qemu":
		return m.reconcileQEMU(ctx, spec)
	default:
		return fmt.Errorf("unsupported runtime %q", spec.Kind)
	}
}

func reconcileSystemd(ctx context.Context, spec model.RuntimeSpec) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl not found")
	}
	active := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", spec.Name).Run() == nil
	if (spec.State == "stopped" && !active) || (spec.State != "stopped" && active) {
		return nil
	}
	verb := "start"
	if spec.State == "stopped" {
		verb = "stop"
	}
	cmd := exec.CommandContext(ctx, "systemctl", verb, spec.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s: %w: %s", verb, spec.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func reconcileContainer(ctx context.Context, spec model.RuntimeSpec) error {
	engine := ""
	for _, candidate := range []string{"podman", "docker"} {
		if _, err := exec.LookPath(candidate); err == nil {
			engine = candidate
			break
		}
	}
	if engine == "" {
		return errors.New("podman or docker is required")
	}
	if spec.State == "stopped" {
		cmd := exec.CommandContext(ctx, engine, "rm", "-f", spec.Name)
		if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "No such") && !strings.Contains(string(out), "no such") {
			return fmt.Errorf("remove container: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	digest := containerSpecDigest(spec)
	// The managed spec digest covers image, args, environment and ports, so local
	// configuration drift is corrected even while the WAN/control plane is absent.
	inspect := exec.CommandContext(ctx, engine, "inspect", "--format", `{{.Config.Image}}|{{index .Config.Labels "io.zyvor.fleet.spec-sha"}}`, spec.Name)
	if out, err := inspect.Output(); err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
		if len(parts) == 2 && parts[0] == spec.Image && parts[1] == digest {
			return nil
		}
		remove := exec.CommandContext(ctx, engine, "rm", "-f", spec.Name)
		if out, err := remove.CombinedOutput(); err != nil {
			return fmt.Errorf("replace drifted container: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	args := []string{"run", "-d", "--restart", "unless-stopped", "--name", spec.Name,
		"--label", "io.zyvor.fleet.managed=true", "--label", "io.zyvor.fleet.spec-sha=" + digest}
	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+spec.Env[k])
	}
	for _, p := range spec.Ports {
		args = append(args, "-p", p)
	}
	args = append(args, "--", spec.Image)
	args = append(args, spec.Args...)
	cmd := exec.CommandContext(ctx, engine, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run container: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func containerSpecDigest(spec model.RuntimeSpec) string {
	stable := struct {
		Image string            `json:"image"`
		Args  []string          `json:"args,omitempty"`
		Env   map[string]string `json:"env,omitempty"`
		Ports []string          `json:"ports,omitempty"`
	}{Image: spec.Image, Args: spec.Args, Env: spec.Env, Ports: spec.Ports}
	b, _ := json.Marshal(stable)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) reconcileK3s(spec model.RuntimeSpec) error {
	if spec.Manifest == "" {
		return errors.New("manifest is empty")
	}
	if err := os.MkdirAll(m.K3sManifestDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(m.K3sManifestDir, "zyvor-fleet-"+spec.Name+".yaml")
	if spec.State == "stopped" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	desired := []byte(spec.Manifest)
	if current, err := os.ReadFile(path); err == nil && string(current) == string(desired) {
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, desired, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (m *Manager) reconcileQEMU(ctx context.Context, spec model.RuntimeSpec) error {
	if !m.AllowQEMU {
		return errors.New("qemu reconciliation is disabled; set ZYVOR_FLEET_ALLOW_QEMU=1 explicitly")
	}
	binary := "qemu-system-x86_64"
	if runtime.GOARCH == "arm64" {
		binary = "qemu-system-aarch64"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s not found", binary)
	}
	pidfile := filepath.Join(os.TempDir(), "zyvor-fleet-"+spec.Name+".pid")
	if spec.State == "stopped" {
		b, err := os.ReadFile(pidfile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		pid := strings.TrimSpace(string(b))
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(pid) {
			return errors.New("invalid qemu pid file")
		}
		cmd := exec.CommandContext(ctx, "kill", pid)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("stop qemu: %w: %s", err, strings.TrimSpace(string(out)))
		}
		_ = os.Remove(pidfile)
		return nil
	}
	if _, err := os.Stat(pidfile); err == nil {
		return nil
	}
	if !filepath.IsAbs(spec.Disk) {
		return errors.New("qemu disk path must be absolute")
	}
	cpus := spec.CPUs
	if cpus <= 0 {
		cpus = 2
	}
	mem := spec.MemoryMiB
	if mem <= 0 {
		mem = 2048
	}
	args := []string{"-daemonize", "-name", spec.Name, "-pidfile", pidfile, "-m", fmt.Sprint(mem), "-smp", fmt.Sprint(cpus), "-drive", "file=" + spec.Disk + ",format=qcow2,if=virtio", "-netdev", "user,id=n0", "-device", "virtio-net-pci,netdev=n0", "-display", "none"}
	cmd := exec.CommandContext(ctx, binary, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start qemu: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
