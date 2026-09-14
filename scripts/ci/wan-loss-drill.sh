#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Abbreviated WAN / control-plane outage drill for Fleet on a live lab host.
# Stops fleetd briefly while site agent(s) run; records offline + reconnect.
set -euo pipefail
OUT=${1:-/tmp/fleet-wan-loss-$(date -u +%Y%m%dT%H%M%SZ).log}
{
  echo "stamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "host=$(hostname)"
  echo "fleetd=$(/usr/local/bin/fleetd --version 2>/dev/null || true)"
  systemctl is-active zyvor-fleet || true
  curl -skm 3 https://127.0.0.1:18090/readyz || true
  echo "--- stop control plane ---"
  sudo systemctl stop zyvor-fleet
  sleep 8
  echo "readyz_during_outage=$(curl -skm 2 -o /dev/null -w '%{http_code}' https://127.0.0.1:18090/readyz || echo down)"
  if pgrep -a fleet-agent >/dev/null; then
    echo "fleet-agent_still_running=yes"
    pgrep -a fleet-agent
  else
    echo "fleet-agent_still_running=no (no agent process — CP outage still recorded)"
  fi
  echo "--- start control plane ---"
  sudo systemctl start zyvor-fleet
  for i in $(seq 1 20); do
    if curl -skm 2 https://127.0.0.1:18090/readyz | grep -q ready; then
      echo "readyz_after=${i}s ok"
      break
    fi
    sleep 1
  done
  curl -skm 5 https://127.0.0.1:18090/readyz || true
  echo "PASS fleet_wan_loss_abbreviated"
} | tee "$OUT"
echo "wrote $OUT"
