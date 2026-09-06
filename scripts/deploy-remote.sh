#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# ─────────────────────────────────────────────────────────────
# Zyvor Fleet — Remote deployment (SSH + rsync)
#
# Profiles:
#   default     Sync → ensure Go → build on remote → install → start demo service
#   --quick     Rsync + remote build only (skip system Go install if present)
#   --quick --build-local   Rsync pre-built Linux binaries (build locally first)
#
# Auth: SSH keys (recommended). Password via sshpass is supported but deprecated.
# ─────────────────────────────────────────────────────────────
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="0.2.0"
REMOTE_DIR=""
DEPLOY_PROFILE="full"
DEPLOY_LOG="${FLEET_DEPLOY_LOG:-${HOME}/.zyvor-fleet/deploy-$(date +%Y%m%d-%H%M%S).log}"
# Listen port: --port / FLEET_PORT win; otherwise reuse the remote unit's
# existing listen address; otherwise default 8080.
PORT_EXPLICIT=false
if [ -n "${FLEET_PORT:-}" ]; then
    PORT_EXPLICIT=true
fi
FLEET_PORT="${FLEET_PORT:-8080}"

QUICK_MODE=false
UNINSTALL=false
KEY_AUTH=false
DRY_RUN=false
SKIP_SYNC=false
SKIP_VERIFY=false
BUILD_LOCAL=false
VERIFY_ONLY=false
PREFLIGHT_ONLY=false
NO_SERVICE=false
VERBOSE=false
SSH_RETRIES="${FLEET_SSH_RETRIES:-3}"
POSITIONAL=()

usage() {
    cat <<EOF
Zyvor Fleet remote deploy v${VERSION}

Usage:
  \$0 <host> <user> [options]
  \$0 user@host [options]

Profiles:
  (default)                 Full remote build + Go toolchain if needed + systemd demo unit
  --quick                   Rsync + go build on remote (skip toolchain install when Go exists)
  --quick --build-local     Install locally built Linux binaries (Linux build host required)

Options:
  --help              Show this help
  --dry-run           Print steps without SSH/rsync/build
  --preflight-only    SSH + disk/sudo checks, then exit
  --verify-only       Hit /readyz on the remote host only
  --skip-sync         Skip rsync (sources already on host)
  --skip-verify       Skip health check
  --build-local       With --quick: use local bin/fleetd + fleet-agent + fleetctl
  --no-service        Install binaries only (do not enable systemd unit)
  --key               SSH key auth (clear password)
  --uninstall         Stop service and remove install
  --port <N>          Listen port (sets ZYVOR_FLEET_LISTEN=:<N>). Default: existing
                      remote unit port if present, else 8080 / \$FLEET_PORT
  -v, --verbose       Verbose rsync

Environment:
  FLEET_DEPLOY_LOG     Log file path
  FLEET_SSH_RETRIES    SSH retry count (default: 3)
  FLEET_PORT           Listen port (same as --port; preserved across redeploys
                       when neither --port nor FLEET_PORT is set)
  FLEET_GO_VERSION     Go toolchain to install if missing (default: 1.27.1)
  DEPLOY_DIR           Override remote staging dir (default: ~/.deployments/zyvor-fleet)

Examples:
  \$0 <user>@<host> --key
  \$0 <host> <user>
  \$0 <user>@<host> --quick
  \$0 <user>@<host> --build-local --quick
  \$0 <user>@<host> --port 18080 --key
  FLEET_PORT=18080 \$0 <user>@<host> --key
  make deploy-remote H=<host> U=<user> ARGS='--port 18080'
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help)        usage; exit 0 ;;
        --quick)          QUICK_MODE=true; DEPLOY_PROFILE="quick"; shift ;;
        --uninstall)      UNINSTALL=true; shift ;;
        --key)            KEY_AUTH=true; shift ;;
        --dry-run)        DRY_RUN=true; shift ;;
        --skip-sync)      SKIP_SYNC=true; shift ;;
        --skip-verify)    SKIP_VERIFY=true; shift ;;
        --build-local)    BUILD_LOCAL=true; shift ;;
        --verify-only)    VERIFY_ONLY=true; shift ;;
        --preflight-only) PREFLIGHT_ONLY=true; shift ;;
        --no-service)     NO_SERVICE=true; shift ;;
        --port)
            [ -n "${2:-}" ] || { echo "--port requires a value" >&2; exit 1; }
            FLEET_PORT="$2"
            PORT_EXPLICIT=true
            shift 2 ;;
        -v|--verbose)     VERBOSE=true; shift ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

case "${FLEET_PORT}" in
    ''|*[!0-9]*|0) echo "invalid port: '${FLEET_PORT}' (must be a positive integer)" >&2; exit 1 ;;
esac

TARGET_HOST="${POSITIONAL[0]:-}"
TARGET_USER="${POSITIONAL[1]:-root}"
TARGET_PASS="${POSITIONAL[2]:-}"

if [ "$KEY_AUTH" = true ]; then
    TARGET_PASS=""
fi

if [[ -n "${TARGET_HOST}" && "${TARGET_HOST}" == *"@"* ]]; then
    TARGET_USER="${TARGET_HOST%%@*}"
    TARGET_HOST="${TARGET_HOST#*@}"
fi

_use_color() { [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; }
if _use_color; then
    C_OK=$'\033[32m'; C_FAIL=$'\033[31m'; C_INFO=$'\033[36m'; C_WARN=$'\033[33m'
    C_DIM=$'\033[2m'; C_BOLD=$'\033[1m'; C_CYAN=$'\033[96m'; C_RST=$'\033[0m'
else
    C_OK= C_FAIL= C_INFO= C_WARN= C_DIM= C_BOLD= C_CYAN= C_RST=
fi

_log_file() { mkdir -p "$(dirname "$DEPLOY_LOG")" 2>/dev/null || true; echo "[$(date -Iseconds)] $*" >>"$DEPLOY_LOG" 2>/dev/null || true; }
ok()   { echo "${C_OK}  ✓ $*${C_RST}"; _log_file "OK $*"; }
fail() { echo "${C_FAIL}  ✗ $*${C_RST}" >&2; _log_file "FAIL $*"; exit 1; }
info() { echo "${C_INFO}  → $*${C_RST}"; _log_file "INFO $*"; }
warn() { echo "${C_WARN}  ! $*${C_RST}"; _log_file "WARN $*"; }
dry()  { echo "${C_DIM}  (dry) $*${C_RST}"; _log_file "DRY $*"; }

print_banner() {
    local target="${TARGET_USER}@${TARGET_HOST}"
    echo ""
    echo "${C_CYAN}${C_BOLD}  Zyvor Fleet remote deploy${C_RST}  ${C_DIM}v${VERSION}${C_RST}"
    echo "${C_DIM}  ${target}  ·  ${DEPLOY_PROFILE}${C_RST}"
    [ "$DRY_RUN" = true ] && echo "${C_WARN}  dry-run — no remote changes${C_RST}"
    echo ""
}

STEP_T0=0
STEP_IDX=0
step_begin() {
    STEP_IDX=$((STEP_IDX + 1))
    STEP_T0=$(date +%s)
    echo ""
    echo "${C_BOLD}${C_CYAN}  Step ${STEP_IDX}: $*${C_RST}"
    _log_file "STEP ${STEP_IDX}: $*"
}
step_end() { echo "${C_DIM}  finished in $(( $(date +%s) - STEP_T0 ))s${C_RST}"; }

SSH_OPTS="-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=15 -o ServerAliveInterval=30"
if [ -z "${TARGET_PASS}" ]; then
    SSH_OPTS+=" -o BatchMode=yes -o PreferredAuthentications=publickey"
fi

_ssh_once() {
    if [ -n "${TARGET_PASS}" ] && command -v sshpass &>/dev/null; then
        export SSHPASS="${TARGET_PASS}"
        sshpass -e ssh ${SSH_OPTS} "${TARGET_USER}@${TARGET_HOST}" "$@"
    else
        ssh ${SSH_OPTS} "${TARGET_USER}@${TARGET_HOST}" "$@"
    fi
}

_ssh() {
    local attempt=1 max="${SSH_RETRIES}"
    while [ "$attempt" -le "$max" ]; do
        if _ssh_once "$@"; then
            return 0
        fi
        attempt=$((attempt + 1))
        if [ "$attempt" -le "$max" ]; then
            local _d=$(( 2 * (attempt - 1) )); _d=$(( _d < 2 ? 2 : _d > 30 ? 30 : _d ))
            warn "SSH retry ${attempt}/${max}" && sleep "${_d}"
        fi
    done
    return 1
}

# Prefer an explicit --port / FLEET_PORT; otherwise keep whatever the remote
# unit already listens on so quick redeploys do not bounce labs off a custom port.
resolve_listen_port() {
    if [ "$PORT_EXPLICIT" = true ]; then
        info "Listen port ${FLEET_PORT} (explicit)"
        return 0
    fi
    if [ "$DRY_RUN" = true ]; then
        info "Listen port ${FLEET_PORT} (default; dry-run skips remote probe)"
        return 0
    fi
    local existing=""
    existing="$(_ssh_once 'bash -s' <<'PROBE' 2>/dev/null || true
unit=/etc/systemd/system/zyvor-fleet.service
envf=/etc/zyvor-fleet/fleet.env
for f in "$envf" "$unit"; do
  [ -f "$f" ] || continue
  line=$(grep -E '^[[:space:]]*(Environment=)?ZYVOR_FLEET_LISTEN=' "$f" 2>/dev/null | tail -1 || true)
  [ -n "$line" ] || continue
  addr=${line#*ZYVOR_FLEET_LISTEN=}
  addr=${addr%%[[:space:]]*}
  addr=${addr#\"}
  addr=${addr%\"}
  case "$addr" in
    :[0-9]*|[0-9]*:[0-9]*)
      port=${addr##*:}
      case "$port" in
        ''|*[!0-9]*|0) ;;
        *) printf '%s\n' "$port"; exit 0 ;;
      esac
      ;;
  esac
done
# Also parse --listen from ExecStart
if [ -f "$unit" ]; then
  line=$(grep -E '^ExecStart=' "$unit" 2>/dev/null | tail -1 || true)
  if [[ "$line" =~ --listen[[:space:]]+:([0-9]+) ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    exit 0
  fi
fi
PROBE
)" || true
    existing="$(printf '%s' "$existing" | tr -d '[:space:]')"
    case "${existing}" in
        ''|*[!0-9]*|0) info "Listen port ${FLEET_PORT} (default)" ;;
        *)
            FLEET_PORT="$existing"
            info "Listen port ${FLEET_PORT} (preserving remote unit)"
            ;;
    esac
}

# Install or patch the systemd unit. New installs get a demo unit; existing
# units only get the listen address updated so secrets/state survive redeploys.
remote_install_service() {
    _ssh env FLEET_PORT="${FLEET_PORT}" REMOTE_STAGING="${REMOTE_DIR}" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
UNIT=/etc/systemd/system/zyvor-fleet.service
ENV_DIR=/etc/zyvor-fleet
ENV_FILE="${ENV_DIR}/fleet.env"
DATA_DIR=/var/lib/zyvor-fleet
ADDR=":${FLEET_PORT}"

ensure_user_and_dirs() {
  if ! id -u zyvor-fleet >/dev/null 2>&1; then
    $SUDO useradd --system --home "${DATA_DIR}" --shell /usr/sbin/nologin zyvor-fleet 2>/dev/null \
      || $SUDO useradd --system --home "${DATA_DIR}" --shell /sbin/nologin zyvor-fleet
  fi
  $SUDO mkdir -p "${ENV_DIR}" "${DATA_DIR}"
  $SUDO chown zyvor-fleet:zyvor-fleet "${DATA_DIR}"
  $SUDO chmod 0750 "${DATA_DIR}"
}

ensure_env() {
  if [ -f "${ENV_FILE}" ]; then
    if $SUDO grep -qE '^ZYVOR_FLEET_LISTEN=' "${ENV_FILE}"; then
      $SUDO sed -i "s|^ZYVOR_FLEET_LISTEN=.*|ZYVOR_FLEET_LISTEN=${ADDR}|" "${ENV_FILE}"
    else
      printf 'ZYVOR_FLEET_LISTEN=%s\n' "$ADDR" | $SUDO tee -a "${ENV_FILE}" >/dev/null
    fi
    $SUDO chmod 0600 "${ENV_FILE}"
    return 0
  fi
  SECRET="$(openssl rand -base64 48 | tr -d '\n')"
  $SUDO tee "${ENV_FILE}" >/dev/null <<EOF
ZYVOR_FLEET_LISTEN=${ADDR}
ZYVOR_FLEET_ADMIN_EMAIL=admin@zyvor.local
ZYVOR_FLEET_ADMIN_PASSWORD=zyvor-fleet-demo
ZYVOR_FLEET_SESSION_SECRET=${SECRET}
ZYVOR_FLEET_DEMO=1
ZYVOR_FLEET_DATA=${DATA_DIR}/state.json
EOF
  $SUDO chmod 0600 "${ENV_FILE}"
  echo "Wrote demo env: ${ENV_FILE}"
}

patch_unit_listen() {
  local unit="$1"
  if grep -qE '^Environment=ZYVOR_FLEET_LISTEN=' "$unit"; then
    $SUDO sed -i "s|^Environment=ZYVOR_FLEET_LISTEN=.*|Environment=ZYVOR_FLEET_LISTEN=${ADDR}|" "$unit"
  fi
  if grep -qE '^ExecStart=.*--listen'; then
    $SUDO sed -i "s|--listen :[0-9]*|--listen ${ADDR}|" "$unit"
  fi
}

ensure_user_and_dirs
ensure_env

if [ -f "$UNIT" ]; then
  patch_unit_listen "$UNIT"
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable --now zyvor-fleet.service
  $SUDO systemctl restart zyvor-fleet.service
  echo "Updated existing unit listen address to ${ADDR}"
  $SUDO systemctl --no-pager --full status zyvor-fleet.service | head -20 || true
  exit 0
fi

$SUDO tee "$UNIT" >/dev/null <<UNIT
[Unit]
Description=Zyvor Fleet control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=zyvor-fleet
Group=zyvor-fleet
EnvironmentFile=${ENV_FILE}
Environment=ZYVOR_FLEET_LISTEN=${ADDR}
ExecStart=/usr/local/bin/fleetd --demo --listen ${ADDR} --data ${DATA_DIR}/state.json
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}
CapabilityBoundingSet=
LockPersonality=true
MemoryDenyWriteExecute=true

[Install]
WantedBy=multi-user.target
UNIT

$SUDO systemctl daemon-reload
$SUDO systemctl enable --now zyvor-fleet.service
$SUDO systemctl --no-pager --full status zyvor-fleet.service | head -20 || true
REMOTE
}

_rsync() {
    local opts="-az --delete"
    [ "$VERBOSE" = true ] && opts+=" --progress"
    if [ -n "${TARGET_PASS}" ] && command -v sshpass &>/dev/null; then
        export SSHPASS="${TARGET_PASS}"
        rsync ${opts} -e "sshpass -e ssh ${SSH_OPTS}" "$@"
    else
        rsync ${opts} -e "ssh ${SSH_OPTS}" "$@"
    fi
}

validate() {
    [ -n "${TARGET_HOST}" ] || { usage; exit 1; }
    [ -f "${PROJECT_DIR}/go.mod" ] || fail "Not in zyvor-fleet repo: ${PROJECT_DIR}"
    if [ -n "${TARGET_PASS}" ]; then
        warn "Password auth is deprecated. Prefer: ssh-copy-id ${TARGET_USER}@${TARGET_HOST}"
        command -v sshpass &>/dev/null || fail "sshpass required for password auth"
    fi
}

check_connectivity() {
    info "SSH → ${TARGET_USER}@${TARGET_HOST}  log: ${DEPLOY_LOG}"
    if [ "$DRY_RUN" = true ]; then
        REMOTE_DIR="${DEPLOY_DIR:-${HOME}/.deployments/zyvor-fleet}"
        return 0
    fi
    _ssh "echo ok" &>/dev/null || fail "SSH failed — try: ssh-copy-id ${TARGET_USER}@${TARGET_HOST}"
    ok "SSH connected"
    local remote_home
    remote_home=$(_ssh "echo \$HOME" 2>/dev/null | tr -d '\r')
    remote_home="${remote_home:-/home/${TARGET_USER}}"
    REMOTE_DIR="${DEPLOY_DIR:-${remote_home}/.deployments/zyvor-fleet}"
    info "Remote path: ${REMOTE_DIR}"
}

preflight_remote() {
    info "Preflight on ${TARGET_HOST}..."
    if [ "$DRY_RUN" = true ]; then return 0; fi
    _ssh bash <<'REMOTE' || fail "Preflight failed"
set -e
echo "  host: $(hostname -f 2>/dev/null || hostname)"
echo "  os:   $(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" || uname -s)"
echo "  arch: $(uname -m)"
echo "  disk: $(df -h / 2>/dev/null | awk 'NR==2{print $4 " free"}' || echo n/a)"
if [ "$(id -u)" -ne 0 ]; then
    if ! sudo -n true 2>/dev/null; then
        echo "  non-root user needs passwordless sudo for install"
        exit 1
    fi
    echo "  passwordless sudo: ok"
else
    echo "  running as root"
fi
command -v curl >/dev/null && echo "  curl: ok" || echo "  curl: missing (needed to fetch Go)"
REMOTE
    ok "Preflight passed"
}

build_local_artifacts() {
    step_begin "Local build (Linux release)"
    if [ "$DRY_RUN" = true ]; then
        dry "would run: make build"
        step_end
        return 0
    fi
    if [ "$(uname -s)" != "Linux" ]; then
        fail "--build-local requires a Linux build host (same arch as remote)"
    fi
    (cd "${PROJECT_DIR}" && make build)
    [ -f "${PROJECT_DIR}/bin/fleetd" ] || fail "bin/fleetd missing"
    [ -f "${PROJECT_DIR}/bin/fleet-agent" ] || fail "bin/fleet-agent missing"
    [ -f "${PROJECT_DIR}/bin/fleetctl" ] || fail "bin/fleetctl missing"
    ok "Local binaries ready"
    step_end
}

sync_files() {
    if [ "$SKIP_SYNC" = true ]; then
        info "Skipping rsync (--skip-sync)"
        return 0
    fi
    if [ "$DRY_RUN" = true ]; then
        dry "would rsync → ${REMOTE_DIR}"
        return 0
    fi
    step_begin "Sync source"
    _ssh "mkdir -p '${REMOTE_DIR}'"
    _rsync \
        --exclude '.git' \
        --exclude 'bin' \
        --exclude 'data' \
        --exclude '.deploy-last' \
        --exclude '*.png' \
        "${PROJECT_DIR}/" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/"
    ok "Source synced to ${REMOTE_DIR}"
    step_end
}

sync_binaries_only() {
    step_begin "Sync release binaries"
    if [ "$DRY_RUN" = true ]; then
        dry "would rsync bin/fleetd bin/fleet-agent bin/fleetctl"
        step_end
        return 0
    fi
    [ -f "${PROJECT_DIR}/bin/fleetd" ] || fail "Missing bin/fleetd"
    _ssh "mkdir -p '${REMOTE_DIR}/bin'"
    _rsync "${PROJECT_DIR}/bin/fleetd" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/bin/fleetd"
    _rsync "${PROJECT_DIR}/bin/fleet-agent" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/bin/fleet-agent"
    _rsync "${PROJECT_DIR}/bin/fleetctl" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/bin/fleetctl"
    ok "Binaries synced"
    step_end
}

ensure_go_remote() {
    step_begin "Ensure Go toolchain"
    if [ "$DRY_RUN" = true ]; then
        dry "would ensure go 1.27+"
        step_end
        return 0
    fi
    if [ "$QUICK_MODE" = true ]; then
        if _ssh 'export PATH=/usr/local/go/bin:$PATH; command -v go >/dev/null'; then
            ok "Go already present (quick)"
            step_end
            return 0
        fi
        warn "Go missing on remote — installing anyway"
    fi
    _ssh env FLEET_GO_VERSION="${FLEET_GO_VERSION:-1.27.1}" FLEET_FORCE_GO_UPGRADE="${FLEET_FORCE_GO_UPGRADE:-}" bash <<'REMOTE'
set -euo pipefail
export PATH=/usr/local/go/bin:${PATH}
WANT="${FLEET_GO_VERSION:-1.27.1}"
if command -v go >/dev/null 2>&1; then
    ver=$(go env GOVERSION 2>/dev/null || go version)
    case "$ver" in
      go${WANT}|go${WANT}.*)
        echo "Go present: ${ver}"
        exit 0
        ;;
    esac
    # Also accept any patch within the same minor (e.g. want 1.27.1, have go1.27.0)
    want_minor="${WANT%.*}"
    cur_minor=$(printf '%s' "$ver" | sed -n 's/^go\([0-9]*\.[0-9]*\).*/\1/p')
    if [ "$cur_minor" = "$want_minor" ] && [ "${FLEET_FORCE_GO_UPGRADE:-}" != "1" ]; then
      echo "Go present: ${ver} (minor ${want_minor} ok)"
      exit 0
    fi
    echo "Upgrading Go ${ver} → ${WANT}"
fi
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac
GO_VER="${WANT}"
TMP=$(mktemp -d)
curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-${GOARCH}.tar.gz" -o "${TMP}/go.tgz"
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO rm -rf /usr/local/go
$SUDO tar -C /usr/local -xzf "${TMP}/go.tgz"
rm -rf "$TMP"
echo 'export PATH=/usr/local/go/bin:$PATH' | $SUDO tee /etc/profile.d/go.sh >/dev/null
export PATH=/usr/local/go/bin:$PATH
go version
REMOTE
    ok "Go ready"
    step_end
}

build_install_remote() {
    step_begin "Build + install on remote"
    if [ "$DRY_RUN" = true ]; then
        dry "would go build and install to /usr/local/bin"
        step_end
        return 0
    fi
    _ssh env REMOTE_STAGING="${REMOTE_DIR}" NO_SERVICE="$NO_SERVICE" VERSION="${VERSION}" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
export PATH=/usr/local/go/bin:${HOME}/go/bin:${PATH}
cd "${REMOTE_STAGING}"
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o bin/fleetd ./cmd/fleetd
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/zyvorai/zyvor-fleet/internal/agent.Version=${VERSION}" -o bin/fleet-agent ./cmd/fleet-agent
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o bin/fleetctl ./cmd/fleetctl
$SUDO install -m755 bin/fleetd /usr/local/bin/fleetd
$SUDO install -m755 bin/fleet-agent /usr/local/bin/fleet-agent
$SUDO install -m755 bin/fleetctl /usr/local/bin/fleetctl
echo "Installed: $(command -v fleetd) $(command -v fleet-agent) $(command -v fleetctl)"
if [ "${NO_SERVICE}" = "true" ]; then
  echo "Skipping systemd unit (--no-service)"
fi
REMOTE
    if [ "$NO_SERVICE" != true ]; then
        remote_install_service
    fi
    ok "Binaries installed"
    step_end
}

install_binaries_quick() {
    step_begin "Install synced binaries"
    if [ "$DRY_RUN" = true ]; then
        dry "would install remote bin/* to /usr/local/bin"
        step_end
        return 0
    fi
    _ssh env REMOTE_STAGING="${REMOTE_DIR}" NO_SERVICE="$NO_SERVICE" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO install -m755 "${REMOTE_STAGING}/bin/fleetd" /usr/local/bin/fleetd
$SUDO install -m755 "${REMOTE_STAGING}/bin/fleet-agent" /usr/local/bin/fleet-agent
$SUDO install -m755 "${REMOTE_STAGING}/bin/fleetctl" /usr/local/bin/fleetctl
if [ "${NO_SERVICE}" = "true" ]; then
  echo "Skipping systemd unit (--no-service)"
fi
echo "Installed binaries"
REMOTE
    if [ "$NO_SERVICE" != true ]; then
        remote_install_service
    fi
    ok "Quick install done"
    step_end
}

verify_remote() {
    step_begin "Verify health"
    if [ "$DRY_RUN" = true ]; then
        dry "would curl http://${TARGET_HOST}:${FLEET_PORT}/readyz"
        step_end
        return 0
    fi
    local url="http://${TARGET_HOST}:${FLEET_PORT}/readyz"
    local i=0
    while [ "$i" -lt 30 ]; do
        if curl -fsS --connect-timeout 2 "$url" >/dev/null 2>&1; then
            ok "Healthy at ${url}"
            info "UI: http://${TARGET_HOST}:${FLEET_PORT}/"
            info "Login: admin@zyvor.local / zyvor-fleet-demo"
            step_end
            return 0
        fi
        # Fall back to SSH-local curl if host port not reachable from here
        if _ssh "curl -fsS --connect-timeout 2 http://127.0.0.1:${FLEET_PORT}/readyz" >/dev/null 2>&1; then
            ok "Healthy on remote localhost:${FLEET_PORT} (open firewall / tunnel for public access)"
            info "ssh -L ${FLEET_PORT}:127.0.0.1:${FLEET_PORT} ${TARGET_USER}@${TARGET_HOST}"
            step_end
            return 0
        fi
        i=$((i + 1))
        sleep 1
    done
    fail "Health check failed for port ${FLEET_PORT}"
}

uninstall_remote() {
    step_begin "Uninstall"
    if [ "$DRY_RUN" = true ]; then
        dry "would stop zyvor-fleet.service and remove binaries/staging"
        step_end
        return 0
    fi
    _ssh env REMOTE_STAGING="${REMOTE_DIR}" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO systemctl disable --now zyvor-fleet.service 2>/dev/null || true
$SUDO rm -f /etc/systemd/system/zyvor-fleet.service
$SUDO systemctl daemon-reload 2>/dev/null || true
$SUDO rm -f /usr/local/bin/fleetd /usr/local/bin/fleet-agent /usr/local/bin/fleetctl
rm -rf "${REMOTE_STAGING}"
echo "Removed Zyvor Fleet install (left /var/lib/zyvor-fleet and /etc/zyvor-fleet intact)"
REMOTE
    ok "Uninstalled"
    step_end
}

main() {
    print_banner
    validate
    check_connectivity

    if [ "$UNINSTALL" = true ]; then
        uninstall_remote
        exit 0
    fi
    if [ "$PREFLIGHT_ONLY" = true ]; then
        preflight_remote
        exit 0
    fi
    if [ "$VERIFY_ONLY" = true ]; then
        resolve_listen_port
        verify_remote
        exit 0
    fi

    run_step_preflight() { step_begin "Preflight"; preflight_remote; step_end; }
    run_step_preflight
    resolve_listen_port

    if [ "$BUILD_LOCAL" = true ]; then
        build_local_artifacts
        sync_binaries_only
        install_binaries_quick
    else
        sync_files
        if [ "$QUICK_MODE" = false ] || [ "${FLEET_FORCE_GO_UPGRADE:-}" = "1" ] || ! _ssh 'export PATH=/usr/local/go/bin:$PATH; command -v go >/dev/null' 2>/dev/null; then
            ensure_go_remote
        else
            info "Skipping Go install (quick + go present)"
        fi
        build_install_remote
    fi

    if [ "$SKIP_VERIFY" = false ] && [ "$NO_SERVICE" = false ]; then
        verify_remote
    fi

    echo ""
    ok "Deploy complete"
    info "Docs: docs/DEPLOYMENT.md"
}

main
