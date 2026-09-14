#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# CI: HTTPS fleetd TLS smoke (OTA-compatible fleet_url posture).
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"
TMP=$(mktemp -d)
trap 'kill ${PID:-0} 2>/dev/null || true; rm -rf "$TMP"' EXIT
make build
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout "$TMP/key.pem" -out "$TMP/cert.pem" -days 1 -nodes \
  -subj "/CN=zyvor-fleet-ci" \
  -addext "subjectAltName=IP:127.0.0.1,DNS:localhost" >/dev/null 2>&1
PORT_SOCK=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
export ZYVOR_FLEET_ADMIN_PASSWORD=ci-fleet-password
export ZYVOR_FLEET_SESSION_SECRET=ci-fleet-session-secret-0123456789abcdef01234567
./bin/fleetd --demo --listen "127.0.0.1:${PORT_SOCK}" --data "$TMP/state.json" \
  --tls-cert "$TMP/cert.pem" --tls-key "$TMP/key.pem" >"$TMP/fleetd.log" 2>&1 &
PID=$!
BASE="https://127.0.0.1:${PORT_SOCK}"
for i in $(seq 1 50); do
  curl -fsSk "$BASE/healthz" >/dev/null 2>&1 && break
  sleep 0.2
done
curl -fsSk "$BASE/healthz" >/dev/null
COOKIE="$TMP/cookie"
curl -fsSk -c "$COOKIE" -H 'Content-Type: application/json' \
  -d '{"email":"admin@zyvor.local","password":"ci-fleet-password"}' \
  "$BASE/api/v1/auth/login" >/dev/null
curl -fsSk -b "$COOKIE" "$BASE/api/v1/dashboard" >/dev/null
# Device bearer registration under HTTPS (OTA fleet_url posture).
resp=$(curl -sk -b "$COOKIE" -H 'Content-Type: application/json' -H 'X-Zyvor-Request: 1' \
  -d '{"deviceId":"ci-tls-device-001"}' \
  "$BASE/api/v1/ota/devices" || true)
echo "$resp" | head -c 200
# Unauthenticated assignment probe must not be a TLS/protocol failure.
curl -sk -o /dev/null -w "%{http_code}" "$BASE/v1/devices/ci-tls-device-001/assignment" | grep -Eq '401|204|404|200'
echo "PASS: fleet tls-smoke on :${PORT_SOCK}"
