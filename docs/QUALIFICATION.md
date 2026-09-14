---
hero:
  eyebrow: QUALIFICATION
  title: Production qualification matrix — Zyvor Fleet
---

Software rows are automated by `make qualify`. Multi-site WAN-loss drills and
backup/restore sign-off remain operator-recorded in
[`evidence/qualification/ops-checklist.md`](../evidence/qualification/ops-checklist.md).

## Software (host) rows — `make qualify`

| ID | Expected |
|---|---|
| `unit_race_vet` | `gofmt`, `go vet`, `go test -race ./...` |
| `web_js_syntax` | `node --check webui/static/app.js` |
| `build_binaries` | `fleetd`, `fleet-agent`, `fleetctl` build |
| `live_smoke` | Real control-plane + agent enrollment/offline/reconnect drill |
| `ota_contract` | Device assignment + contiguous event ACK (`TestOTAContract*`) |
| `backup_restore_drill` | `scripts/restore-drill.sh` checksum round-trip |

These prove control-plane, agent smoke, the OTA contract, and scripted
backup/restore integrity. They do **not** prove HA (v0.3 is single-writer),
signed artifact policy, multi-day soak, or a live PVC stop→restore→start
(that remains an operator row).

## Cross-product lab rows (optional evidence)

Recorded on a shared Linux host — see [LAB.md](LAB.md):

| ID | Expected |
|---|---|
| `remote_smoke_deploy` | `deploy-remote.sh` health/ready on chosen port (avoid `:8080` collisions) |
| `ota_tls_assignment_commit` | HTTPS Fleet + OTA simulator reaches `committed` |
| `ota_event_ack_drain` | Contiguous ACK clears agent `pending_events` |
| `device_agent_inventory_merge` | Site metadata includes `zyvor.device_agent.*` |

## Operator / lab rows — signed checklist

| Test | Required outcome |
|---|---|
| Persistent volume bootstrap | State survives restart; admin login works |
| Backup and restore | Restored state file yields identical sites/revisions; record digest from `backup-state.sh` / `restore-drill.sh` |
| TLS termination | Secure cookies / direct TLS as documented |
| Enrollment token hygiene | Short-lived, low maxUses; agent drops enroll token after first sync |
| Offline autonomy | WAN cut during apply; agent reconciling from cache |
| Rollout failure budget | Failed health gate → automatic rollback |
| OTA device assignment | Real `zyvor-otad` pulls assignment and ACKs events (lab) |
| Single-replica discipline | No second `fleetd` against the same volume |

## Maturity note

v0.3 does not provide transactional HA storage. Production is one control-plane
replica with a tested backup. See [PRODUCT_PLAN.md](PRODUCT_PLAN.md).
