# Deployment guide

## Control plane production checklist

1. Use a stable persistent volume for the state file.
2. Set `ZYVOR_FLEET_ADMIN_PASSWORD` before first bootstrap.
3. Set a stable random `ZYVOR_FLEET_SESSION_SECRET` with at least 32 bytes.
4. Put the service behind HTTPS (or provide `--tls-cert` and `--tls-key` directly).
5. Back up the state file and test restore.
6. Keep one control-plane replica for the embedded-store mode.
7. Restrict network access to the UI/API as appropriate, while permitting outbound edge agents to reach it.
8. Use short-lived, low-use enrollment tokens.

## Bare binary

```bash
make build
sudo install -m 0755 bin/fleetd /usr/local/bin/fleetd
sudo install -d -m 0750 -o zyvor-fleet -g zyvor-fleet /var/lib/zyvor-fleet
```

Use the provided systemd unit as a starting point and place secrets in `/etc/zyvor-fleet/fleet.env` with mode `0600`.

## TLS

Direct TLS:

```bash
fleetd \
  --listen :8443 \
  --tls-cert /etc/zyvor-fleet/tls.crt \
  --tls-key /etc/zyvor-fleet/tls.key
```

If TLS terminates at a reverse proxy, keep the private control-plane hop appropriately protected and set `ZYVOR_FLEET_SECURE_COOKIES=1`. Cookies are automatically marked `Secure` when direct TLS is configured. With Helm, set `secureCookies=true` when exposing Fleet through HTTPS ingress.

## Docker Compose

The included compose stack is an evaluation environment:

```bash
docker compose up --build
```

It starts one control plane and two demo agents.

## Kubernetes

### Helm

Recommended:

```bash
helm upgrade --install zyvor-fleet ./deploy/helm/zyvor-fleet \
  -n zyvor-fleet --create-namespace \
  --set admin.password="$ADMIN_PASSWORD" \
  --set sessionSecret="$SESSION_SECRET"
```

Use an existing Secret in production:

```bash
kubectl -n zyvor-fleet create secret generic fleet-prod \
  --from-literal=admin-password="$ADMIN_PASSWORD" \
  --from-literal=session-secret="$SESSION_SECRET"

helm upgrade --install zyvor-fleet ./deploy/helm/zyvor-fleet \
  -n zyvor-fleet --create-namespace \
  --set existingSecret=fleet-prod
```

### Why one replica

The v0.2 embedded store is intentionally single-writer and file-backed. Running multiple replicas against a `ReadWriteMany` filesystem would not produce a correct distributed database. The chart therefore fixes the server at one replica and uses `Recreate` strategy.

## Agent as a Linux service

The usual edge deployment is a direct Linux service, not a pod. The provided systemd unit reads `/etc/zyvor-fleet/agent.env`. After first enrollment the durable site credential lives in the configured agent state file; remove the enrollment token from the environment when practical.
