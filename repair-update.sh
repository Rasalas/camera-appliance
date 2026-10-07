#!/usr/bin/env bash
set -euo pipefail

# Run a downloaded release CLI on the host. The old container's Docker 20.10
# client cannot start an updater against a daemon requiring API >= 1.44.
RELEASE_URL="https://github.com/Rasalas/camera-appliance/releases/latest/download/camera-appliance-latest.tar.gz"
INSTALL_DIR="${CAMERA_APPLIANCE_INSTALL_DIR:-/opt/camera-appliance}"

if [[ "$(uname -s)" != Linux ]]; then
  echo 'Diese Reparatur ist für den Linux-Kundenrechner bestimmt.' >&2
  exit 1
fi
if [[ "$(id -u)" != 0 ]]; then
  echo 'Bitte mit sudo bash repair-update.sh ausführen.' >&2
  exit 1
fi
if [[ ! -x "$INSTALL_DIR/bin/camera-appliance" ]]; then
  echo "Bestehende Installation fehlt unter $INSTALL_DIR. Reparatur abgebrochen." >&2
  exit 1
fi
docker version >/dev/null

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh -o "$work_dir/install.sh"
echo 'Aktualisiere auf das aktuelle Release über den Docker-Client des Laptops.'
echo 'Das Update erstellt vorher Backup und Rollback-Snapshot.'
bash "$work_dir/install.sh" --url "$RELEASE_URL" --install-dir "$INSTALL_DIR" --no-kiosk --no-desktop-launchers
echo 'Reparatur abgeschlossen. Browser neu laden; weitere Updates sind über den Update-Button möglich.'
