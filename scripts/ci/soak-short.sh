#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
#
# Abbreviated (~10m) WAN/disk soak wrapper for CI or local drills.
# Does not prove production HA — see docs/HA.md and docs/QUALIFICATION.md.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
OUTDIR=${1:-"$ROOT/evidence/qualification/ci/soak-short"}

# CI sets FLEET_SOAK_SECONDS (ci.yml). Honor it so the 10-minute job
# timeout is not eaten by the local 10m default.
if [[ -z "${FLEET_SOAK_DURATION:-}" && -n "${FLEET_SOAK_SECONDS:-}" ]]; then
  FLEET_SOAK_DURATION="${FLEET_SOAK_SECONDS}s"
fi
export FLEET_SOAK_DURATION="${FLEET_SOAK_DURATION:-10m}"
export FLEET_SOAK_WAN_CYCLE="${FLEET_SOAK_WAN_CYCLE:-90s}"
export FLEET_SOAK_WAN_DOWNTIME="${FLEET_SOAK_WAN_DOWNTIME:-10s}"
export FLEET_SOAK_DISK_CYCLE="${FLEET_SOAK_DISK_CYCLE:-60s}"
export FLEET_SOAK_DISK_TARGET_MB="${FLEET_SOAK_DISK_TARGET_MB:-128}"
export FLEET_SOAK_SAMPLE_INTERVAL="${FLEET_SOAK_SAMPLE_INTERVAL:-5s}"
export FLEET_SOAK_READY_CEILING="${FLEET_SOAK_READY_CEILING:-30}"

exec "$ROOT/scripts/ci/soak.sh" "$OUTDIR"
