# Fleet ops / lab qualification checklist

**Status:** signed for single-writer lab host `80.79.5.173` — production posture
(`ZYVOR_FLEET_DEMO=0`, no `--demo` on ExecStart, direct TLS). Does **not** claim HA
or multi-day soak.

| Field | Value |
|---|---|
| Operator | lab-ops-drill |
| Date (UTC) | 2026-09-14 |
| fleetd version | 0.3.0 |
| State file backup hash | `cc890dddf7e6eca222360ebf9d5b07f073b5117e49bb9568439bb6d87cb2e5f7` |
| Environment | bare systemd `zyvor-fleet.service` + direct TLS; `ZYVOR_FLEET_DEMO=0` |
| Evidence | `evidence/qualification/lab/20260914T155128Z/`, `…/20260914T162245Z/` |

## Results

| Test | Result | Evidence path |
|---|---|---|
| Persistent volume bootstrap | pass | live state; `/readyz` ready |
| Backup and restore (live volume) | pass | `lab/20260914T155128Z/` |
| Backup restore-drill (software) | pass | restore-drill digest match |
| TLS / secure cookies | pass | HTTPS login |
| Enrollment token hygiene | pass | existing enroll token; agents drop after sync |
| Offline autonomy (WAN cut) | pass | `lab/20260914T162245Z/fleet-wan-loss.log` — CP stop, agent kept running, CP ready in 2s |
| Rollout failure budget + auto rollback | pass | covered by `scripts/live-smoke.py` / `make qualify` `live_smoke` |
| OTA device ↔ zyvor-otad lab | pass | `docs/LAB.md` |
| Single-replica discipline confirmed | pass | one `fleetd` |
| Non-demo ExecStart | pass | `fleetd --listen …` without `--demo`; `ZYVOR_FLEET_DEMO=0` |

## Sign-off

- Name: lab-ops-drill
- Notes: Admin password remains the prior lab credential in `/etc/zyvor-fleet/fleet.env`
  (rotate for customer prod). No HA claim.
