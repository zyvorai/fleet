#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# Backup a Fleet control-plane state file (--data / ZYVOR_FLEET_DATA).
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/backup-state.sh <state.json> [archive.tar.gz]

Creates a gzip tarball of the directory that holds the state file (so sibling
TLS material or operator notes in the same dir are included). Prefer stopping
the single fleetd writer first (or take a filesystem snapshot) so the JSON is
quiescent. v0.3 is single-writer — never back up two live writers of the same
volume.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || $# -lt 1 ]]; then
  usage
  exit 0
fi

STATE=$1
if [[ ! -f "$STATE" ]]; then
  echo "state file not found: $STATE" >&2
  exit 1
fi
STATE=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$STATE")
DATA_DIR=$(dirname "$STATE")
STATE_BASE=$(basename "$STATE")
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
OUT=${2:-"fleet-backup-${STAMP}.tar.gz"}
OUT=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "$OUT")

tar -C "$DATA_DIR" -czf "$OUT" .
{
  cd "$DATA_DIR"
  find . -type f | LC_ALL=C sort | while read -r f; do
    sha256sum "$f"
  done
} >"${OUT}.SHA256SUMS"

# Convenience: also emit the state file digest alone for ops-checklist.md
sha256sum "$STATE" | tee "${OUT}.${STATE_BASE}.sha256" >/dev/null

echo "wrote $OUT"
echo "wrote ${OUT}.SHA256SUMS"
echo "state digest: $(cat "${OUT}.${STATE_BASE}.sha256")"
ls -lh "$OUT"
