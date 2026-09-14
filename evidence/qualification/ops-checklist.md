# Fleet ops / lab qualification checklist

**Status:** unsigned / not claimed by `make qualify`.

| Field | Value |
|---|---|
| Operator | |
| Date (UTC) | |
| fleetd version | |
| State file backup hash | |
| Environment (compose/k8s/bare) | |

## Results

| Test | Result (pass/fail/blocked) | Evidence path |
|---|---|---|
| Persistent volume bootstrap | | |
| Backup and restore (live volume) | | |
| Backup restore-drill (software) | | `make qualify` / `scripts/restore-drill.sh` |
| TLS / secure cookies | | |
| Enrollment token hygiene | | |
| Offline autonomy (WAN cut) | | |
| Rollout failure budget + auto rollback | | |
| OTA device ↔ zyvor-otad lab | | |
| Single-replica discipline confirmed | | |

## Sign-off

- Name:
- Notes commit SHA:
