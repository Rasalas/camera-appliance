#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BIN_DIR="$ROOT/bin"
CONFIG_DIR="$ROOT/config"
STATE_DIR="$ROOT/data"
RUN_DIR="$STATE_DIR/run"
LOG_DIR="$STATE_DIR/log"
GENERATED_CONFIG="$STATE_DIR/generated/go2rtc.yaml"
RUNTIME_CONFIG="$STATE_DIR/generated/go2rtc-native.yaml"
MANAGER_PID="$RUN_DIR/camera-appliance.pid"
GO2RTC_PID="$RUN_DIR/go2rtc.pid"

mkdir -p "$CONFIG_DIR" "$STATE_DIR/generated" "$RUN_DIR" "$LOG_DIR"
umask 077

export CAMERA_APPLIANCE_BIND_ADDR="${CAMERA_APPLIANCE_BIND_ADDR:-0.0.0.0:8091}"
export CAMERA_APPLIANCE_CONFIG_DIR="$CONFIG_DIR"
export CAMERA_APPLIANCE_STATE_DIR="$STATE_DIR"
export CAMERA_APPLIANCE_FRONTEND_DIST="$ROOT/frontend/dist"
export CAMERA_APPLIANCE_GO2RTC_URL="http://127.0.0.1:1984"
export CAMERA_APPLIANCE_GO2RTC_RTSP_URL="rtsp://127.0.0.1:8554"
export CAMERA_APPLIANCE_GO2RTC_RESTART_COMMAND="$ROOT/deploy/nas/restart-go2rtc.sh"

is_running() {
  local pid_file="$1"
  local expected="$2"
  local pid=""
  [[ -s "$pid_file" ]] || return 1
  pid="$(<"$pid_file")"
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  [[ "$(readlink -f "/proc/$pid/exe" 2>/dev/null || true)" == "$(readlink -f "$expected")" ]]
}

stop_pid() {
  local pid_file="$1"
  local expected="$2"
  local pid=""
  if ! is_running "$pid_file" "$expected"; then
    rm -f "$pid_file"
    return 0
  fi
  pid="$(<"$pid_file")"
  kill "$pid" 2>/dev/null || true
  for _ in {1..25}; do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.2
  done
  if kill -0 "$pid" 2>/dev/null; then
    kill -9 "$pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
}

write_runtime_config() {
  if [[ ! -f "$GENERATED_CONFIG" ]]; then
    printf 'streams: {}\n' > "$GENERATED_CONFIG"
  fi
  local tmp="$RUNTIME_CONFIG.tmp"
  {
    printf 'api:\n  listen: "127.0.0.1:1984"\n'
    printf 'rtsp:\n  listen: "127.0.0.1:8554"\n'
    printf 'webrtc:\n  listen: "127.0.0.1:8555"\n\n'
    sed '/^api:/,$!b; /^api:/,$d' "$GENERATED_CONFIG"
  } > "$tmp"
  mv "$tmp" "$RUNTIME_CONFIG"
}

start_go2rtc() {
  if is_running "$GO2RTC_PID" "$BIN_DIR/go2rtc"; then
    return 0
  fi
  write_runtime_config
  nohup "$BIN_DIR/go2rtc" -c "$RUNTIME_CONFIG" >> "$LOG_DIR/go2rtc.log" 2>&1 9>&- &
  echo "$!" > "$GO2RTC_PID"
  for _ in {1..30}; do
    curl -fsS --max-time 1 http://127.0.0.1:1984/api/streams >/dev/null 2>&1 && return 0
    sleep 0.2
  done
  echo "go2rtc ist nicht erreichbar; siehe $LOG_DIR/go2rtc.log" >&2
  return 1
}

restart_go2rtc() {
  stop_pid "$GO2RTC_PID" "$BIN_DIR/go2rtc"
  start_go2rtc
}

start_manager() {
  if is_running "$MANAGER_PID" "$BIN_DIR/camera-appliance"; then
    return 0
  fi
  nohup "$BIN_DIR/camera-appliance" serve >> "$LOG_DIR/camera-appliance.log" 2>&1 9>&- &
  echo "$!" > "$MANAGER_PID"
  for _ in {1..30}; do
    curl -fsS --max-time 1 http://127.0.0.1:8091/api/auth/status >/dev/null 2>&1 && return 0
    sleep 0.2
  done
  echo "camera-appliance ist nicht erreichbar; siehe $LOG_DIR/camera-appliance.log" >&2
  return 1
}

ensure_services() {
  if [[ ! -x "$BIN_DIR/camera-appliance" || ! -x "$BIN_DIR/go2rtc" ]]; then
    echo "Binaries fehlen unter $BIN_DIR" >&2
    return 1
  fi
  if ! is_running "$GO2RTC_PID" "$BIN_DIR/go2rtc" || [[ "$GENERATED_CONFIG" -nt "$RUNTIME_CONFIG" ]]; then
    restart_go2rtc
  fi
  start_manager
}

stop_services() {
  stop_pid "$MANAGER_PID" "$BIN_DIR/camera-appliance"
  stop_pid "$GO2RTC_PID" "$BIN_DIR/go2rtc"
}

exec 9>"$RUN_DIR/native-service.lock"
flock -n 9 || exit 0

case "${1:-ensure}" in
  start|ensure)
    ensure_services
    ;;
  restart-go2rtc)
    restart_go2rtc
    ;;
  stop)
    stop_services
    ;;
  restart)
    stop_services
    ensure_services
    ;;
  prepare-go2rtc)
    write_runtime_config
    ;;
  *)
    echo "Usage: $0 {start|ensure|restart-go2rtc|stop|restart|prepare-go2rtc}" >&2
    exit 2
    ;;
esac
