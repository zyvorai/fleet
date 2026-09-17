#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
#
# Duration-configurable WAN-loss + disk-pressure soak driver for Fleet.
#
# Runs fleetd + fleet-agent as host binaries (same pattern as
# scripts/ci/backup-restore.sh / scripts/live-smoke.py). Distroless compose
# images have no shell/dd, so disk pressure is applied on the host data dir.
#
# Env vars (all optional):
#   FLEET_SOAK_DURATION        total run time, e.g. 10m, 4h (default 10m)
#   FLEET_SOAK_WAN_CYCLE       time between WAN-loss cycles (default 90s)
#   FLEET_SOAK_WAN_DOWNTIME    how long control-plane stays down per cycle (default 10s)
#   FLEET_SOAK_DISK_CYCLE      time between disk-pressure cycles (default 60s)
#   FLEET_SOAK_DISK_TARGET_MB  peak fill size per disk-pressure ramp (default 256)
#   FLEET_SOAK_SAMPLE_INTERVAL background sampler cadence (default 5s)
#   FLEET_SOAK_READY_CEILING   max acceptable time-to-ready in seconds (default 30)
#   FLEET_SOAK_AGENT_INTERVAL  fleet-agent --interval (default 1s)
#
# Output: $OUTDIR/{samples.jsonl,wan-cycles.jsonl,disk-cycles.jsonl,summary.json}
# Pass/fail is judged separately by scripts/ci/soak-check.py against summary.json
# — this script's own exit code only reflects "did the drill run to completion."
#
# This does NOT prove production HA. v0.3 remains single-writer; see docs/HA.md.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT"

to_seconds() {
  local input="$1" total=0
  if [[ "$input" =~ ^[0-9]+$ ]]; then
    echo "$input"
    return
  fi
  while [[ "$input" =~ ([0-9]+)([smhd]) ]]; do
    local num="${BASH_REMATCH[1]}" unit="${BASH_REMATCH[2]}"
    case "$unit" in
      s) total=$((total + num)) ;;
      m) total=$((total + num * 60)) ;;
      h) total=$((total + num * 3600)) ;;
      d) total=$((total + num * 86400)) ;;
    esac
    input="${input/${BASH_REMATCH[0]}/}"
  done
  echo "$total"
}

OUTDIR=${1:-/tmp/fleet-soak-$(date -u +%Y%m%dT%H%M%SZ)}
mkdir -p "$OUTDIR"

DURATION=$(to_seconds "${FLEET_SOAK_DURATION:-10m}")
WAN_CYCLE=$(to_seconds "${FLEET_SOAK_WAN_CYCLE:-90s}")
WAN_DOWNTIME=$(to_seconds "${FLEET_SOAK_WAN_DOWNTIME:-10s}")
DISK_CYCLE=$(to_seconds "${FLEET_SOAK_DISK_CYCLE:-60s}")
DISK_TARGET_MB=${FLEET_SOAK_DISK_TARGET_MB:-256}
SAMPLE_INTERVAL=$(to_seconds "${FLEET_SOAK_SAMPLE_INTERVAL:-5s}")
READY_CEILING=${FLEET_SOAK_READY_CEILING:-30}
AGENT_INTERVAL=${FLEET_SOAK_AGENT_INTERVAL:-1s}

WORKDIR="$OUTDIR/runtime"
DATA_DIR="$WORKDIR/data"
AGENT_DIR="$WORKDIR/agent"
mkdir -p "$DATA_DIR" "$AGENT_DIR" "$WORKDIR/manifests"

SAMPLES="$OUTDIR/samples.jsonl"
WAN_LOG="$OUTDIR/wan-cycles.jsonl"
DISK_LOG="$OUTDIR/disk-cycles.jsonl"
SUMMARY="$OUTDIR/summary.json"
: >"$SAMPLES"
: >"$WAN_LOG"
: >"$DISK_LOG"

log() { echo "[soak] $*" >&2; }

export ZYVOR_FLEET_ADMIN_PASSWORD=${ZYVOR_FLEET_ADMIN_PASSWORD:-fleet-soak-password}
export ZYVOR_FLEET_SESSION_SECRET=${ZYVOR_FLEET_SESSION_SECRET:-fleet-soak-session-secret-0123456789abcdef01}
export ZYVOR_FLEET_ENROLLMENT_TOKEN=${ZYVOR_FLEET_ENROLLMENT_TOKEN:-zf_enroll_demo-local-only}
export ZYVOR_FLEET_K3S_MANIFEST_DIR="$WORKDIR/manifests"

PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')
CP_BASE="http://127.0.0.1:${PORT}"
CURL=(curl -sSm 5)

FLEETD_PID=""
AGENT_PID=""
FLEETD_PIDFILE="$WORKDIR/fleetd.pid"
AGENT_PIDFILE="$WORKDIR/agent.pid"
BG_PIDS=()

read_pid() {
  local f="$1"
  [ -f "$f" ] && tr -d ' \n' <"$f" || true
}

cp_ready() {
  "${CURL[@]}" -o /dev/null -w '%{http_code}' "$CP_BASE/readyz" 2>/dev/null || echo 000
}

cp_metric() {
  # Extract integer from a gauge line; for labeled metrics pass a substring match.
  local pattern="$1"
  "${CURL[@]}" "$CP_BASE/metrics" 2>/dev/null \
    | grep -E "$pattern" | tail -1 | awk '{print $NF}' | grep -E '^[0-9]+$' || echo 0
}

sites_json() {
  # Demo mode exposes unauthenticated read of some paths inconsistently;
  # scrape Prometheus gauges instead for online/offline counts.
  python3 - <<PY
import json, urllib.request
try:
    raw = urllib.request.urlopen("$CP_BASE/metrics", timeout=3).read().decode()
except Exception:
    print(json.dumps({"online":0,"offline":0,"degraded":0,"events":0}))
    raise SystemExit
online=offline=degraded=events=0
for line in raw.splitlines():
    if line.startswith('zyvor_fleet_sites{status="online"}'):
        online=int(float(line.split()[-1]))
    elif line.startswith('zyvor_fleet_sites{status="offline"}'):
        offline=int(float(line.split()[-1]))
    elif line.startswith('zyvor_fleet_sites{status="degraded"}'):
        degraded=int(float(line.split()[-1]))
    elif line.startswith('zyvor_fleet_events_total'):
        events=int(float(line.split()[-1]))
print(json.dumps({"online":online,"offline":offline,"degraded":degraded,"events":events}))
PY
}

proc_alive() {
  local pid="${1:-}"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

proc_rss_bytes() {
  local pid="${1:-}"
  if [ -z "$pid" ] || ! proc_alive "$pid"; then
    echo 0
    return
  fi
  # macOS ps: rss is KB; Linux ps: rss is KB as well with -o rss=
  local kb
  kb=$(ps -o rss= -p "$pid" 2>/dev/null | tr -d ' ' || echo 0)
  echo $(( ${kb:-0} * 1024 ))
}

disk_usage_pct() {
  df -P "$1" 2>/dev/null | awk 'NR==2 {gsub("%","",$5); print $5}' || echo 0
}

start_fleetd() {
  : >"$WORKDIR/fleetd.log"
  ./bin/fleetd --demo --listen "127.0.0.1:${PORT}" --data "$DATA_DIR/state.json" \
    >>"$WORKDIR/fleetd.log" 2>&1 &
  FLEETD_PID=$!
  echo "$FLEETD_PID" >"$FLEETD_PIDFILE"
}

stop_fleetd() {
  local pid
  pid=$(read_pid "$FLEETD_PIDFILE")
  if proc_alive "$pid"; then
    kill "$pid" 2>/dev/null || true
    # Do not wait — caller may be a subshell that did not start this pid.
    for _ in $(seq 1 20); do
      proc_alive "$pid" || break
      sleep 0.2
    done
    kill -9 "$pid" 2>/dev/null || true
  fi
  FLEETD_PID=""
  rm -f "$FLEETD_PIDFILE"
}

start_agent() {
  : >"$WORKDIR/agent.log"
  ./bin/fleet-agent \
    --server "$CP_BASE" \
    --name "soak-site-01" \
    --region "ci" \
    --state "$AGENT_DIR/agent.json" \
    --labels "class=soak,env=ci" \
    --interval "$AGENT_INTERVAL" \
    >>"$WORKDIR/agent.log" 2>&1 &
  AGENT_PID=$!
  echo "$AGENT_PID" >"$AGENT_PIDFILE"
}

wait_ready() {
  local ceiling="$1" i
  for ((i = 1; i <= ceiling; i++)); do
    if [ "$(cp_ready)" = "200" ]; then
      echo "$i"
      return 0
    fi
    sleep 1
  done
  echo -1
}

fill_disk() {
  local mb="$1"
  dd if=/dev/zero of="$DATA_DIR/.soak-fill.bin" bs=1M count="$mb" status=none 2>/dev/null || true
}

release_disk() {
  rm -f "$DATA_DIR/.soak-fill.bin"
}

write_summary() {
  local interrupted="$1"
  local sites_end events_end wan_n disk_n
  # Background loops run in subshells — count completed cycles from the logs.
  wan_n=$(wc -l <"$WAN_LOG" | tr -d ' ')
  disk_n=$(wc -l <"$DISK_LOG" | tr -d ' ')
  sites_end=$(sites_json)
  events_end=$(python3 -c "import json,sys; print(json.load(sys.stdin)['events'])" <<<"$sites_end")
  local fleetd_alive agent_alive
  fleetd_alive=$(proc_alive "$(read_pid "$FLEETD_PIDFILE")" && echo true || echo false)
  agent_alive=$(proc_alive "$(read_pid "$AGENT_PIDFILE")" && echo true || echo false)
  python3 - "$SUMMARY" <<PYEOF
import json, sys
sites_end = json.loads('''$sites_end''')
out = {
    "generated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "duration_s": $DURATION,
    "interrupted": bool(int("$interrupted")),
    "mode": "host-binaries",
    "wan_cycles_run": int("$wan_n"),
    "disk_cycles_run": int("$disk_n"),
    "ready_ceiling_s": $READY_CEILING,
    "samples_path": "samples.jsonl",
    "wan_cycles_path": "wan-cycles.jsonl",
    "disk_cycles_path": "disk-cycles.jsonl",
    "ha_claim": False,
    "note": "Single-writer soak only; does not prove production HA (see docs/HA.md).",
    "metrics": {
        "events_start": $EVENTS_START,
        "events_end": $events_end,
        "sites_online_end": sites_end.get("online", 0),
        "sites_offline_end": sites_end.get("offline", 0),
    },
    "pids_alive_end": {
        "fleetd": json.loads("$fleetd_alive"),
        "fleet_agent": json.loads("$agent_alive"),
    },
}
with open(sys.argv[1], "w") as f:
    json.dump(out, f, indent=2)
    f.write("\n")
PYEOF
  log "wrote $SUMMARY"
}

cleanup() {
  local ec=$?
  trap - EXIT INT TERM
  log "cleanup (exit=$ec)"
  for pid in "${BG_PIDS[@]:-}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
  # Wait only for sampler/wan/disk loops — bare `wait` would block on fleetd/agent.
  for pid in "${BG_PIDS[@]:-}"; do
    wait "$pid" 2>/dev/null || true
  done
  release_disk || true
  write_summary "$( [ "$ec" != "0" ] && echo 1 || echo 0 )" || true
  stop_fleetd || true
  local agent_pid
  agent_pid=$(read_pid "$AGENT_PIDFILE")
  if proc_alive "$agent_pid"; then
    kill "$agent_pid" 2>/dev/null || true
    wait "$agent_pid" 2>/dev/null || true
  fi
  exit "$ec"
}
trap cleanup EXIT INT TERM

log "building binaries"
make build

log "starting fleetd on $CP_BASE"
start_fleetd
for i in $(seq 1 60); do
  [ "$(cp_ready)" = "200" ] && break
  sleep 0.5
done
[ "$(cp_ready)" = "200" ] || { log "fleetd failed to become ready"; cat "$WORKDIR/fleetd.log" >&2 || true; exit 1; }

log "starting fleet-agent"
start_agent
# Wait until at least one site is online.
for i in $(seq 1 60); do
  online=$(python3 -c "import json,sys; print(json.load(sys.stdin)['online'])" <<<"$(sites_json)")
  [ "$online" -ge 1 ] && break
  sleep 0.5
done

EVENTS_START=$(cp_metric '^zyvor_fleet_events_total')
EVENTS_START=${EVENTS_START:-0}

sampler_loop() {
  local end=$((SECONDS + DURATION))
  while [ "$SECONDS" -lt "$end" ]; do
    local ts ready sites disk_pct cp_rss agent_rss agent_alive
    ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    ready=$(cp_ready)
    sites=$(sites_json)
    disk_pct=$(disk_usage_pct "$DATA_DIR")
    cp_rss=$(proc_rss_bytes "$(read_pid "$FLEETD_PIDFILE")")
    agent_rss=$(proc_rss_bytes "$(read_pid "$AGENT_PIDFILE")")
    agent_alive=$(proc_alive "$(read_pid "$AGENT_PIDFILE")" && echo true || echo false)
    printf '{"ts":"%s","cp_ready_code":"%s","disk_pct":%s,"sites":%s,"agent_alive":%s,"cp_stats":{"mem_bytes":%s},"agent_stats":{"mem_bytes":%s}}\n' \
      "$ts" "$ready" "${disk_pct:-0}" "$sites" "$agent_alive" "$cp_rss" "$agent_rss" >>"$SAMPLES"
    sleep "$SAMPLE_INTERVAL"
  done
}

wan_loop() {
  local end=$((SECONDS + DURATION))
  while [ "$SECONDS" -lt "$end" ]; do
    sleep "$WAN_CYCLE"
    [ "$SECONDS" -ge "$end" ] && break
    local pre_sites post_sites outage_start outage_end time_to_ready agent_during
    pre_sites=$(sites_json)
    outage_start=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    stop_fleetd
    sleep "$WAN_DOWNTIME"
    agent_during=$(proc_alive "$(read_pid "$AGENT_PIDFILE")" && echo true || echo false)
    start_fleetd
    time_to_ready=$(wait_ready "$READY_CEILING")
    outage_end=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    # Give the agent a couple of sync intervals to re-register online.
    sleep 3
    post_sites=$(sites_json)
    local cycle_n
    cycle_n=$(wc -l <"$WAN_LOG" | tr -d ' ')
    cycle_n=$((cycle_n + 1))
    printf '{"cycle":%d,"outage_start":"%s","outage_end":"%s","time_to_ready_s":%s,"agent_alive_during_outage":%s,"pre_outage_sites":%s,"post_restore_sites":%s}\n' \
      "$cycle_n" "$outage_start" "$outage_end" "$time_to_ready" "$agent_during" "$pre_sites" "$post_sites" >>"$WAN_LOG"
    log "wan cycle $cycle_n done (time_to_ready=${time_to_ready}s agent_alive=$agent_during)"
  done
}

disk_loop() {
  local end=$((SECONDS + DURATION))
  local steps=(25 50 75 100)
  while [ "$SECONDS" -lt "$end" ]; do
    sleep "$DISK_CYCLE"
    [ "$SECONDS" -ge "$end" ] && break
    local before after released
    before=$(disk_usage_pct "$DATA_DIR")
    for pct in "${steps[@]}"; do
      local mb=$((DISK_TARGET_MB * pct / 100))
      fill_disk "$mb"
      sleep 2
    done
    after=$(disk_usage_pct "$DATA_DIR")
    release_disk
    sleep 2
    released=$(disk_usage_pct "$DATA_DIR")
    local cycle_n
    cycle_n=$(wc -l <"$DISK_LOG" | tr -d ' ')
    cycle_n=$((cycle_n + 1))
    printf '{"cycle":%d,"disk_pct_before":%s,"disk_pct_peak":%s,"disk_pct_after_release":%s,"target_mb":%d}\n' \
      "$cycle_n" "${before:-0}" "${after:-0}" "${released:-0}" "$DISK_TARGET_MB" >>"$DISK_LOG"
    log "disk cycle $cycle_n done (before=${before}% peak=${after}% after_release=${released}%)"
  done
}

SECONDS=0
log "soak starting: duration=${DURATION}s wan_cycle=${WAN_CYCLE}s wan_downtime=${WAN_DOWNTIME}s disk_cycle=${DISK_CYCLE}s"

sampler_loop &
BG_PIDS+=("$!")
wan_loop &
BG_PIDS+=("$!")
disk_loop &
BG_PIDS+=("$!")

while [ "$SECONDS" -lt "$DURATION" ]; do
  sleep 1
done

log "soak duration elapsed, tearing down"
