#!/usr/bin/env bash
set -euo pipefail

# Run a downloaded release CLI on the host. The old container's Docker 20.10
# client cannot start an updater against a daemon requiring API >= 1.44.
REPO_URL="https://github.com/Rasalas/camera-appliance"
RAW_URL="https://raw.githubusercontent.com/Rasalas/camera-appliance"
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

# Pin installer and archive to one tag so neither main nor a newer release can mix in.
latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$REPO_URL/releases/latest")"
tag="${latest_url##*/releases/tag/}"
if [[ "$tag" == "$latest_url" || ! "$tag" =~ ^v[0-9A-Za-z._-]+$ ]]; then
  echo "Aktuelles Release konnte nicht ermittelt werden ($latest_url). Reparatur abgebrochen." >&2
  exit 1
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
curl -fsSL "$RAW_URL/$tag/install.sh" -o "$work_dir/install.sh"
echo "Aktualisiere auf Release $tag über den Docker-Client des Laptops."
echo 'Das Update erstellt vorher Backup und Rollback-Snapshot.'
bash "$work_dir/install.sh" --url "$REPO_URL/releases/download/$tag/camera-appliance-latest.tar.gz" --install-dir "$INSTALL_DIR" --no-kiosk --no-desktop-launchers
echo 'Reparatur abgeschlossen. Browser neu laden; weitere Updates sind über den Update-Button möglich.'
