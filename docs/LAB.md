---
hero:
  eyebrow: LAB
  title: Lab control plane — deploy, OTA contract, Device Agent
---

Recorded 14–15 September 2026 on `80.79.5.173` (Ubuntu 24.04, user `sus`).
Single-writer evaluation stack with **production posture** on this host
(non-demo Fleet, HTTPS). Not a multi-replica HA control plane.

## What runs where

| Piece | systemd | URL / notes |
|---|---|---|
| Fleet control plane | `zyvor-fleet.service` | `https://80.79.5.173:18090/` — `ZYVOR_FLEET_DEMO=0`, no `--demo` |
| Fleet site agent | `zyvor-fleet-agent.service` | enrolls against the control plane |
| Nodra CP (sibling) | `nodra-server.service` | `https://80.79.5.173:18447/` (TLS) |
| Device Agent (sibling) | `zyvor-device-agent.service` | `http://127.0.0.1:9188` — **lab-surrogate only** |
| Zyvor OTA demo (sibling) | `zyvor-otad-demo.service` | Unix socket; simulator backend |
| relay-edge (sibling) | `relay-edge.service` | `https://80.79.5.173:18086/ui/` — `EDGE_REQUIRE_AUTH=1` |

`:8080` was already Kryton — pass `--port 18090` to `scripts/deploy-remote.sh`.

Login: `admin@zyvor.local` / password from `/etc/zyvor-fleet/fleet.env`
(`ZYVOR_FLEET_ADMIN_PASSWORD`). Rotate before shared use.

## Deploy

```sh
./scripts/deploy-remote.sh 80.79.5.173 sus --key --port 18090
# Health: GET /readyz
```

Ensure **non-demo** + **direct TLS** so Zyvor OTA can use `fleet_url`
(the OTA client rejects `http://`):

```sh
# Production-shaped lab unit (no --demo):
# ExecStart=/usr/local/bin/fleetd --listen :18090 --data /var/lib/zyvor-fleet/state.json \
#   --tls-cert /etc/zyvor-fleet/tls/cert.pem --tls-key /etc/zyvor-fleet/tls/key.pem
# Environment: ZYVOR_FLEET_DEMO=0
```

Trust the same certificate from `fleet-agent` (host CA store or a custom
pool). Without that, enrollment fails with `x509: certificate signed by
unknown authority`.

## OTA contract on this host

```sh
curl -sk -c cj -b cj -X POST https://127.0.0.1:18090/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"admin@zyvor.local\",\"password\":\"$ZYVOR_FLEET_ADMIN_PASSWORD\"}"

curl -sk -b cj -H 'X-Zyvor-Request: 1' -H 'Content-Type: application/json' \
  -d '{"deviceId":"NLDW4-4-16-36","name":"lab-host"}' \
  https://127.0.0.1:18090/api/v1/ota/devices
```

Device (OTA agent): `GET /v1/devices/{id}/assignment`, `POST …/events`.
Full contract: [OTA_CONTRACT.md](OTA_CONTRACT.md). ACKs are contiguous.

Lab result: signed job committed on the OTA simulator; after aligning
`ackedSequence`, Fleet drained the outbox (`pending_events=0`).

## Device Agent inventory merge

```sh
fleet-agent \
  --server https://127.0.0.1:18090 \
  --name lab-nldw4 \
  --enrollment-token … \
  --device-agent-url http://127.0.0.1:9188
```

Site `lab-nldw4` came `online` with `zyvor.device_agent.reachable=true`.
Device Agent on this host is **x86 lab-surrogate** — not Minewing HIL.

## Ops evidence on this host

| Drill | Evidence |
|---|---|
| Backup / restore / TLS / non-demo | [ops-checklist.md](https://github.com/zyvorai/fleet/blob/main/evidence/qualification/ops-checklist.md) |
| Abbreviated WAN cut | `evidence/qualification/lab/20260914T162245Z/fleet-wan-loss.log` |
| Sibling Nodra TLS + WAN/disk | Nodra `ops-checklist.md` |
| Sibling relay-edge auth+TLS | relay-edge `ops-checklist.md` |

## Qualify vs this lab

`make qualify` is the **software** matrix. This host also carries **signed**
ops rows (see [QUALIFICATION.md](QUALIFICATION.md)). It does **not** claim
Fleet HA or multi-day multi-site soak.
