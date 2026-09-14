#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# CI: compose image smoke for Zyvor Fleet (control-plane; distroless-writable /tmp).
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
cleanup() {
  docker rm -f fleet-ci-cp >/dev/null 2>&1 || true
  docker compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker compose build control-plane
# Distroless nonroot cannot write the default named volume; use /tmp for CI state.
docker compose run -d --name fleet-ci-cp --service-ports \
  -e ZYVOR_FLEET_ADMIN_PASSWORD=zyvor-fleet-demo \
  -e ZYVOR_FLEET_SESSION_SECRET=local-demo-session-secret-change-me-1234567890 \
  control-plane --demo --listen :8080 --data /tmp/state.json
for i in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
curl -fsS http://127.0.0.1:8080/healthz >/dev/null
ZYVOR_FLEET_SMOKE_URL=http://127.0.0.1:8080 ./scripts/smoke.sh
echo "PASS: fleet compose-smoke"
