# Changelog

## 0.2.0 — 2026-09-06

Production-safety and fleet-operations release.

### Added

- Dynamic site groups with label selectors and pinned members.
- Rollout planning endpoint with target, offline-site and wave counts.
- Strict wave rollouts with configurable inter-wave pauses and failure budgets.
- Approval gates, scheduled start/end windows, pause/resume/abort/retry controls.
- Manual rollback and automatic rollback to captured previous revisions.
- Runtime plus HTTP/TCP workload health probes reported per site.
- Degraded site state and automatic transition from degraded to offline autonomy.
- Prometheus-format `/metrics` endpoint.
- Site metadata/label editing and rollout-safe site deletion protection.
- `fleetctl` group, rollout-plan and rollout lifecycle commands.
- Tagged multi-architecture GHCR image publishing with SBOM/provenance metadata.
- Apple-inspired rollout safety UI with dynamic groups, plan preview, health state and rollback controls.
- Container managed-spec fingerprints covering image, command, environment and ports.

### Security and durability

- Existing sessions are revoked when a user's password changes.
- Session signatures require canonical Base64URL HMAC encoding.
- Control-plane and agent state writes fsync the file and parent directory after atomic rename.
- The agent rejects a cached revision whose body does not match the desired revision ID.
- Repeated persistent workload failures are event-deduplicated locally.
- K3s reconciliation skips unchanged manifest writes.

## 0.1.0 — 2026-09-06

Initial Apache-2.0 release baseline.

### Added

- `fleetd` control plane with embedded Apple-inspired Zyvor web console.
- PBKDF2 password login, signed cookie sessions, admin/operator/viewer authorization and user administration APIs.
- Restricted enrollment tokens exchanged for independent site identities.
- `fleet-agent` inventory, heartbeat, pull sync, full desired-revision cache and local event journal.
- Offline drift reconciliation with typed systemd, container, k3s and opt-in QEMU adapters.
- Declarative revisions and gated wave rollouts.
- `fleetctl` administrative CLI.
- Docker Compose, systemd, raw Kubernetes, Kustomize and Helm deployment assets.
- Unit/integration/race tests, CodeQL, build/release workflows and OpenAPI documentation.
