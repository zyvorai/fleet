---
hero:
  eyebrow: PRODUCT
  title: Zyvor Fleet
  lead: Offline-first edge fleet control plane for Linux, Kubernetes, containers and virtual machines.
  highlights:
    - {value: "4", label: "Typed runtime adapters — systemd, container, k3s, QEMU", footnote: "1"}
    - {value: "0", label: "Cloud calls required to keep reconciling through a WAN outage", footnote: "2"}
    - {value: "v0.3", label: "Current shipped release — cordons, scoped tokens, signed webhooks, audit", footnote: "3"}
    - {value: "5,000", label: "Bounded mutation audit trail records retained", footnote: "4"}
  hub_bands:
    - {icon: "🚀", title: "Tutorial", description: "A hands-on walkthrough: stand up a demo fleet, tour the web console, declare desired state, and promote your first rollout.", href: TUTORIAL.md}
    - {icon: "📦", title: "Deployment guide", description: "Systemd, Docker Compose, and Kubernetes/Helm install paths, plus the production checklist to run before go-live.", href: DEPLOYMENT.md}
    - {icon: "⚙️", title: "Runtime adapters", description: "What each workload kind — systemd, container, k3s, QEMU — does, and its safety boundary.", href: RUNTIME_ADAPTERS.md}
    - {icon: "🛠️", title: "Troubleshooting", description: "Real operational issues people hit, with the documented fix — not a generic checklist.", href: TROUBLESHOOTING.md}
    - {icon: "💬", title: "FAQ", description: "Licensing, support, and production-readiness questions people ask before adopting Fleet.", href: FAQ.md}
    - {icon: "🗺️", title: "Product plan", description: "What v0.3 shipped, and what's next on the v0.4 and v1.0 roadmap.", href: PRODUCT_PLAN.md}
footnotes:
  - {marker: "1", text: "systemd, container (Docker/Podman), k3s, and QEMU/KVM — deliberately no generic shell workload type.", href: RUNTIME_ADAPTERS.md, href_label: "See Runtime adapters."}
  - {marker: "2", text: "\"No cloud/control-plane call is necessary to keep the last accepted desired state alive\" once a site has cached its revision.", href: "https://github.com/zyvorai/fleet#offline-autonomy", href_label: "See Offline autonomy in the README."}
  - {marker: "3", text: "v0.3 added site maintenance/cordon mode, scoped API tokens, HMAC-signed webhooks, and a bounded mutation audit trail.", href: PRODUCT_PLAN.md, href_label: "See Product plan."}
  - {marker: "4", text: "Successful and failed operator/API mutations are recorded without request bodies or credentials, bounded to the most recent 5,000 records.", href: V0.3_OPERATIONS.md, href_label: "See v0.3 operations."}
---

Zyvor Fleet manages **remote sites as a fleet**, not merely clusters. A site
can be a factory gateway, retail server, telecom POP, branch appliance,
rugged Linux box, k3s node, GPU edge server or KVM host. The control plane
declares what should run; a small `fleet-agent` pulls that desired state,
caches the complete revision locally, and keeps reconciling it when the WAN
disappears — with no arbitrary remote shell exposed.

For the full picture — architecture diagram, feature list, quick start,
Kubernetes/Helm instructions, security defaults and licensing — see the
[**README on GitHub**](https://github.com/zyvorai/fleet#readme).

## Three ways to run the control plane

<div class="compare-cards" markdown="1">
- **Bare binary**

    `make build`, install `fleetd` as a systemd service, and point it at a
    persistent volume. The usual choice for a standalone edge control plane.

- **Docker Compose**

    `docker compose up --build` starts one control plane plus two simulated
    edge sites — the fastest way to see the console and protocol end to end.

- **Kubernetes / Helm**

    `helm upgrade --install zyvor-fleet ./deploy/helm/zyvor-fleet` deploys one
    control-plane replica with a `ReadWriteOnce` volume — intentionally one
    replica, since the v0.3 embedded store is single-writer.
</div>

See the [deployment guide](DEPLOYMENT.md) for the full production checklist.
