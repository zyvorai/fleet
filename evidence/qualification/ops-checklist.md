# Fleet ops / lab qualification checklist

**Status:** signed for single-writer lab host `80.79.5.173` (evaluation stack with `--demo`).
This closes backup/TLS/single-replica ops rows for that host. It does **not** claim HA,
multi-day soak, or a non-demo production control plane.

| Field | Value |
|---|---|
| Operator | lab-ops-drill (suite CI follow-up) |
| Date (UTC) | 2026-09-14 |
| fleetd version | 0.3.0 (`e50cfeb`) |
| State file backup hash | `cc890dddf7e6eca222360ebf9d5b07f073b5117e49bb9568439bb6d87cb2e5f7` |
| Environment (compose/k8s/bare) | bare systemd `zyvor-fleet.service` + direct TLS |
| Evidence | `evidence/qualification/lab/20260914T155128Z/` |

## Results

| Test | Result (pass/fail/blocked) | Evidence path |
|---|---|---|
| Persistent volume bootstrap | pass | live `/var/lib/zyvor-fleet/state.json`; `/readyz` ready after prior restarts |
| Backup and restore (live volume) | pass | backup archive sha256 `0d061c22…`; restore→temp `fleetd` on `:19091` → `ready` |
| Backup restore-drill (software) | pass | `restore-drill.log` digest match |
| TLS / secure cookies | pass | HTTPS `:18090` login → admin session (`tls-login.json`) |
| Enrollment token hygiene | pass | lab uses short-lived demo enroll token; agents drop after sync (LAB.md) |
| Offline autonomy (WAN cut) | blocked | not drilled this pass — remains operator soak |
| Rollout failure budget + auto rollback | blocked | not drilled this pass |
| OTA device ↔ zyvor-otad lab | pass | see `docs/LAB.md` (assignment commit + ACK drain) |
| Single-replica discipline confirmed | pass | one `fleetd` against `/var/lib/zyvor-fleet` |

## Sign-off

- Name: lab-ops-drill
- Notes commit SHA: (this commit)
- Limits: host still runs `--demo` / `ZYVOR_FLEET_DEMO=1` for evaluation login. Production
  installs must unset demo and set strong `ZYVOR_FLEET_ADMIN_PASSWORD` +
  `ZYVOR_FLEET_SESSION_SECRET` per [docs/PRODUCTION.md](../../docs/PRODUCTION.md).
