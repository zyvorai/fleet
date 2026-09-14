---
hero:
  eyebrow: PRODUCTION
  title: Production operations runbook — Zyvor Fleet
---

Companion to [DEPLOYMENT.md](DEPLOYMENT.md) and [V0.3_OPERATIONS.md](V0.3_OPERATIONS.md).
For a multi-product evaluation stack (Fleet + OTA simulator + Device Agent +
Nodra), see [LAB.md](LAB.md) first — that path is not a multi-replica
production control plane.

## Preconditions

1. Software matrix green: `make qualify` → `evidence/qualification/software-matrix.json`.
2. Ops checklist signed: `evidence/qualification/ops-checklist.md`.
3. One control-plane replica only (embedded store).
4. `ZYVOR_FLEET_ADMIN_PASSWORD` and ≥32-byte `ZYVOR_FLEET_SESSION_SECRET` set.
5. HTTPS (direct TLS or trusted ingress with `ZYVOR_FLEET_SECURE_COOKIES=1`).
   Direct TLS is required when Zyvor OTA agents poll this control plane
   (`fleet_url` rejects HTTP).

## Day-2 monitoring

- Scrape `/metrics` from a restricted network path (endpoint is unauthenticated — do not expose publicly).
- Alert on rising offline sites, failed rollouts, webhook `consecutiveFailures`.
- For OTA: watch `/api/v1/ota/devices/{id}/events` ACK lag vs agent outbox.
  ACKs are contiguous — a sequence hole freezes the agent outbox until
  `ackedSequence` is repaired (see [OTA_CONTRACT.md](OTA_CONTRACT.md), [LAB.md](LAB.md)).

## OTA devices

Register devices via `/api/v1/ota/devices`, store the returned token only on the
device (`fleet_token_file`), never in git. Publish signed Assignment JSON with
`PUT …/assignment`. Clear with `DELETE` after terminal events are observed.
Contract details: [OTA_CONTRACT.md](OTA_CONTRACT.md).

## Needs attention (known v0.3 limits)

- No HA / multi-writer store — failover is restore-from-backup.
- Webhook dispatch is opportunistic on API traffic; prefer a health-check poller that hits authenticated APIs if delivery must be prompt.
- Demo mode (`--demo`) and compose stack are evaluation-only.

## Release artifacts

Tag `v*` → GitHub Release binaries (linux amd64/arm64) + GHCR image with SBOM/provenance.
Verify checksums from the release assets; Cosign signs `SHA256SUMS` when the
release workflow completes successfully.
