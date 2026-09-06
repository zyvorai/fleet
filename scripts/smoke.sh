#!/bin/sh
set -eu
BASE="${ZYVOR_FLEET_SMOKE_URL:-http://127.0.0.1:18080}"
EMAIL="${ZYVOR_FLEET_ADMIN_EMAIL:-admin@zyvor.local}"
PASSWORD="${ZYVOR_FLEET_ADMIN_PASSWORD:-zyvor-fleet-demo}"
COOKIE="${TMPDIR:-/tmp}/zyvor-fleet-smoke-cookie.$$"
trap 'rm -f "$COOKIE"' EXIT

curl -fsS "$BASE/healthz" >/dev/null
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" \
  "$BASE/api/v1/auth/login" >/dev/null
curl -fsS -b "$COOKIE" "$BASE/api/v1/dashboard" >/dev/null
curl -fsS -b "$COOKIE" "$BASE/api/v1/sites" >/dev/null
printf '%s\n' "Zyvor Fleet HTTP smoke test passed: $BASE"
