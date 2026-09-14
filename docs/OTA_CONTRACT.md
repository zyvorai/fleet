---
hero:
  eyebrow: OTA CONTRACT
  title: Zyvor OTA adapter contract (Fleet server)
---

Fleet implements the server side of the Zyvor OTA Fleet adapter contract
documented in `zyvor-ota` as `docs/FLEET.md`. Device identity is bound to a
digest-stored bearer token; the path alone never authorizes.

## Device endpoints (agent)

| Method | Path | Auth | Behavior |
|---|---|---|---|
| `GET` | `/v1/devices/{device_id}/assignment` | Bearer device token | `200` assignment JSON, `204` none |
| `POST` | `/v1/devices/{device_id}/events` | Bearer device token | Persist ≤100 events; ACK contiguous `{"sequence":N}` |

Redirects are not issued. Use HTTPS in production (`--tls-cert`/`--tls-key` or ingress).

## Operator endpoints (UI / API token)

| Method | Path | Role |
|---|---|---|
| `GET` | `/api/v1/ota/devices` | admin, operator |
| `POST` | `/api/v1/ota/devices` | admin — returns plaintext token **once** |
| `DELETE` | `/api/v1/ota/devices/{device_id}` | admin |
| `PUT` | `/api/v1/ota/devices/{device_id}/assignment` | admin, operator — Assignment JSON body |
| `DELETE` | `/api/v1/ota/devices/{device_id}/assignment` | admin, operator |
| `GET` | `/api/v1/ota/devices/{device_id}/events` | admin, operator, viewer |

## Rollout policy stays in Fleet

Canary waves, observation windows and pause-on-uncertainty remain Fleet
operator decisions. OTA only verifies and installs. Point `zyvor-otad`
`fleet_url` at this control plane and provision `fleet_token_file` with the
device token returned at registration.

## Lab quick check

```bash
make build
# after control plane is up and you are logged in / hold an admin API token:
curl -sk -H "Authorization: Bearer $ADMIN_API_TOKEN" -H 'Content-Type: application/json' \
  -H 'X-Zyvor-Request: 1' \
  -d '{"deviceId":"minewing-gw1-lab-001","name":"lab"}' \
  https://fleet.example/api/v1/ota/devices
```

Automated coverage: `go test ./internal/server/ -run OTA` and `make qualify`.

## Contiguous event ACK

`POST /v1/devices/{id}/events` persists events and returns the highest
**contiguous** sequence Fleet has stored from the device baseline
(`ackedSequence`). Gaps are not skipped: if the agent outbox starts at
sequence 10 but Fleet previously accepted a synthetic `sequence: 1` with
nothing in between, the agent will retain `pending_events` until an operator
aligns `ackedSequence` with the real contiguous prefix (or drains/repairs the
outbox under an audited process).

Recorded lab repair: after aligning `ackedSequence` to 18, Fleet drained
through 27 and the OTA simulator reported `pending_events=0`
([LAB.md](LAB.md)).

Zyvor OTA requires HTTPS for `fleet_url`. Prefer `--tls-cert`/`--tls-key` on
`fleetd` (or a trusted ingress) and provision `fleet_ca` / host CA trust on
the agent host.