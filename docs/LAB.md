---
hero:
  eyebrow: LAB
  title: Lab control plane — deploy, OTA contract, Device Agent
---

Recorded 14 September 2026 on `80.79.5.173` (Ubuntu 24.04, user `sus`).
This is an evaluation stack, not a multi-replica production control plane.

## What runs where

| Piece | systemd | URL |
|---|---|---|
| Fleet control plane | `zyvor-fleet.service` | `https://80.79.5.173:18090/` |
| Fleet site agent | `zyvor-fleet-agent.service` | enrolls against the control plane |
| Nodra CP (sibling repo) | `nodra-server.service` | `https://80.79.5.173:18447/` (TLS) |
| Device Agent (sibling) | `zyvor-device-agent.service` | `http://127.0.0.1:9188` |
| Zyvor OTA demo (sibling) | `zyvor-otad-demo.service` | Unix socket on the host |
| relay-edge (sibling) | `relay-edge.service` | `https://80.79.5.173:18086/ui/` |

`:8080` was already Kryton on this machine — pass `--port 18090` (or another
free port) to `scripts/deploy-remote.sh`.

Demo login: `admin@zyvor.local` / password from `/etc/zyvor-fleet/fleet.env`
(`ZYVOR_FLEET_ADMIN_PASSWORD`). As of 2026-09-14 the lab unit runs **without**
`--demo` (`ZYVOR_FLEET_DEMO=0`).

## Deploy

```sh
./scripts/deploy-remote.sh 80.79.5.173 sus --key --port 18090
# Health: GET /readyz
```

After install, enable **direct TLS** so Zyvor OTA can use `fleet_url`
(the OTA client rejects `http://`):

```sh
# cert + key owned by zyvor-fleet, SAN includes 127.0.0.1
fleetd --demo --listen :18090 --data /var/lib/zyvor-fleet/state.json \
  --tls-cert /etc/zyvor-fleet/tls/cert.pem \
  --tls-key /etc/zyvor-fleet/tls/key.pem
```

Trust the same certificate from `fleet-agent` (host CA store or a custom
pool). Without that, enrollment fails with `x509: certificate signed by
unknown authority`.

## OTA contract on this host

Operator (session cookie + `X-Zyvor-Request: 1`, or a scoped API token):

```sh
curl -sk -c cj -b cj -X POST https://127.0.0.1:18090/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@zyvor.local","password":"zyvor-fleet-demo"}'

curl -sk -b cj -H 'X-Zyvor-Request: 1' -H 'Content-Type: application/json' \
  -d '{"deviceId":"NLDW4-4-16-36","name":"lab-host"}' \
  https://127.0.0.1:18090/api/v1/ota/devices
# returns plaintext token once — install as OTA fleet_token_file
```

Device (OTA agent):

- `GET /v1/devices/{device_id}/assignment`
- `POST /v1/devices/{device_id}/events`

Full contract: [OTA_CONTRACT.md](OTA_CONTRACT.md). Event ACKs are the highest
**contiguous** sequence from 1 (or from a repaired `ackedSequence` baseline).
Do not inject a lone `sequence: 1` event if the agent journal already sits
at a much higher `event_sequence`.

Lab result: signed job `lab-wire-signed-2` committed on the OTA simulator;
after aligning `ackedSequence` to 18, Fleet drained the remainder
(`ackedSequence=27`, agent `pending_events=0`).

## Device Agent inventory merge

```sh
fleet-agent \
  --server https://127.0.0.1:18090 \
  --name lab-nldw4 \
  --enrollment-token zf_enroll_demo-local-only \
  --device-agent-url http://127.0.0.1:9188
```

Lab site `lab-nldw4` came `online` with metadata
`zyvor.device_agent.reachable=true` and `zyvor.device.serial=ZY-5206F159C3A4`.

## Qualify vs this lab

`make qualify` is the **software** matrix (unit/race, OTA contract tests,
live-smoke, binaries). This host lab is extra integration evidence. It does
not replace [QUALIFICATION.md](QUALIFICATION.md) ops rows (backup/restore,
WAN-loss soak) or Fleet HA (not in v0.3).
