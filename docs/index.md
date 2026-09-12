# Zyvor Fleet

**Offline-first edge fleet control plane for Linux, Kubernetes, containers and virtual machines.**

Zyvor Fleet manages **remote sites as a fleet**, not merely clusters. A site
can be a factory gateway, retail server, telecom POP, branch appliance,
rugged Linux box, k3s node, GPU edge server or KVM host. The control plane
declares what should run; a small `fleet-agent` pulls that desired state,
caches the complete revision locally, and keeps reconciling it when the WAN
disappears — with no arbitrary remote shell exposed.

For the full picture — architecture diagram, feature list, quick start,
Kubernetes/Helm instructions, security defaults and licensing — see the
[**README on GitHub**](https://github.com/zyvorai/fleet#readme).

## Start here

- [FAQ](FAQ.md) — licensing, support, and production-readiness questions
- [Troubleshooting](TROUBLESHOOTING.md) — real operational issues, with the documented fix
- [Tutorial: getting started](TUTORIAL.md) — a guided walkthrough of the web console and CLI, start to first rollout
- [Deployment guide](DEPLOYMENT.md) — systemd, Docker, Kubernetes/Helm and production hardening
- [Runtime adapters](RUNTIME_ADAPTERS.md) — what each workload `kind` does and its safety boundary
- [v0.3 operations](V0.3_OPERATIONS.md) — maintenance cordons, API-token scopes, webhook signing, audit
- [Product plan](PRODUCT_PLAN.md) — what's shipped and what's next
- [Testing](TESTING.md) — how to run the release gate locally
