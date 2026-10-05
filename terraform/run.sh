#!/usr/bin/env bash
# Installs STAY from a GitHub release and starts it if it is not already running.
# Settings arrive as STAY_* environment variables (see main.tf). Safe to re-run:
# a running daemon is never restarted, because that would end the user's shells;
# a newly installed version takes effect the next time the daemon starts.
set -euo pipefail

: "${STAY_VERSION:=latest}"
: "${STAY_PORT:=7681}"
: "${STAY_REPO:=bbushvt/stay}"
: "${STAY_INSTALL_DIR:=$HOME/.local/bin}"
: "${STAY_LAYOUT_B64:=}"
# Override for testing or mirrors; any base that serves <base>/v<ver>/<file>.
: "${STAY_DOWNLOAD_BASE:=https://github.com/${STAY_REPO}/releases/download}"

STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/stay"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/stay"
BIN="$STAY_INSTALL_DIR/stay"
mkdir -p "$STATE_DIR" "$CONFIG_DIR" "$STAY_INSTALL_DIR"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

log() { echo "[stay] $*"; }

if [ -n "$STAY_LAYOUT_B64" ]; then
  printf '%s' "$STAY_LAYOUT_B64" | base64 -d > "$CONFIG_DIR/layout.yaml"
  log "wrote $CONFIG_DIR/layout.yaml"
fi

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *) log "unsupported architecture $(uname -m)"; exit 1 ;;
esac
[ "$(uname -s)" = Linux ] || { log "only Linux is supported"; exit 1; }

resolve_latest() {
  # Follow the /releases/latest redirect to .../tag/vX.Y.Z (no API rate limits).
  local url
  url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${STAY_REPO}/releases/latest")
  local ver="${url##*/v}"
  # No releases yet: the redirect lands on /releases and this isn't a version.
  [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] && echo "$ver"
}

install_stay() {
  local ver="$STAY_VERSION"
  if [ "$ver" = latest ]; then
    ver=$(resolve_latest) || true
    [ -n "$ver" ] || { log "could not resolve latest version (no releases yet?)"; return 1; }
  fi
  if [ -x "$BIN" ] && [ "$("$BIN" --version 2>/dev/null | awk '{print $2}')" = "$ver" ]; then
    log "stay $ver already installed"
    return 0
  fi

  local file="stay_${ver}_linux_${ARCH}.tar.gz"
  log "downloading stay $ver ($ARCH)"
  curl -fsSL -o "$TMP/$file" "$STAY_DOWNLOAD_BASE/v${ver}/${file}"
  curl -fsSL -o "$TMP/checksums.txt" "$STAY_DOWNLOAD_BASE/v${ver}/checksums.txt"
  (cd "$TMP" && grep -F " $file" checksums.txt | sha256sum -c --quiet -) \
    || { log "checksum verification failed"; return 1; }
  tar -xzf "$TMP/$file" -C "$TMP" stay
  install -m 0755 "$TMP/stay" "$BIN.new"
  mv -f "$BIN.new" "$BIN" # atomic; safe even while an older daemon runs
  log "installed $BIN ($("$BIN" --version))"
}

# A failed upgrade must not stop an already-installed daemon from starting.
if ! install_stay; then
  [ -x "$BIN" ] || { log "install failed and no existing binary"; exit 1; }
  log "install failed; continuing with the existing binary"
fi

running() { curl -fs -o /dev/null --max-time 2 "http://127.0.0.1:${STAY_PORT}/"; }

if running; then
  log "already running on 127.0.0.1:${STAY_PORT}"
  exit 0
fi

log "starting on 127.0.0.1:${STAY_PORT}"
# setsid + nohup: the daemon must outlive this script and its session.
setsid nohup "$BIN" --listen "127.0.0.1:${STAY_PORT}" \
  >> "$STATE_DIR/stay.log" 2>&1 < /dev/null &
echo $! > "$STATE_DIR/stay.pid"

for _ in $(seq 1 50); do
  if running; then log "up (log: $STATE_DIR/stay.log)"; exit 0; fi
  sleep 0.2
done
log "did not become ready; see $STATE_DIR/stay.log"
tail -n 20 "$STATE_DIR/stay.log" || true
exit 1
