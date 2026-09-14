#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# Restore a Fleet control-plane data directory from scripts/backup-state.sh.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/restore-state.sh <archive.tar.gz> <data-dir>

Stops nothing by itself — stop fleetd (single writer) before restoring.
Never point two live fleetd processes at the same restored volume.
After restore, start one replica and verify login + GET /readyz (or /healthz).

Set FLEET_RESTORE_FORCE=1 to overwrite an existing non-empty data-dir.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || $# -ne 2 ]]; then
  usage
  exit 0
fi

ARCHIVE=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$1")
DATA_DIR=$2
mkdir -p "$DATA_DIR"
DATA_DIR=$(cd "$DATA_DIR" && pwd)

if [[ ! -f "$ARCHIVE" ]]; then
  echo "archive not found: $ARCHIVE" >&2
  exit 1
fi

if [[ -z "${FLEET_RESTORE_FORCE:-}" ]]; then
  if find "$DATA_DIR" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
    echo "refusing to overwrite existing data in $DATA_DIR" >&2
    echo "set FLEET_RESTORE_FORCE=1 after stopping the writer if this is intentional" >&2
    exit 1
  fi
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tar -C "$tmp" -xzf "$ARCHIVE"
if [[ -n "${FLEET_RESTORE_FORCE:-}" ]]; then
  find "$DATA_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
fi
cp -a "$tmp"/. "$DATA_DIR"/
echo "restored $ARCHIVE → $DATA_DIR"
echo "start one fleetd --data $DATA_DIR/state.json (or your path) and verify /readyz"
