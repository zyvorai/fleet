# Zyvor Fleet architecture

## Scope

Fleet owns the lifecycle of **sites** and their desired workload state. It is deliberately not an IoT event broker, a Kubernetes distribution, a VM hypervisor, or a remote-shell product.

The core invariants are:

1. A site initiates all control-plane communication.
2. An enrollment token is not a permanent site identity.
3. A site caches the complete accepted desired revision.
4. Losing the WAN does not erase desired state or stop local reconciliation.
5. Server-originated desired state is typed; there is no arbitrary shell command primitive.
6. Rollout progression is based on agent-reported application of a revision.
7. v0.2 embedded persistence is single-writer and is deployed honestly as one replica.

## Components

### `fleetd`

One Go process provides:

- embedded static web console;
- human authentication and RBAC;
- site enrollment exchange;
- site identity authentication;
- inventory and heartbeat ingestion;
- revision storage;
- rollout wave coordination;
- operational event retention;
- persistent state.

The HTTP server uses explicit timeouts and maximum header/body sizes. Browser security headers are applied centrally.

### `fleet-agent`

The edge process provides:

- first-run site enrollment;
- per-site credential persistence;
- platform/runtime inventory;
- pull-based desired-state sync;
- local desired-state cache;
- drift reconciliation through typed runtime adapters;
- local operational event queue;
- heartbeat and event replay.

### `fleetctl`

A small administrative CLI authenticates through the same human session API and can read fleet state or create enrollment tokens.

## Protocol

### Enrollment

```text
operator creates enrollment token
           │
           ▼
 fleet-agent POST /agent/register
 Authorization: Bearer <enrollment>
           │
           ▼
 fleetd validates hash / expiry / use count
           │
           ├─ increments token use
           ├─ creates site record
           └─ returns site ID + new random site token
                              │
                              ▼
                  agent stores site identity locally
```

Only the hash of the enrollment token and the hash of the final site token are stored centrally.

### Steady-state sync

```text
site                                     fleetd
 │                                         │
 ├── GET /agent/sync ─────────────────────►│
 │   site ID + bearer identity             │
 │◄── desired revision + typed commands ───┤
 │                                         │
 ├── reconcile cached revision             │
 │                                         │
 ├── POST /agent/heartbeat ────────────────►│
 │   inventory + applied revision + events │
 │◄── OK ──────────────────────────────────┤
```

The server does not open a connection to the edge host.

## Failure behavior

### Control plane unavailable

The agent records a single connectivity transition and continues reconciling `CachedRevision`. It does not need a successful heartbeat to keep local workloads converged.

### Agent restart while offline

The site credential, cached revision, last applied revision and queued events are stored atomically in the local state file. After restart, the agent can continue local reconciliation before control-plane connectivity returns.

### Site reconnect

The next successful sync clears local connectivity-loss state. A reconnect event is queued, the newest server revision is accepted, and the next heartbeat replays queued events.

### Server restart

Server state is persisted with atomic temporary-file + rename updates. Session continuity depends on a stable `ZYVOR_FLEET_SESSION_SECRET`.

## Rollout algorithm

A rollout has resolved target sites, strict wave size, optional inter-wave pause, failure budget, approval gate and maintenance window.

1. Targets are resolved from explicit sites, a dynamic group, or a label selector.
2. An optional approval/start gate prevents activation.
3. Exactly one wave is activated; no later wave starts while any activated site is non-terminal.
4. Agents report either the requested applied revision or a failed revision/health result.
5. Exceeding `maxFailures` fails the rollout. With `autoRollback`, every activated site is returned to its captured previous revision.
6. Successful waves honor `pauseSeconds` before the next wave. Operators can pause, resume, abort, retry or manually roll back.
7. The rollout completes only when all targets are terminal within the configured failure budget.

Offline sites never falsely complete. They retain the prior cached desired state until they can receive and accept the newer revision.

## Persistence

v0.2 uses a transaction-like in-process store:

- writers hold an exclusive mutex;
- mutation runs against a deep clone;
- a callback error discards the mutation;
- unchanged transactions avoid disk writes;
- changed state is written mode `0600` to a temporary file and atomically renamed.

This design is easy to audit and suitable for the initial OSS control plane, but it is not a horizontally replicated database. Helm deliberately sets one replica.

## Trust boundaries

### Browser / operator

Human login produces a signed session token placed in an HttpOnly cookie. Authorization uses `admin`, `operator`, and `viewer` roles. Mutating cookie API calls also require `X-Zyvor-Request: 1`, reducing cross-site request abuse alongside SameSite Strict cookies.

### Site

Each site has a high-entropy random token. It is sent only as a bearer credential over the configured transport and stored as a hash centrally.

### Runtime adapter

The server sends a structured `RuntimeSpec`, not command strings. The agent maps that structure to specific local operations. Workload names are validated before they enter paths or command arguments.

## Zyvor suite integration boundary

Fleet should integrate through adapters/events rather than absorb adjacent products:

- Nodra: edge data and device plane.
- PacketWolf: network evidence and policy validation.
- Relay: durable action workflows.
- Argus: health and application verification gates.
- Forge: GPU/inference status and rollout-aware capacity.
- HyperCluster: cluster lifecycle action provider.
- IronWolf: physical lifecycle provider.

That keeps Fleet small enough to remain a reliable site control plane.
