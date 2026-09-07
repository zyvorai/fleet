<div align="center">

# Zyvor Fleet

### Every site. Still running.

**Offline-first edge fleet control plane for Linux, Kubernetes, containers and virtual machines.**

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![CI](https://github.com/zyvorai/fleet/actions/workflows/ci.yml/badge.svg)](https://github.com/zyvorai/fleet/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8.svg)](go.mod)

[Quick start](#quick-start) · [Tutorial](docs/TUTORIAL.md) · [Architecture](#architecture) · [Offline autonomy](#offline-autonomy) · [Runtime adapters](#runtime-adapters) · [Kubernetes](#kubernetes) · [Security](#security) · [Docs](#documentation)

</div>

---

Zyvor Fleet manages **remote sites as a fleet**, not merely clusters. A site can be a factory gateway, retail server, telecom POP, branch appliance, rugged Linux box, k3s node, GPU edge server or KVM host.

The control plane declares what should run. A small `fleet-agent` pulls that desired state, caches the complete revision locally and keeps reconciling it when the WAN disappears. The agent exposes no arbitrary remote shell.

```text
                       ZYVOR FLEET CONTROL PLANE
                     UI · API · RBAC · rollouts
                                  │
                      pull / heartbeat / replay
                                  │
              ┌───────────────────┼───────────────────┐
              │                   │                   │
          Site Pune           Site Delhi          Site Tokyo
          fleet-agent         fleet-agent         fleet-agent
              │                   │                   │
       ┌──────┼──────┐      ┌─────┼─────┐       ┌────┼─────┐
     systemd container k3s   k3s container      QEMU systemd GPU

        WAN down? Each agent continues from its cached desired revision.
```

## Why this is a separate Zyvor product

Zyvor already has strong point products. Fleet is the **site lifecycle and desired-state layer** that connects them.

| Product | Owns | Fleet relationship |
|---|---|---|
| **Nodra** | MQTT/HTTP edge ingress, store-and-forward, twins, local routes | Optional edge-data plane |
| **Zyvor Relay** | Durable operational action/ack/verify workflow | Optional action bus |
| **PacketWolf** | eBPF network intelligence | Optional network health/policy evidence |
| **Argus** | Application assurance | Optional post-rollout verification |
| **Forge** | GPU/inference operations | Optional edge AI plane |
| **HyperCluster** | Kubernetes cluster lifecycle | Fleet can coordinate site-level promotion |
| **IronWolf** | Bare-metal lifecycle | Fleet can represent/target the resulting sites |
| **Zeus OS / Machina** | VM operations | Fleet handles cross-site desired-state rollout, not deep VM management |

Fleet intentionally does **not** reimplement Nodra's device/event data plane or Fabric's private-cloud VM control plane.

## Features

### Fleet control plane

- Apple-inspired, Zyvor orange/black embedded web console; no CDN, trackers, external fonts or npm runtime.
- Secure email/password login with PBKDF2-HMAC-SHA256 password hashes.
- Signed, HttpOnly, SameSite session cookies and role-based API authorization.
- One-time/restricted enrollment tokens exchanged for independent per-site identities.
- Live fleet inventory: OS, architecture, CPU, memory, addresses, detected runtimes and capabilities.
- Site maintenance/cordon mode that excludes serviced sites from new rollouts by default.
- Scoped bearer API tokens for CI/GitOps automation without shared human credentials.
- Signed outgoing webhooks with durable cursors plus a bounded operator/API mutation audit trail.
- Automatic online/degraded/offline state, per-workload health and operational event stream.
- Declarative revisions with typed workload specs.
- Dynamic label-based site groups and pre-flight rollout plans.
- Strict wave rollouts with approval, scheduling windows, inter-wave pause, failure budgets, pause/resume/abort/retry and manual/automatic rollback.
- Prometheus-format `/metrics` endpoint.
- Single-writer atomic JSON persistence for the initial open-source release.
- Embedded UI + API in one `fleetd` binary.
- Health/readiness endpoints and structured JSON logging.

### Offline-first agent

- Pull-based control flow: edge sites need no inbound management port.
- Complete desired revision cached on local disk with `0600` permissions.
- Local reconciling continues when the control plane cannot be reached.
- Connectivity transition events are queued locally and replayed after reconnect.
- Bounded local event queue.
- Runtime/capability discovery.
- Per-site bearer identity stored locally rather than reusing the enrollment token.
- No generic server-triggered shell execution.

### Typed runtime adapters

| Kind | Behavior | Safety boundary |
|---|---|---|
| `systemd` | Start/stop a named unit | Validated unit name; `systemctl` only |
| `container` | Run/replace/stop Docker or Podman containers | Validated name; declared image/env/ports/args only |
| `k3s` | Atomically maintain a manifest in the k3s manifests directory | Validated workload name; explicit manifest content |
| `qemu` | Start/stop a basic KVM/QEMU VM | Disabled by default; absolute disk path required |

The container adapter fingerprints image, args, environment and ports and replaces any drifted managed container. The k3s adapter continuously restores the declared manifest while skipping unchanged writes. This makes cached desired state useful during real WAN loss rather than functioning as a passive snapshot.

## Quick start

Requirements: Go 1.27+.

```bash
git clone https://github.com/zyvorai/fleet.git
cd fleet
make check
make build
```

### Start the control plane for local evaluation

```bash
export ZYVOR_FLEET_ADMIN_PASSWORD='zyvor-fleet-demo'
export ZYVOR_FLEET_SESSION_SECRET='local-demo-session-secret-change-me-1234567890'

./bin/fleetd --demo
```

Open **http://127.0.0.1:8080**.

Demo login:

```text
admin@zyvor.local
zyvor-fleet-demo
```

Demo enrollment token printed at startup:

```text
zf_enroll_demo-local-only
```

`--demo` is explicit and is not intended for production.

### Enroll a site

On another Linux machine:

```bash
export ZYVOR_FLEET_ENROLLMENT_TOKEN='zf_enroll_demo-local-only'

./bin/fleet-agent \
  --server http://CONTROL_PLANE:8080 \
  --name factory-pune-01 \
  --region india-west \
  --labels class=factory,tier=production
```

The agent exchanges the enrollment token for its own high-entropy site credential, stores it locally, reports inventory and begins pull-based sync.

### CLI

```bash
export ZYVOR_FLEET_ADMIN_PASSWORD='zyvor-fleet-demo'

fleetctl status
fleetctl sites
fleetctl events
fleetctl revisions
fleetctl rollouts
fleetctl groups
fleetctl group-create "Production" env=production,class=factory
fleetctl rollout-plan GROUP_ID
fleetctl rollout-pause ROLLOUT_ID
fleetctl site-maintenance SITE_ID on "scheduled service"
fleetctl api-token-create github-ci operator read,rollouts:write
fleetctl audit
fleetctl webhooks
fleetctl enroll-token "Factory install"
```

## Desired-state example

A revision can combine multiple runtime types:

```json
{
  "name": "Factory stack 2026.09",
  "notes": "Promote edge API and maintain time sync",
  "workloads": [
    {
      "kind": "systemd",
      "name": "chronyd",
      "state": "running"
    },
    {
      "kind": "container",
      "name": "edge-api",
      "state": "running",
      "image": "ghcr.io/example/edge-api:1.4.0",
      "ports": ["8081:8080"]
    },
    {
      "kind": "k3s",
      "name": "local-inference",
      "state": "running",
      "manifest": "apiVersion: apps/v1\nkind: Deployment\n..."
    }
  ]
}
```

Create the revision in the UI, target explicit sites or a dynamic group, preview the rollout plan, set wave/failure/approval/window policy and start the rollout. A strict wave does not advance until every site in the active wave has reported success or failure; health failures count against the configured failure budget.

## Offline autonomy

The edge state file stores:

```text
site identity
agent credential
desired revision ID
complete cached revision
last applied revision
connectivity state
queued operational events
last successful sync
```

When sync fails:

1. The agent marks the local connectivity transition once.
2. The cached revision remains authoritative locally.
3. Typed runtime adapters continue drift reconciliation.
4. Events stay on disk.
5. When the control plane returns, queued events replay through the next heartbeat.
6. Any newer desired revision is then reconciled.

No cloud/control-plane call is necessary to keep the last accepted desired state alive.

## Architecture

```text
Browser
   │ HTTPS
   ▼
┌──────────────────────────────────────────────┐
│ fleetd                                       │
│                                              │
│ embedded web UI ─┐                           │
│ REST API ────────┼─ auth / RBAC             │
│ rollout engine ──┼─ single-writer store     │
│ event stream ────┘                           │
└──────────────────┬───────────────────────────┘
                   │ outbound pull from sites
         ┌─────────┴─────────┐
         ▼                   ▼
    fleet-agent          fleet-agent
    local state          local state
         │                   │
 typed adapters         typed adapters
 systemd/container      k3s/qemu/...
```

The initial open-source persistence mode is deliberately honest: it is a **single-writer control plane**. Kubernetes manifests therefore deploy one control-plane replica with a `ReadWriteOnce` volume. A transactional HA storage adapter is a later milestone; v0.2 does not pretend that a local file store is horizontally scalable.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the protocol and failure model. See [docs/V0.3_OPERATIONS.md](docs/V0.3_OPERATIONS.md) for maintenance cordons, API-token scopes, webhook signing and audit semantics.

## Kubernetes

### Helm

```bash
helm upgrade --install zyvor-fleet ./deploy/helm/zyvor-fleet \
  --namespace zyvor-fleet --create-namespace \
  --set admin.password='CHANGE_ME' \
  --set sessionSecret="$(openssl rand -base64 48)"
```

Enable an in-cluster node agent only when that deployment model is appropriate:

```bash
helm upgrade --install zyvor-fleet ./deploy/helm/zyvor-fleet \
  --namespace zyvor-fleet --create-namespace \
  --set admin.password='CHANGE_ME' \
  --set sessionSecret="$(openssl rand -base64 48)" \
  --set agent.enabled=true \
  --set agent.enrollmentToken='YOUR_TOKEN'
```

For ordinary remote edge boxes, install `fleet-agent` directly on the site rather than running it as a Kubernetes DaemonSet.

### Raw manifests

Copy `deploy/kubernetes/secret.example.yaml`, replace both secret values, then:

```bash
kubectl apply -k deploy/kubernetes
```

## Containers

```bash
docker compose up --build
```

The compose demo starts the control plane plus two simulated Linux sites. It is useful for the fleet UX and protocol; containerized demo agents do not manage the Docker host unless you intentionally grant host-level runtime access.

## Security

Important defaults:

- production bootstrap requires an explicit admin password;
- session secret must be 32+ bytes for stable production sessions;
- cookie sessions are HttpOnly + SameSite Strict;
- browser-session API mutations use a same-origin marker; bearer API tokens are CSRF-independent Authorization credentials;
- scoped API, enrollment and site-agent bearer tokens are persisted only as SHA-256 digests centrally;
- outgoing webhooks are HMAC-SHA256 signed and expose delivery health without exposing their signing secret through list APIs;
- operator/API mutations are recorded in a bounded audit trail without request bodies;
- state files are mode `0600`;
- login attempts are throttled;
- CSP denies third-party scripts/styles/connections;
- arbitrary shell is not a supported command type;
- QEMU control is opt-in (`ZYVOR_FLEET_ALLOW_QEMU=1`).

For production, use TLS directly or through a trusted ingress/reverse proxy. See [SECURITY.md](SECURITY.md).

## Product plan

**v0.2 — included here** adds dynamic groups, rollout pre-flight planning, strict waves, approval and maintenance windows, failure budgets, health gates, automatic/manual rollback, retry controls, site metadata, metrics, stronger session revocation, durable fsync writes and stronger offline drift detection.

Next milestones are documented in [docs/PRODUCT_PLAN.md](docs/PRODUCT_PLAN.md): signed artifacts, agent OTA, air-gap OCI bundles, enterprise identity/integrations and a real transactional HA storage adapter.

## Documentation

- [Tutorial: getting started](docs/TUTORIAL.md) — a guided walkthrough of the web console and CLI, start to first rollout
- [Architecture and failure model](ARCHITECTURE.md)
- [Product plan](docs/PRODUCT_PLAN.md)
- [Deployment guide](docs/DEPLOYMENT.md)
- [Runtime adapters](docs/RUNTIME_ADAPTERS.md)
- [REST API / OpenAPI](docs/openapi.yaml)
- [Security](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

## Testing

```bash
make check
make test-race
make live-smoke
```

CI verifies formatting, `go vet`, race-enabled Go tests, JavaScript syntax and all binaries. The integration tests also exercise dynamic selectors, pre-flight planning, approval and rollout lifecycle controls, strict inter-wave pauses, health failures, automatic rollback/retry, active-rollout deletion protection, metrics, session revocation, and typed runtime health/drift behavior.

## License

Apache License 2.0. See [LICENSE](LICENSE).

---

**Zyvor Fleet** is designed to make the control plane optional for continuity, not mandatory for survival.
