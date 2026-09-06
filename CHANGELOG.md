# Changelog

## Unreleased

### Web console

- Full visual redesign of the embedded web console: light theme by default,
  toggleable dark theme (proper Apple-system dark colors, not just an
  inverted palette), collapsible sidebar, and a two-step (email, then
  password) sign-in flow with a "remember this device" option.
- Settings page integrations, operator management and enrollment-token
  actions are now fully wired end-to-end (enable/disable and configure an
  integration, edit/delete an operator, revoke a token) — these existing
  backend endpoints previously had no UI affordance.
- Added a "Delete site" action to the site detail drawer and a "Remove"
  action on dynamic site groups, closing the last unreachable-from-the-UI
  admin actions (`DELETE /api/v1/sites/{id}`, `DELETE
  /api/v1/site-groups/{id}`).
- Mobile sidebar drawer now has a dimming backdrop that's tappable to
  dismiss, instead of opening with no scrim.
- Added `aria-label`s to all modal/drawer close buttons.

### Fixed

- Empty API list responses (Go's `nil` slice serializes as JSON `null`)
  were crashing the Rollouts and Events pages; normalized once at the
  client's `api()` boundary instead of guarding every call site.
- `GET /api/v1/enrollment-tokens` returned untagged, capitalized JSON keys
  inconsistent with every other endpoint; added proper `json:` tags.
- `PATCH /api/v1/users/{id}` allowed an admin to demote their own account
  away from admin even when other admins existed; blocked explicitly.

### Testing

- Added test coverage for `internal/agent`'s reconciliation loop
  (`runner.go`) — previously untested despite being the offline-autonomy
  core the product is named for. Covers first-enrollment, the
  local-autonomy reconciliation path when the control plane is
  unreachable, reconnect handling, the desired-revision mismatch
  fail-closed guard, unsupported-command acking, and revision
  apply/fail/dedup behavior.

### Deployment

- Added the safe subset of systemd sandboxing directives to the edge
  agent's unit file (`ProtectHome`, `ProtectKernelTunables`,
  `ProtectControlGroups`, `MemoryDenyWriteExecute`, etc.), documenting why
  `ProtectSystem`/capability restrictions are deliberately left to the
  operator since runtime adapters need real local privilege.
- Added resource requests/limits to the Helm chart's agent DaemonSet.

### Project

- Repository and Go module path renamed from `zyvor-fleet` to
  `github.com/zyvorai/fleet`.

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
