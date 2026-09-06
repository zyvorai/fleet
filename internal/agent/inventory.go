// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/zyvorai/fleet/internal/model"
)

func Inventory() model.Inventory {
	host, _ := os.Hostname()
	inv := model.Inventory{Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, CPUCount: runtime.NumCPU(), Metadata: map[string]string{}}
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		inv.Kernel = strings.TrimSpace(string(out))
	}
	if f, err := os.Open("/proc/meminfo"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				kb, _ := strconv.ParseUint(fields[1], 10, 64)
				inv.MemoryBytes = kb * 1024
				break
			}
		}
	}
	checks := map[string]string{"k3s": "k3s", "kubectl": "kubernetes", "podman": "podman", "docker": "docker", "qemu-system-x86_64": "qemu", "qemu-system-aarch64": "qemu", "firecracker": "firecracker", "cloud-hypervisor": "cloud-hypervisor"}
	seen := map[string]bool{}
	for bin, name := range checks {
		if _, err := exec.LookPath(bin); err == nil && !seen[name] {
			inv.Runtimes = append(inv.Runtimes, name)
			seen[name] = true
		}
	}
	if _, err := os.Stat("/dev/kvm"); err == nil {
		inv.Capabilities = append(inv.Capabilities, "kvm")
	}
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		inv.Capabilities = append(inv.Capabilities, "nvidia-gpu")
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			inv.Addresses = append(inv.Addresses, a.String())
		}
	}
	return inv
}
