#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then echo "run as root" >&2; exit 1; fi
BIN="${1:-./bin/fleet-agent}"
test -x "$BIN"
install -m 0755 "$BIN" /usr/local/bin/fleet-agent
install -d -m 0750 /etc/zyvor-fleet /var/lib/zyvor-fleet-agent
if [ ! -f /etc/zyvor-fleet/agent.env ]; then
  cat >/etc/zyvor-fleet/agent.env <<'ENV'
ZYVOR_FLEET_SERVER=https://fleet.example.com
ZYVOR_FLEET_SITE_NAME=edge-site-01
ZYVOR_FLEET_REGION=region-1
ZYVOR_FLEET_LABELS=class=edge,tier=production
# Set only for first enrollment, then remove after the agent has persisted its site identity.
ZYVOR_FLEET_ENROLLMENT_TOKEN=REPLACE_ME
ENV
  chmod 0600 /etc/zyvor-fleet/agent.env
fi
install -m 0644 deploy/systemd/zyvor-fleet-agent.service /etc/systemd/system/zyvor-fleet-agent.service
systemctl daemon-reload
echo "Edit /etc/zyvor-fleet/agent.env, then run: systemctl enable --now zyvor-fleet-agent"
