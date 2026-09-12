---
hero:
  eyebrow: TESTING
  title: Testing
---

## Release gate

Run from the repository root:

```bash
./scripts/test-all.sh
```

The gate verifies:

- `gofmt` cleanliness;
- `go vet ./...`;
- `go test -race ./...`;
- JavaScript syntax with `node --check` when Node is installed;
- builds of `fleetd`, `fleet-agent` and `fleetctl`.

## Automated coverage

The Go suite exercises:

- PBKDF2 password verification and signed-session tamper rejection;
- canonical HMAC session encoding and password-change session revocation;
- store rollback, atomic persistence and agent-state persistence;
- embedded UI/security headers, login/RBAC and same-origin mutation protection;
- enrollment tokens and independent per-site credentials;
- dynamic groups/selectors and rollout plan resolution;
- approval, scheduled/strict waves, inter-wave pauses and lifecycle actions;
- failure budgets, automatic rollback and retry;
- deletion protection for sites participating in active rollouts;
- runtime, HTTP and TCP health probes;
- container desired-spec drift fingerprints and k3s reconciliation;
- Prometheus metrics output.

## Manual/live release smoke

`make live-smoke` runs a real `fleetd` and `fleet-agent` against temporary state. It validates enrollment, dynamic-group planning, baseline revision application, a deliberately failed HTTP health gate with automatic rollback, full control-plane shutdown, offline K3s drift repair from cached state, reconnect, queued-event replay and `/metrics`. CI runs this built-binary drill after the race/unit gate.

For v0.2.0 the same live drill passed on 2026-09-06. The source ZIP is additionally extracted into a clean temporary directory and the release gate is run from the extracted copy before publication.
