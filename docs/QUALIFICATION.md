---
hero:
  eyebrow: QUALIFICATION
  title: Production qualification matrix — Zyvor Fleet
---

Software rows are automated by `make qualify`. Lab ops rows for host
`80.79.5.173` are **signed** in
[`evidence/qualification/ops-checklist.md`](https://github.com/zyvorai/fleet/blob/main/evidence/qualification/ops-checklist.md)
(backup/TLS/non-demo/abbreviated WAN). Multi-site soak and HA remain open.

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
backup/restore integrity. They do **not** prove HA (v0.3 is single-writer)
or multi-day soak.

## Cross-product lab rows (optional evidence)

Recorded on a shared Linux host — see [LAB.md](LAB.md):

| ID | Expected |
|---|---|
| `remote_smoke_deploy` | `deploy-remote.sh` health/ready on chosen port (avoid `:8080` collisions) |
| `ota_tls_assignment_commit` | HTTPS Fleet + OTA simulator reaches `committed` |
| `ota_event_ack_drain` | Contiguous ACK clears agent `pending_events` |
| `device_agent_inventory_merge` | Site metadata includes `zyvor.device_agent.*` |

## Operator / lab rows — checklist status

Evidence: `ops-checklist.md`, `lab/20260914T155128Z/`, `lab/20260914T162245Z/`.

| Test | Lab status |
|---|---|
| Persistent volume bootstrap | **pass** (signed) |
| Backup and restore (live + restore-drill) | **pass** (signed) |
| TLS termination | **pass** (signed) |
| Enrollment token hygiene | **pass** (signed) |
| Offline autonomy (abbreviated WAN cut) | **pass** — `scripts/ci/wan-loss-drill.sh` / `fleet-wan-loss.log` |
| Rollout failure budget | **pass** — covered by `live_smoke` auto-rollback |
| OTA device assignment (simulator lab) | **pass** — see LAB.md |
| Single-replica discipline | **pass** (signed) |
| Non-demo ExecStart | **pass** — `ZYVOR_FLEET_DEMO=0` |
| Multi-site / multi-day WAN soak | **open** |
| HA / multi-writer | **not available** in v0.3 |

## Maturity note

v0.3 does not provide transactional HA storage. Production is one control-plane
replica with a tested backup. See [PRODUCT_PLAN.md](PRODUCT_PLAN.md) and
[PRODUCTION.md](PRODUCTION.md).

## GitHub CI (lab substitute)

CI runs compose smoke, HTTPS TLS smoke, backup/restore, container build, and
govulncheck. These complement (do not replace) the signed lab ops checklist.
CI still does **not** claim HA, multi-site soak, or Minewing device HIL.
