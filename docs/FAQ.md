# FAQ

Questions people evaluating Zyvor Fleet actually ask, before they've
decided to adopt it. Already decided?
[`docs/TUTORIAL.md`](TUTORIAL.md) is a better starting point.

## Licensing & cost

**Is it really free?** Yes. Apache-2.0 — use, modify, and run it for
personal, lab, and commercial production use at no charge, subject to
preserving notices (see [`NOTICE`](../NOTICE)). See the README's
[License](../README.md#license) section.

**What does "Enterprise" mean here?** Production support, SLAs, and
Zyvor's other commercial products are licensed separately from this
open-source control plane. Contact sales@zyvor.dev. Nothing in this
repository requires it.

## Support

**What if I find a bug?** Open a GitHub issue.

**What if I find a security vulnerability?** See [`SECURITY.md`](../SECURITY.md)
for private reporting — supported versions are the latest tagged minor
release only.

## Production readiness

**Is this production-ready?** Current release is v0.3, which added site
maintenance/cordon mode, scoped API tokens, HMAC-signed webhooks, and a
bounded mutation audit trail — real, shipped features
([`docs/V0.3_OPERATIONS.md`](V0.3_OPERATIONS.md)). What's **not** there
yet: [`ARCHITECTURE.md`](../ARCHITECTURE.md) states plainly that "v0.3 does
not pretend that a local file store is horizontally scalable" — the
control plane is single-writer. A real transactional HA storage adapter,
signed artifacts, agent OTA, and air-gap OCI bundles are listed in
[`docs/PRODUCT_PLAN.md`](PRODUCT_PLAN.md) as **future** milestones, not
present today. If you need HA today, evaluate accordingly.

**Before going to production, what should I check?** See
[`docs/DEPLOYMENT.md`](DEPLOYMENT.md)'s "Control plane production
checklist" — persistent volume, admin password, session secret, and TLS
are all called out explicitly as things a default/demo setup won't have
configured for you.

## Runtime & platform support

**What can it actually manage?** Four typed runtime adapters:
`systemd`, `container` (Docker/Podman), `k3s`, and `qemu`/KVM — see
[`docs/RUNTIME_ADAPTERS.md`](RUNTIME_ADAPTERS.md). There is deliberately no
generic/arbitrary-shell workload type; every adapter has its own
health-probe gates.

**Does it require Kubernetes?** No — Kubernetes/k3s is one of four runtime
targets, not a requirement to run the control plane itself (which also
ships a Helm chart and raw manifests if you do want it on k8s).

## Offline operation

**What happens when a site loses connectivity?** This is a first-class
design point, not a degraded mode: the agent caches the complete desired
revision, its typed runtime adapter keeps reconciling drift locally, and
operational events queue on disk. "No cloud/control-plane call is
necessary to keep the last accepted desired state alive" (README's
"Offline autonomy" section). Queued events replay on the next heartbeat
once connectivity returns.

## Integration with other Zyvor products

**Does it integrate with Zyvor Device Agent?** Yes, for real — an opt-in
`-device-agent-url` flag (or `ZYVOR_FLEET_DEVICE_AGENT_URL`) merges Device
Agent's namespaced hardware metadata into fleet-agent heartbeats.
Deliberately excludes fields using a different taxonomy than Fleet's own
(hostname/os/arch/kernel/capabilities); a failure to reach Device Agent
never blocks a heartbeat.

**Does it integrate with Nodra?** Not in code today. The README's "Why
this is a separate Zyvor product" table describes Nodra as an "optional
edge-data plane" relationship, but that's architectural positioning, not a
shipped integration — don't assume Fleet consumes Nodra data out of the
box.

## Security

**Can an operator run arbitrary commands on a site through Fleet?** No —
"The agent exposes no arbitrary remote shell" (README). See
[`SECURITY.md`](../SECURITY.md) for the full security model and trust
boundaries.
