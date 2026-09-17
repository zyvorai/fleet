---
hero:
  eyebrow: HA
  title: High availability design (not shipped)
---

**Status: design only.** Zyvor Fleet v0.3 ships a single-writer embedded
`state.json` store. This document outlines how a future Postgres-backed HA
mode would work. Nothing here is implemented or claimed as production HA.

## What exists today

| Concern | v0.3 behavior |
|---|---|
| Control-plane replicas | Exactly one writer (`deploy/helm` fixes `replicas: 1`) |
| Persistence | Atomic rewrite of a local JSON file (`internal/store`) |
| Failover | Restore-from-backup onto one new `fleetd` ([PRODUCTION.md](PRODUCTION.md)) |
| Agents during CP outage | Continue offline autonomy from cached revision; reconnect later |
| Soak / WAN drills | Single-writer resilience only — [scripts/ci/soak-short.sh](../scripts/ci/soak-short.sh) |

Running two `fleetd` processes against the same file (or a `ReadWriteMany`
volume) is **not** a supported HA configuration — it races the store and
corrupts desired state.

## Target shape (future)

A Postgres-backed store would externalize durable control-plane state so
multiple `fleetd` replicas can share one database:

```
                    ┌─────────────┐
   agents / UI ────►│  fleetd ×N  │──┐
                    └─────────────┘  │
                                     ▼
                              ┌────────────┐
                              │  Postgres  │
                              └────────────┘
```

### Store responsibilities to move off the local file

- Sites, groups, revisions, rollouts, enrollment tokens, API tokens
- Users / password hashes, webhook configs + delivery cursors
- OTA device registry, assignments, contiguous event ACK cursors
- Audit log (append-only)

Keep process-local: session HMAC secrets (or move to a shared secret),
in-memory rate limiters, and Prometheus scrapes.

### Consistency model

1. **Strong reads/writes for desired state** — every mutating API call runs
   inside a DB transaction. Rollout wave advances, revision publishes, and
   OTA `ackedSequence` updates must be conflict-safe under concurrent
   writers (row-level locks or conditional `UPDATE … WHERE version = $n`).
2. **Leader-safe rollout worker (optional first step)** — elect a single
   active reconciler via a Postgres advisory lock / lease so wave timers and
   auto-rollback do not double-fire. This is *single-active-writer failover*,
   not multi-writer HA, and must be labeled as such in qualification.
3. **True multi-writer path (later)** — claimable work items for rollout
   ticks and webhook delivery (compare Nodra's `PostgresQueue` claim lease
   pattern) so any replica can process work without an elected leader.

### Migration sketch

1. Introduce `ZYVOR_FLEET_STORE=file|postgres` (default `file`).
2. Schema + migrations for the tables above; keep JSON schema versioning
   for export/import compatibility with existing backups.
3. CI job against Postgres 16 (gate `go test -run Postgres`).
4. Helm: allow `replicas > 1` **only** when `store=postgres`; keep
   `Recreate` + one replica for file mode.
5. Document ops: connection pooling, failover of the DB itself, backup via
   `pg_dump` in addition to (or instead of) `scripts/backup-state.sh`.

## Explicit non-goals for the first HA milestone

- Multi-region active/active with conflict-free CRDTs
- Sharing one SQLite/file via NFS and calling it HA
- Claiming HA from soak scripts alone — soak proves reconnect/resilience
  under a single writer, not multi-replica correctness

## Tracking

| Item | Status |
|---|---|
| Abbreviated WAN/disk soak (CI scheduled) | **landed** — `scripts/ci/soak-short.sh`, `.github/workflows/soak.yml` (single-writer reconnect; not HA) |
| Longer optional soak script | **landed** — `scripts/ci/soak.sh` + `scripts/ci/soak-check.py` (still single-writer; does not close multi-day lab soak) |
| Multi-hour / multi-day soak on lab host | **open** — needs self-hosted runner |
| Postgres store adapter | **not started** |
| Multi-writer / leader-safe rollout coordination | **not started** |
| Production HA claim | **not available** |

See [QUALIFICATION.md](QUALIFICATION.md), [PRODUCTION.md](PRODUCTION.md), and
[PRODUCT_PLAN.md](PRODUCT_PLAN.md) v1.0 direction.
