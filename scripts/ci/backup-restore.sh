#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# CI: backup/restore round-trip for Fleet state (software-class; not live PVC).
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
TMP=$(mktemp -d)
trap 'kill ${PID:-0} 2>/dev/null || true; rm -rf "$TMP"' EXIT
make build
export ZYVOR_FLEET_ADMIN_PASSWORD=ci-backup-password
export ZYVOR_FLEET_SESSION_SECRET=ci-backup-session-secret-0123456789abcdef01
PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
DATA="$TMP/data"
mkdir -p "$DATA"
./bin/fleetd --demo --listen "127.0.0.1:${PORT}" --data "$DATA/state.json" >"$TMP/fleetd.log" 2>&1 &
PID=$!
for i in $(seq 1 50); do
  curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null
COOKIE="$TMP/cookie"
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"email":"admin@zyvor.local","password":"ci-backup-password"}' \
  "http://127.0.0.1:${PORT}/api/v1/auth/login" >/dev/null
curl -fsS -b "$COOKIE" "http://127.0.0.1:${PORT}/api/v1/dashboard" >/dev/null
kill "$PID"
wait "$PID" 2>/dev/null || true
PID=0
BEFORE=$(sha256sum "$DATA/state.json" | awk '{print $1}')
./scripts/backup-state.sh "$DATA/state.json" "$TMP/backup.tar.gz"
RESTORE="$TMP/restore"
mkdir -p "$RESTORE"
FLEET_RESTORE_FORCE=1 ./scripts/restore-state.sh "$TMP/backup.tar.gz" "$RESTORE"
AFTER=$(sha256sum "$RESTORE/state.json" | awk '{print $1}')
test "$BEFORE" = "$AFTER"
./bin/fleetd --demo --listen "127.0.0.1:${PORT}" --data "$RESTORE/state.json" >"$TMP/fleetd2.log" 2>&1 &
PID=$!
for i in $(seq 1 50); do
  curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"email":"admin@zyvor.local","password":"ci-backup-password"}' \
  "http://127.0.0.1:${PORT}/api/v1/auth/login" >/dev/null
echo "PASS: fleet backup-restore"
