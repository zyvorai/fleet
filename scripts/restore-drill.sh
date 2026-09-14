#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# Local checksum round-trip for Fleet backup/restore (no live fleetd required).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

SRC=${1:-}
if [[ -z "$SRC" ]]; then
  mkdir -p "$WORK/src"
  # Minimal valid-shaped state for a dry drill (schema only).
  printf '%s\n' '{"schemaVersion":1,"sites":[],"users":[],"events":[]}' >"$WORK/src/state.json"
  SRC="$WORK/src/state.json"
elif [[ ! -f "$SRC" ]]; then
  echo "state file not found: $SRC" >&2
  exit 1
else
  SRC=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$SRC")
fi

BEFORE=$(sha256sum "$SRC" | awk '{print $1}')
ARCHIVE="$WORK/fleet-drill.tar.gz"
"$ROOT/scripts/backup-state.sh" "$SRC" "$ARCHIVE" >/dev/null
DEST="$WORK/restored"
mkdir -p "$DEST"
FLEET_RESTORE_FORCE=1 "$ROOT/scripts/restore-state.sh" "$ARCHIVE" "$DEST" >/dev/null
AFTER=$(sha256sum "$DEST/$(basename "$SRC")" | awk '{print $1}')

if [[ "$BEFORE" != "$AFTER" ]]; then
  echo "FAIL: digest mismatch before=$BEFORE after=$AFTER" >&2
  exit 1
fi

echo "PASS restore-drill digest=$AFTER"
echo "Record this hash in evidence/qualification/ops-checklist.md when drilling a real volume."
