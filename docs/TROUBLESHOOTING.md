---
hero:
  eyebrow: TROUBLESHOOTING
  title: Troubleshooting
---

Real operational issues, with the documented fix — not a generic checklist.
If your symptom isn't here, check [`ARCHITECTURE.md`](https://github.com/zyvorai/fleet/blob/main/ARCHITECTURE.md)
and [`docs/DEPLOYMENT.md`](DEPLOYMENT.md), then
[open an issue](https://github.com/zyvorai/fleet/issues).

## A site keeps running its old desired state after I pushed a new one

Expected if the site is offline: the agent's cached revision stays
authoritative locally until the control plane is reachable again. Per the
README's "Offline autonomy" section, the sequence on reconnect is: (1) the
agent's connectivity transition is marked, (2) queued operational events
replay on the next heartbeat, then (3) any newer desired revision is
reconciled. If a site has been offline a while, allow for that heartbeat
cycle rather than assuming the push failed.

## A rollout looks stuck / a runtime adapter won't apply a workload

Check the relevant adapter's health-probe gates first —
[`docs/RUNTIME_ADAPTERS.md`](RUNTIME_ADAPTERS.md)'s "Health probes"
section applies across all four adapter types (`systemd`, `container`,
`k3s`, `qemu`), each with its own gating rules. A workload that never
becomes healthy per its adapter's probe will correctly block a rollout
from proceeding rather than silently marking it done.

## Trying to run a second control-plane replica for HA

Not supported today — [`docs/DEPLOYMENT.md`](DEPLOYMENT.md#why-one-replica)
and [`ARCHITECTURE.md`](https://github.com/zyvorai/fleet/blob/main/ARCHITECTURE.md) are explicit that v0.3's
storage is single-writer; a transactional HA storage adapter is a future
milestone in [`docs/PRODUCT_PLAN.md`](PRODUCT_PLAN.md), not present yet.
Running two replicas against the same store isn't a supported
configuration — plan around a single control-plane instance for now.

## Fresh control-plane install looks insecure / missing config

Check [`docs/DEPLOYMENT.md`](DEPLOYMENT.md)'s "Control plane production
checklist" first — a default/local-evaluation setup deliberately doesn't
configure a persistent volume, admin password, session secret, or TLS for
you. This is a checklist to complete before going to production, not a
default hardening posture.

## Device Agent metadata isn't showing up in fleet-agent heartbeats

Confirm `-device-agent-url` (or `ZYVOR_FLEET_DEVICE_AGENT_URL`) is actually
set on the agent — the integration is opt-in, off by default. Also note
only namespaced `zyvor.*` metadata is merged; hostname/os/arch/kernel/
capabilities fields are deliberately excluded (they use a different
taxonomy than Fleet's own), so their absence from a heartbeat is expected,
not a bug. A Device Agent that's unreachable never blocks a heartbeat
either — check Fleet's own logs, not just Device Agent's, if metadata is
silently missing.

## Trying to get Nodra data through Fleet

There is no code integration between Fleet and Nodra in this repository —
the "optional edge-data plane" relationship in the README's product table
is architectural positioning, not a built pipe. Don't configure Fleet
expecting it to surface Nodra telemetry; that's not implemented.

## Nothing here matches

Check [`ARCHITECTURE.md`](https://github.com/zyvorai/fleet/blob/main/ARCHITECTURE.md) for the full failure model,
then [open an issue](https://github.com/zyvorai/fleet/issues) with your
`fleetctl` output and relevant logs (redact credentials/tokens).
