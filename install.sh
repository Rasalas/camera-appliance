#!/usr/bin/env bash
set -euo pipefail

DEFAULT_RELEASE_URL="https://github.com/Rasalas/camera-appliance/releases/latest/download/camera-appliance-latest.tar.gz"
DEFAULT_INSTALLER_URL="https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh"
RELEASE_URL="${CAMERA_APPLIANCE_RELEASE_URL:-$DEFAULT_RELEASE_URL}"
INSTALL_DIR="/opt/camera-appliance"
EXPECTED_SHA256=""
USER_NAME="${CAMERA_APPLIANCE_USER:-}"
ENABLE_KIOSK=1
INSTALL_DESKTOP=1
ENABLE_SYSTEMD=1
NO_START=0

usage() {
  cat <<'USAGE'
Usage:
  curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh | sudo bash -s -- [options]

Options:
  --url URL                         Release archive URL (SHA256SUMS must sit next to it)
  --sha256 HEX                      Expected archive SHA-256 instead of the release's SHA256SUMS
  --user USER                       Linux desktop/kiosk user (auto-detected by default)
  --install-dir DIR                 Install directory (default: /opt/camera-appliance)
  --no-kiosk                        Do not enable the kiosk browser user service
  --no-desktop-launchers            Do not install desktop launchers
  --no-systemd                      Do not install/enable camera-appliance.service
  --no-start                        Install/enable services without starting them
  -h, --help                        Show this help

Hinweis:
  Das Skript installiert nur die Bootstrap-Abhängigkeiten, lädt ein Release, prüft es gegen
  die SHA256SUMS des Releases und ruft dann camera-appliance install oder camera-appliance update
  auf. Firewall-Regeln werden nicht geändert.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url)
      RELEASE_URL="${2:-}"
      shift 2
      ;;
    --sha256)
      EXPECTED_SHA256="${2:-}"
      shift 2
      ;;
    --user)
      USER_NAME="${2:-}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --enable-kiosk)
      ENABLE_KIOSK=1
      shift
      ;;
    --no-kiosk)
      ENABLE_KIOSK=0
      shift
      ;;
    --no-desktop-launchers)
      INSTALL_DESKTOP=0
      shift
      ;;
    --no-systemd)
      ENABLE_SYSTEMD=0
      shift
      ;;
    --no-start)
      NO_START=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unbekannte Option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Bitte mit sudo/root ausführen. Beispiel:" >&2
  echo "  curl -fsSL $DEFAULT_INSTALLER_URL | sudo bash" >&2
  exit 1
fi

if [[ -z "$RELEASE_URL" ]]; then
  echo "--url darf nicht leer sein." >&2
  exit 2
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

# Prints the lowercase digest listed for name in a sha256sum file.
checksum_for() {
  local sums_file="$1" name="$2"
  awk -v name="$name" 'NF == 2 { file = $2; sub(/^\*/, "", file); if (file == name) { print tolower($1); exit } }' "$sums_file"
}

# Fails unless file matches the expected SHA-256; label names it in messages.
verify_sha256() {
  local file="$1" expected="$2" label="$3" actual
  if [[ ! "$expected" =~ ^[0-9a-f]{64}$ ]]; then
    echo "Keine gültige SHA-256-Prüfsumme für $label gefunden; breche ab." >&2
    return 1
  fi
  actual="$(sha256sum "$file" | awk '{print $1}')"
  if [[ "$actual" != "$expected" ]]; then
    echo "SHA-256-Prüfung für $label fehlgeschlagen (erwartet $expected, erhalten $actual); breche ab." >&2
    return 1
  fi
  echo "Prüfsumme OK: $label"
}

need_or_install() {
  local missing_base=()
  for command_name in curl tar docker ffmpeg; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
      missing_base+=("$command_name")
    fi
  done

  if [[ "${#missing_base[@]}" -eq 0 ]] && docker compose version >/dev/null 2>&1; then
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    echo "Installiere fehlende Bootstrap-Abhängigkeiten."
    apt-get update
    apt-get install -y ca-certificates curl tar docker.io ffmpeg
    apt-get install -y docker-compose-plugin >/dev/null 2>&1 || apt-get install -y docker-compose-v2 >/dev/null 2>&1 || true
    systemctl enable --now docker >/dev/null 2>&1 || true
  elif [[ "${#missing_base[@]}" -gt 0 ]]; then
    echo "Fehlende Abhängigkeiten: ${missing_base[*]}" >&2
    echo "Bitte curl, tar, Docker und ffmpeg installieren und erneut ausführen." >&2
    exit 1
  fi

  if docker compose version >/dev/null 2>&1; then
    return 0
  fi

  install_compose_plugin_binary
  if ! docker compose version >/dev/null 2>&1; then
    echo "Docker ist installiert, aber 'docker compose' ist weiterhin nicht verfügbar." >&2
    echo "Bitte Docker Compose v2 Plugin installieren und erneut ausführen." >&2
    exit 1
  fi
}

install_compose_plugin_binary() {
  local arch url primary_dir primary_target plugin_dir
  case "$(uname -m)" in
    x86_64|amd64)
      arch="x86_64"
      ;;
    aarch64|arm64)
      arch="aarch64"
      ;;
    *)
      echo "Docker Compose Fallback unterstützt diese Architektur nicht: $(uname -m)" >&2
      return 1
      ;;
  esac

  primary_dir="/usr/local/lib/docker/cli-plugins"
  primary_target="$primary_dir/docker-compose"
  compose_version="${COMPOSE_VERSION:-v2.39.1}"
  asset="docker-compose-linux-$arch"
  url="https://github.com/docker/compose/releases/download/$compose_version/$asset"
  checksum_url="https://github.com/docker/compose/releases/download/$compose_version/checksums.txt"
  download="$tmp_dir/$asset"
  checksums="$tmp_dir/compose-checksums.txt"
  echo "Docker Compose Plugin fehlt; installiere Compose $compose_version nach $primary_target"
  if ! curl -fsSL "$checksum_url" -o "$checksums"; then
    echo "checksums.txt für Compose $compose_version konnte nicht geladen werden; ohne Prüfung wird nichts installiert." >&2
    return 1
  fi
  curl -fsSL "$url" -o "$download"
  verify_sha256 "$download" "$(checksum_for "$checksums" "$asset")" "$asset" || return 1
  mkdir -p "$primary_dir"
  install -m 0755 "$download" "$primary_target"

  for plugin_dir in \
    /usr/local/lib/docker/cli-plugins \
    /usr/local/libexec/docker/cli-plugins \
    /usr/lib/docker/cli-plugins \
    /usr/libexec/docker/cli-plugins
  do
    mkdir -p "$plugin_dir"
    if [[ "$plugin_dir/docker-compose" != "$primary_target" ]]; then
      ln -sfn "$primary_target" "$plugin_dir/docker-compose"
    fi
  done
}

detect_desktop_user() {
  if [[ -n "$USER_NAME" && "$USER_NAME" != "root" ]]; then
    return 0
  fi
  if [[ -n "${SUDO_USER:-}" && "${SUDO_USER:-}" != "root" ]]; then
    USER_NAME="$SUDO_USER"
    return 0
  fi
  local candidate=""
  candidate="$(logname 2>/dev/null || true)"
  if [[ -n "$candidate" && "$candidate" != "root" ]]; then
    USER_NAME="$candidate"
    return 0
  fi
  if [[ -e /dev/console ]]; then
    candidate="$(stat -c '%U' /dev/console 2>/dev/null || true)"
    if [[ -n "$candidate" && "$candidate" != "root" ]]; then
      USER_NAME="$candidate"
      return 0
    fi
  fi
  candidate="$(find /home -mindepth 1 -maxdepth 1 -type d -printf '%f\n' 2>/dev/null | head -n 1 || true)"
  if [[ -n "$candidate" ]]; then
    echo "WARNUNG: Desktop-Benutzer konnte nicht eindeutig erkannt werden; rate '$candidate'. Bitte --user angeben, falls falsch." >&2
    USER_NAME="$candidate"
    return 0
  fi
  return 1
}

find_release_binary() {
  local root="$1"
  local binary
  binary="$(find "$root" -path '*/bin/camera-appliance' -type f -perm -111 | head -n 1 || true)"
  if [[ -z "$binary" ]]; then
    binary="$(find "$root" -path '*/bin/camera-appliance' -type f | head -n 1 || true)"
  fi
  if [[ -z "$binary" ]]; then
    echo "Release enthält kein bin/camera-appliance Binary." >&2
    exit 1
  fi
  chmod +x "$binary"
  printf '%s\n' "$binary"
}

need_or_install
if [[ "$ENABLE_KIOSK" -eq 1 || "$INSTALL_DESKTOP" -eq 1 ]]; then
  if ! detect_desktop_user; then
    echo "Desktop-Benutzer konnte nicht automatisch erkannt werden." >&2
    echo "Bitte erneut mit --user BENUTZER ausführen oder --no-kiosk --no-desktop-launchers setzen." >&2
    exit 1
  fi
  echo "Desktop/Kiosk-Benutzer: $USER_NAME"
fi

archive="$tmp_dir/camera-appliance-release.tar.gz"
extract_dir="$tmp_dir/release"
mkdir -p "$extract_dir"

echo "Lade camera-appliance Release:"
echo "  $RELEASE_URL"
curl -fL "$RELEASE_URL" -o "$archive"
archive_url="${RELEASE_URL%%[?#]*}"
archive_name="${archive_url##*/}"
if [[ -z "$EXPECTED_SHA256" ]]; then
  sums_url="${archive_url%/*}/SHA256SUMS"
  if ! curl -fsSL "$sums_url" -o "$tmp_dir/SHA256SUMS"; then
    echo "SHA256SUMS konnte nicht geladen werden: $sums_url" >&2
    echo "Ohne Prüfsumme wird nichts installiert. Für ein eigenes Archiv --sha256 HEX angeben." >&2
    exit 1
  fi
  EXPECTED_SHA256="$(checksum_for "$tmp_dir/SHA256SUMS" "$archive_name")"
fi
EXPECTED_SHA256="$(printf '%s' "$EXPECTED_SHA256" | tr 'A-F' 'a-f')"
verify_sha256 "$archive" "$EXPECTED_SHA256" "$archive_name"
# Als root keine Ownership/Permissions aus dem Archiv übernehmen.
tar -xzf "$archive" --no-same-owner --no-same-permissions -C "$extract_dir"
release_binary="$(find_release_binary "$extract_dir")"

install_common_args=(--install-dir "$INSTALL_DIR" --digest "sha256:$EXPECTED_SHA256")
if [[ -n "$USER_NAME" ]]; then
  install_common_args+=(--user "$USER_NAME")
fi
if [[ "$NO_START" -eq 1 ]]; then
  install_common_args+=(--no-start)
fi
update_args=(--install-dir "$INSTALL_DIR" --digest "sha256:$EXPECTED_SHA256")
if [[ "$NO_START" -eq 1 ]]; then
  update_args+=(--no-restart)
fi

# Updates replace /opt only; refresh unit copies so boot/kiosk fixes reach old installs.
# They take effect on the next start, running services are not restarted.
refresh_installed_units() {
  local source="$INSTALL_DIR/systemd" target owner
  target="/etc/systemd/system/camera-appliance.service"
  if [[ -f "$target" && -f "$source/camera-appliance.service" ]] && ! cmp -s "$source/camera-appliance.service" "$target"; then
    install -m 0644 "$source/camera-appliance.service" "$target"
    systemctl daemon-reload >/dev/null 2>&1 || true
    echo "camera-appliance.service aktualisiert."
  fi
  for target in /home/*/.config/systemd/user/camera-kiosk.service; do
    if [[ -f "$target" && -f "$source/camera-kiosk.service" ]] && ! cmp -s "$source/camera-kiosk.service" "$target"; then
      owner="$(stat -c '%u:%g' "$target")"
      install -m 0644 -o "${owner%:*}" -g "${owner#*:}" "$source/camera-kiosk.service" "$target"
      echo "Kiosk-Dienst aktualisiert: $target (wirksam nach Neustart oder erneuter Anmeldung)."
    fi
  done
}

if [[ -x "$INSTALL_DIR/bin/camera-appliance" ]]; then
  echo "Bestehende Installation gefunden. Führe update aus."
  "$release_binary" update --archive "$archive" "${update_args[@]}"
  refresh_installed_units
else
  echo "Keine bestehende Installation gefunden. Führe Erstinstallation aus."
  # camera-appliance.service starts existing images without --build.
  release_root="$(dirname "$(dirname "$release_binary")")"
  echo "Baue Docker-Images; das kann mehrere Minuten dauern."
  compose_build=(docker compose --project-name camera-appliance)
  if [[ -f "$release_root/release.env" ]]; then
    compose_build+=(--env-file "$release_root/release.env")
  fi
  "${compose_build[@]}" -f "$release_root/compose.yaml" build
  install_args=(install --archive "$archive" "${install_common_args[@]}")
  if [[ "$ENABLE_SYSTEMD" -eq 1 ]]; then
    install_args+=(--enable-systemd)
  fi
  if [[ "$ENABLE_KIOSK" -eq 1 ]]; then
    install_args+=(--enable-kiosk)
  fi
  if [[ "$INSTALL_DESKTOP" -eq 1 ]]; then
    install_args+=(--install-desktop-launchers)
  fi
  "$release_binary" "${install_args[@]}"
fi

cat <<'NEXT'

Nächste Schritte:
  1. /etc/camera-appliance/secrets.env prüfen und change-me Werte ersetzen.
  2. http://127.0.0.1:8091 auf dem Kunden-Laptop öffnen.
  3. Kameras suchen, zuordnen, go2rtc-Konfiguration rendern und go2rtc neu starten.

Was dieses Skript nicht automatisch macht:
  - GitHub/Release-Sichtbarkeit konfigurieren.
  - Kamera-Passwörter erraten oder setzen.
  - Firewall-Ports öffnen. Das ist für den lokalen Kiosk-Betrieb nicht nötig.
NEXT
