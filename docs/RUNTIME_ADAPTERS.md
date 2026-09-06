# Runtime adapters

Fleet intentionally has no `shell` workload type. Every desired-state operation goes through a narrow typed adapter with validation.

## Health probes

Every running workload can add a local gate:

```json
"health": {"type":"http","url":"http://127.0.0.1:8080/healthz","expectedStatus":200,"timeoutSeconds":5,"graceSeconds":20}
```

Supported types are `runtime`, `http`, and `tcp`. Runtime health is always checked first. `graceSeconds` allows a newly promoted workload time to become ready before the agent reports the revision failed.

## systemd

```json
{"kind":"systemd","name":"chronyd","state":"running"}
```

Allowed states: `running`, `stopped`, `present` (`present` follows the running path). Fleet checks `systemctl is-active` first and only invokes `start`/`stop` when state differs.

## container

```json
{
  "kind":"container",
  "name":"edge-api",
  "state":"running",
  "image":"ghcr.io/example/edge-api:1.4.0",
  "env":{"MODE":"edge"},
  "ports":["8081:8080"],
  "args":["--listen",":8080"]
}
```

Podman is preferred; Docker is the fallback. Fleet labels managed containers with a SHA-256 fingerprint of image, args, environment and ports. Any drift in that typed spec causes the container to be replaced, including while the agent is operating from cached desired state offline. Health configuration is intentionally excluded from the container fingerprint so probe-only edits do not restart workloads.

## k3s

```json
{
  "kind":"k3s",
  "name":"local-api",
  "state":"running",
  "manifest":"apiVersion: apps/v1\nkind: Deployment\n..."
}
```

The agent atomically maintains `zyvor-fleet-<name>.yaml` in `/var/lib/rancher/k3s/server/manifests` by default. Override with `ZYVOR_FLEET_K3S_MANIFEST_DIR`. Identical manifest content is left untouched to avoid unnecessary disk writes. `stopped` removes the managed manifest.

## qemu

QEMU is deliberately opt-in:

```bash
export ZYVOR_FLEET_ALLOW_QEMU=1
```

```json
{"kind":"qemu","name":"legacy-api","state":"running","disk":"/var/lib/edge/images/legacy-api.qcow2","cpus":2,"memoryMiB":2048}
```

The disk must be absolute. The adapter creates a simple headless VM and tracks its PID. Deep VM networking, storage, snapshots and migration belong in Zyvor Fabric/Machina/Zeus rather than Fleet.

## Permissions

Grant only what a site's selected adapters require: ordinary user for inventory, narrow systemd policy, Podman/Docker access, k3s manifest-directory write access, or `/dev/kvm` + disk access for QEMU. Avoid unrestricted root where a narrower local policy is sufficient.
