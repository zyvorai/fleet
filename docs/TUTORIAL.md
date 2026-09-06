# Getting started with Zyvor Fleet

This is a hands-on walkthrough: stand up a demo fleet, tour the web console,
declare desired state, promote it in a rollout, and see the CLI equivalent
of everything you clicked. It complements the terse [README](../README.md)
quick start rather than replacing it — start here if you want the guided
version.

By the end you'll have a running control plane, two enrolled "edge" sites,
a revision applied across both, and a working mental model of how Fleet
behaves when a site's connection drops.

## 1. Start a demo fleet

The fastest path uses Docker Compose: one command gives you a control plane
plus two already-enrolled simulated edge sites, so you're not starting from
an empty dashboard.

```bash
git clone https://github.com/zyvorai/fleet.git
cd fleet
docker compose up
```

Wait for the log line confirming the control plane is healthy, then open
**http://localhost:8080**. The other two containers (`edge-a`, `edge-b`)
are real `fleet-agent` processes enrolling themselves as `factory-pune-01`
and `retail-delhi-07` — give it up to a minute or so (the control plane
needs to report healthy before the agents start, then each enrolls and
sends its first heartbeat) and both should show up as online in the
console.

Prefer to build locally instead of Docker? See the README's
[Quick start](../README.md#quick-start) for the `make build` + `./bin/fleetd
--demo` + manual `fleet-agent` path — everything below works the same way
either way.

## 2. Sign in

Zyvor Fleet's sign-in is two steps, like signing into an Apple ID: enter
your email, then your password.

1. Email: `admin@zyvor.local`
2. Continue
3. Password: `zyvor-fleet-demo`
4. Check "Remember me on this device" if you want the console to skip
   straight to the password step next time you load the page.
5. Sign In

You'll land on **Overview** — the fleet topology view, a live map of every
enrolled site radiating out from the control plane, plus at-a-glance
counts (sites, online, autonomy, running rollouts) and a recent-events
feed.

Two things worth knowing about the console itself:

- It defaults to a light theme; the moon/sun icon (top right) switches to
  a dark theme, which is remembered per browser.
- The sidebar can collapse to icons-only via the chevron at its bottom —
  useful on a smaller screen.

## 3. Look at your sites

Click **Sites**. You should see `factory-pune-01` and `retail-delhi-07`,
both online, each showing detected runtimes, CPU/memory, and their current
desired-revision hash (empty for now — you haven't created one yet).

Click **Details** on either row to open its drawer: architecture, compute,
agent version, last-seen time, labels, and workload health once a revision
is applied. Admins can delete a site from here (decommissioning it from
the fleet); operators and admins can edit a site's name, region, and
labels.

Labels matter because they drive **dynamic groups** — reusable rollout
targets defined by a selector instead of a fixed site list. A selector
matches sites whose labels satisfy *every* key=value pair you give it
(there's no "or" — a selector holds one value per key). Click **New
group**, name it "Production", and give it the selector
`tier=production` — both demo sites carry that label (`class` is what
differs between them: `factory` vs. `retail`), so the group resolves to
both. Try `class=factory` instead and you'll see it narrow to just
`factory-pune-01`.

## 4. Declare desired state (a revision)

A **revision** is the typed desired state you want sites to converge on —
one or more workloads, each with a `kind`:

| Kind | What it manages |
|---|---|
| `systemd` | start/stop a named unit |
| `container` | run/replace/stop a Docker or Podman container |
| `k3s` | maintain a manifest in the k3s manifests directory |
| `qemu` | start/stop a basic KVM/QEMU VM (opt-in, disabled by default) |

The README's [desired-state example](../README.md#desired-state-example)
mixes `systemd` and `container` workloads — real for a Linux edge host
with `systemctl` and Docker/Podman installed, but the demo agents here
don't have either (the container image is intentionally minimal). `k3s`
is the one kind that only needs filesystem access, so it's what will
actually reconcile to `applied` in this demo without any extra setup.
`docker-compose.yml` already points `ZYVOR_FLEET_K3S_MANIFEST_DIR` at a
writable path for you; if you're running agents locally instead of via
Compose, export that same variable to a writable directory before
starting `fleet-agent` (the default, `/var/lib/rancher/k3s/server/manifests`,
needs root).

Go to **Rollouts → New revision**. Give it a name like `Demo stack 2026.09`
and paste this into Workloads (JSON):

```json
[
  {
    "kind": "k3s",
    "name": "hello",
    "state": "running",
    "manifest": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: hello\ndata:\n  greeting: hello-from-fleet\n"
  }
]
```

Click **Create revision**. It now appears under Revisions, ready to be
promoted — creating a revision doesn't touch any site by itself; a
rollout is what actually pushes it.

## 5. Start a rollout

Click **Start rollout** (or **New rollout** from Overview). This is where
Fleet's safety model lives:

- **Revision** — pick the one you just created.
- **Wave size** — how many sites get the new state at once. With only two
  demo sites, the default of 10 means both go in a single wave; in a real
  fleet of thousands, a small wave size limits blast radius.
- **Pause between waves** — a cooldown before the next wave starts, so you
  have time to notice a problem.
- **Maximum failures** — the failure budget; a wave stops advancing once
  this many sites in it have failed.
- **Sites / Dynamic group** — target explicit checked sites, or point at
  the "Production" group you made earlier so membership stays live as sites are
  added or relabeled later.
- **Auto rollback** — automatically revert to each site's previous
  revision if the failure budget is exceeded.
- **Require admin approval** — gate activation behind an explicit approval
  step (useful for changes that need a second pair of eyes).

Click **Preview plan** first — it tells you exactly how many sites, how
many waves, and how many are currently offline. A rollout still targets
offline sites; each one applies the new desired state on its own the next
time it successfully syncs (see
[Offline autonomy](#7-what-happens-when-a-site-goes-offline) below).

Click **Create rollout**. Watch it on the Rollouts page: wave count,
completed-vs-total sites, and a progress bar. Pause/Resume/Abort/Retry/
Rollback controls appear depending on the rollout's current status.
Within a few sync cycles both demo sites should show the revision applied
in their Sites drawer.

## 6. Watch it happen in Events

Click **Events**. Enrollment, connectivity, and rollout lifecycle events
all land here and stay visible even across a site reconnecting — nothing
is lost just because a site was briefly offline.

## 7. What happens when a site goes offline

This is Fleet's core design point, not an edge case. Try it:

```bash
docker compose stop edge-a
```

`factory-pune-01` will show as offline in the console once its heartbeat
lapses. It doesn't roll back or drop its workloads — the agent cached the
full revision body locally, and a typed adapter keeps reconciling it
against that cache every cycle with no control-plane round trip required.
On a real Linux edge host running a `container`-kind workload, this is
what keeps a crashed container restarted even with the WAN down: delete
the container by hand and the agent puts it back within one reconciliation
interval, control plane or no control plane.

Bring it back:

```bash
docker compose start edge-a
```

It reconnects, a `controlplane.reconnected` event appears, any events it
queued while offline replay, and it picks up anything new you'd rolled
out in the meantime.

## 8. Manage access (Settings)

Click **Settings**:

- **Zyvor integrations** — enable/configure Nodra, PacketWolf, and the
  other listed integrations (API key + base URL). As of this release these
  store configuration only; wiring an integration to actually call out to
  it is separate, per-integration work — check `docs/PRODUCT_PLAN.md` for
  what's planned here.
- **Operators** — add a user with the minimum role they need
  (`viewer`/`operator`/`admin`), edit their role or reset their password,
  or remove them. You can't demote or delete your own currently-signed-in
  account.
- **Enrollment tokens** — create a short-lived, use-limited token for
  enrolling new sites, or revoke one early.

## 9. The same workflow from `fleetctl`

Everything above has a CLI equivalent, useful for scripting or CI:

```bash
export ZYVOR_FLEET_ADMIN_PASSWORD='zyvor-fleet-demo'

fleetctl status
fleetctl sites
fleetctl events
fleetctl revisions
fleetctl rollouts
fleetctl groups
fleetctl group-create "Production" tier=production
fleetctl rollout-plan GROUP_ID
fleetctl rollout-pause ROLLOUT_ID
fleetctl enroll-token "New factory install"
```

`fleetctl` logs in fresh on each invocation (`--server`/`--email`/
`--password`, or the `ZYVOR_FLEET_*` environment variables shown above) —
there's no separate CLI session to manage.

## What's next

- **Deploying for real**: [docs/DEPLOYMENT.md](DEPLOYMENT.md) — systemd,
  Docker, raw Kubernetes, and Helm, including production hardening notes.
- **Runtime adapter details**: [docs/RUNTIME_ADAPTERS.md](RUNTIME_ADAPTERS.md)
  — exactly what each workload `kind` does and its safety boundary.
- **Full API reference**: [docs/openapi.yaml](openapi.yaml).
- **Roadmap**: [docs/PRODUCT_PLAN.md](PRODUCT_PLAN.md) — what's explicitly
  planned but not yet built (signed artifacts, agent OTA, air-gapped
  bundles, OIDC/SAML, and more).

Clean up the demo whenever you're done:

```bash
docker compose down -v
```
