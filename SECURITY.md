# Security policy

## Supported versions

Security fixes are provided for the latest tagged minor release.

## Reporting

Please report suspected vulnerabilities privately to the Zyvor maintainers rather than opening a public issue. Include the affected version, deployment mode, reproduction steps, impact, and any suggested mitigation.

## Security model

Zyvor Fleet deliberately separates two trust paths:

- **Human API/UI:** PBKDF2-HMAC-SHA256 passwords, signed HttpOnly SameSite session cookies, role checks, and a same-origin mutation marker.
- **Site agents:** short-lived/restricted enrollment tokens are exchanged for independent high-entropy per-site bearer identities. The enrollment plaintext and agent plaintext are never returned by list APIs.

The server sets CSP, frame denial, nosniff, referrer and browser permission headers. Login attempts receive basic in-memory throttling. Persistent state is written with mode `0600` and atomic rename.

### Production requirements

1. Terminate TLS at `fleetd` or a trusted ingress/reverse proxy.
2. Set a strong `ZYVOR_FLEET_ADMIN_PASSWORD` on first start.
3. Set `ZYVOR_FLEET_SESSION_SECRET` to at least 32 random bytes and keep it stable and secret.
4. Do not enable `--demo` in production.
5. Restrict the control-plane data volume and back it up.
6. Rotate enrollment tokens frequently and keep `maxUses` small.
7. Run the edge agent with only the OS privileges required by the runtime adapters you actually use.
8. QEMU lifecycle is disabled by default and requires `ZYVOR_FLEET_ALLOW_QEMU=1`.

## Remote execution boundary

Fleet does **not** expose arbitrary remote shell execution. Desired state is reconciled through typed adapters (`systemd`, `container`, `k3s`, `qemu`) with name/path validation. This is intentional.
