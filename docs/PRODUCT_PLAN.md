# Zyvor Fleet product plan

Fleet is the site lifecycle and desired-state coordination layer in the Zyvor ecosystem. It stays intentionally separate from Nodra's edge-data path and Fabric's private-cloud VM control plane.

## v0.2.0 — current release

### Fleet operations

- Site enrollment, independent site identity and live inventory.
- Editable site region/labels and reusable dynamic site groups.
- Selector/group/explicit-site rollout targeting plus pre-flight rollout plans.
- Revisions spanning systemd, Docker/Podman, k3s and opt-in QEMU.
- Strict waves: a wave must reach a terminal result before another activates.
- Inter-wave pause, failure budget, approval gate and start/end maintenance window.
- Pause, resume, abort, retry, manual rollback and automatic rollback.
- Per-workload runtime/HTTP/TCP health checks and degraded-site visibility.
- Prometheus-format fleet metrics and operational event stream.

### Offline autonomy

- Complete accepted revision cached at each site.
- Typed drift reconciliation continues without WAN/control-plane connectivity.
- Container drift fingerprint covers image, args, environment and ports.
- Unchanged k3s manifests are not rewritten.
- Connectivity and failure events persist locally and replay after reconnect.
- Agent fails closed if a desired revision ID and cached revision body disagree.

### Security / supply chain

- PBKDF2 password hashes, role-based access and signed HttpOnly sessions.
- Password changes invalidate existing sessions via per-user auth version.
- Canonical session-HMAC verification and same-origin mutation marker.
- Hashed enrollment/site tokens, 0600 state files and bounded request bodies.
- CI, race tests, CodeQL, multi-architecture tagged releases, SBOM/provenance workflow.

## v0.3 candidates

- Site maintenance/cordon mode with explicit rollout override.
- Scoped API tokens for CI/GitOps automation.
- HMAC-signed outgoing event webhooks with durable delivery cursors.
- Bounded mutation audit trail for human and API-token operations.
- Signed desired-state/artifact bundles with policy-controlled trust roots.
- Agent self-update with staged channels and rollback.
- Air-gap OCI bundle export/import and local registry mirroring.
- OpenTelemetry export and richer SLO/rollout analytics.
- Fleet-to-Nodra, PacketWolf, Relay, Argus, Forge and HyperCluster integration adapters.
- OIDC/SAML enterprise identity and token federation/rotation.
- More runtime adapters through a versioned provider interface.

## v1.0 direction

- Transactional HA storage adapter with leader-safe rollout coordination.
- Multi-control-plane disaster recovery and tested backup/restore tooling.
- Signed policy/artifact promotion across disconnected regions.
- Enterprise audit export and integration-driven rollout verification.

The embedded file store remains deliberately single-writer until a real transactional HA backend exists. Fleet does not claim horizontal control-plane HA in v0.2.
