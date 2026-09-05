# camera-appliance

`camera-appliance` turns a Linux Mint laptop into a local camera viewing appliance.

The Go manager discovers cameras, stores stable device identities, binds devices to slots, renders go2rtc config, and serves a local German kiosk viewer/admin UI on `127.0.0.1:8091`. go2rtc exposes stable stream aliases (`cam1` through `cam5`) for the viewer.

## Architecture

```text
Tapo cameras -> camera-appliance discovery/state -> go2rtc stable aliases -> camera-appliance Vue viewer
```

Camera identity is never bound to IP address alone. Devices store MAC, ONVIF endpoint reference, serial number, manufacturer, model, hardware ID, hostname, and last known IP.

Project decisions are documented in [docs/decisions](docs/decisions/README.md).

## Local Development

```bash
make dev
```

This starts a local go2rtc helper, builds the Vue frontend, builds the Go binary, uses local dev state in `./data`, and serves the admin UI at:

- [http://127.0.0.1:8091](http://127.0.0.1:8091)

On macOS, go2rtc runs natively from `./bin/go2rtc` because Docker Desktop containers may not reach LAN cameras reliably. On other platforms, the Makefile falls back to a local Docker helper container.

For frontend hot reload:

```bash
make dev-hot
```

That starts the Go backend on `127.0.0.1:8091` and Vite on its printed local URL, usually `http://127.0.0.1:5173`.

Useful development commands:

```bash
make test
make build
make dev-go2rtc
make stop-dev-go2rtc
make status
make discover
make render-go2rtc
make compose-config
make clean
```

## CLI

```bash
camera-appliance serve
camera-appliance status
camera-appliance discover
camera-appliance assign --slot cam1 --device DEVICE_ID --username tapo_hof --label Hof --stream stream2
camera-appliance render-go2rtc
camera-appliance restart-go2rtc
camera-appliance restart-stack
camera-appliance reset-bindings --yes
camera-appliance backup --out /var/lib/camera-appliance/backups/appliance.tar.gz
camera-appliance restore --in /var/lib/camera-appliance/backups/appliance.tar.gz
```

## Runtime Paths

- Code: `/opt/camera-appliance`
- Secrets: `/etc/camera-appliance/secrets.env`
- Local config: `/etc/camera-appliance/local.env`
- State: `/var/lib/camera-appliance/state.db`
- Generated go2rtc config: `/var/lib/camera-appliance/generated/go2rtc.yaml`
- Backups: `/var/lib/camera-appliance/backups`

Generated config can contain camera credentials and is ignored by Git.

## Secrets

Do not commit real credentials. Copy `.env.example` to `/etc/camera-appliance/secrets.env` and replace `change-me` values locally.

All CLI/API/UI output redacts credential-containing URLs. The admin UI binds to localhost by default.

## Viewer

The normal camera view is the local UI at:

- [http://127.0.0.1:8091](http://127.0.0.1:8091)

The viewer consumes only stable go2rtc aliases:

- `rtsp://go2rtc:8554/cam1`
- `rtsp://go2rtc:8554/cam2`
- `rtsp://go2rtc:8554/cam3`
- `rtsp://go2rtc:8554/cam4`
- `rtsp://go2rtc:8554/cam5`

Do not put camera DHCP IPs into viewer configuration.

## Einzelbilder per FTP/SFTP hochladen

Unter **System → Bild-Upload** lassen sich Protokoll, Server, Port, Benutzername,
Passwort und ein vorhandenes Zielverzeichnis konfigurieren. SFTP benötigt zusätzlich
den SHA256-Fingerabdruck des SSH-Hostschlüssels vom Serverbetreiber. FTP ist
unverschlüsselt; SFTP verschlüsselt sowohl Bilder als auch Zugangsdaten.

In **Einrichtung → Kamera → Einzelbild hochladen** nimmt „Jetzt aufnehmen &
hochladen“ ein neues JPEG aus dem ausgewählten Kamerastream auf. Der vorhandene
Kamerazugang und die direkte oder Relay-Verbindung werden weiterverwendet.
Eine Vorschau kann über „Vorschau aufnehmen“ geladen werden. „Nur Bildausschnitt“
erlaubt die Auswahl eines Rahmens im Originalbild oder die Eingabe von Prozentwerten.
„Bildbereich speichern“ merkt sich diese Auswahl pro Kamera; der Upload verwendet
immer die aktuell angezeigte Auswahl. Alternativ wird das gesamte Originalbild
hochgeladen. Die Anzeige-Transforms des Viewers ändern das Upload-Bild nicht.

Es gibt keinen Intervallbetrieb und kein Videoarchiv. Jede Datei erhält einen
eindeutigen Namen mit Kamera-Kennung und UTC-Zeit. Das Zielverzeichnis muss existieren
und Schreiben sowie Umbenennen erlauben: Eine Übertragung wird zunächst als `.part`
geschrieben und erst danach als `.jpg` veröffentlicht. Bei Netzwerkabbruch können
`.part`-Dateien auf dem Server zurückbleiben. Ein Upload hat maximal 30 Sekunden
Übertragungszeit zuzüglich der bestehenden Aufnahmezeit von bis zu 8 Sekunden.

Das Serverpasswort liegt mit Dateimodus `0600` in
`/etc/camera-appliance/snapshot-upload-password.json`. Die API liefert nur zurück,
ob ein Passwort vorhanden ist. Ein leeres Passwortfeld behält das gespeicherte
Passwort für dasselbe Ziel; Änderungen an Server, Port, Protokoll, Benutzer oder
SSH-Hostschlüssel benötigen ein neues Passwort. Das Passwort lässt sich dort auch
löschen. Geschützte Backups enthalten diese Datei, Support-Bundles nicht.

Die Funktion erfordert `ffmpeg` für die vorhandene Einzelbildaufnahme. Installer
und Docker-Image bringen es mit; bei älteren nativen Installationen kann es mit
`sudo apt install ffmpeg` nachinstalliert werden. Bei einem Capture-Hop muss es auf
dem konfigurierten SSH-Host verfügbar sein. Der Go-Build benötigt Go 1.26 oder neuer
für die verwendeten SSH/SFTP-Bibliotheken.

## Optional AgentDVR

AgentDVR is not required for normal install, startup, status, or camera display. It remains available only as an optional Docker Compose profile for NVR experiments:

```bash
sudo docker compose --profile agentdvr up -d agentdvr
```

If used, configure it manually with the same stable go2rtc aliases and never with camera DHCP IPs.

## Linux Mint Install

On a normal Linux Mint laptop, install `curl` first if it is missing:

```bash
sudo apt update
sudo apt install -y curl
```

The bootstrap installer installs the remaining base dependencies where possible:

- `ca-certificates`
- `tar`
- `ffmpeg`
- Docker Engine (`docker.io`)
- Docker Compose plugin (`docker-compose-plugin`)

If the distro package does not provide `docker compose`, the installer downloads the official Docker Compose v2 plugin for the local CPU architecture.

```bash
curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh | sudo bash
```

The bootstrap script downloads the latest public release archive. On a fresh laptop it calls `camera-appliance install`; on an existing appliance it calls `camera-appliance update`. It auto-detects the desktop user and enables the kiosk browser by default. It does not overwrite an existing `/etc/camera-appliance/secrets.env` and does not change firewall rules.

After the bootstrap install, the CLI is linked into `/usr/local/bin`, so regular updates can be run directly:

```bash
sudo camera-appliance update
```
Open `http://127.0.0.1:8091`, set the camera password in the local UI, discover cameras, assign devices to `cam1` through `cam5`, render go2rtc config, and restart go2rtc.

## Recovery

Use `camera-appliance backup` before customer changes. Use `camera-appliance restore --in FILE` to restore state and generated config, then restart the stack.
