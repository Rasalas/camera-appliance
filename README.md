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

Die Seitenleiste trennt **Live-Ansicht**, **Kameras**, **System** und **Wartung**.
Unter System stehen Allgemein, Zugriff, Relays und Identitäten. Wartung enthält
fünf eigene Seiten: Watchdog, Sicherung, Version und Updates, Support-Bundle und
Ereignisprotokoll. Bestehende Links wie `/system/bild-upload`, `/backup` und
`/events` werden auf die passenden Seiten weitergeleitet. Auf schmalen
Bildschirmen öffnet **Menü** dieselbe Navigation.

Einstellungsformulare zeigen ungespeicherte Änderungen sowie Fehler direkt am
Formular. Speichern und Abbrechen gelten jeweils für den benannten Bereich;
beispielsweise hat **Anzeige** einen eigenen Speicherknopf. Bildausschnitt,
Privatbereiche und Zeitangabe in den Kameradetails werden weiterhin automatisch
gespeichert und zeigen ihren Speicherzustand an.

Unter **Kameras → Bild-Upload** lassen sich Protokoll, Server, Port, Benutzername,
Passwort und ein vorhandenes Zielverzeichnis konfigurieren. SFTP benötigt zusätzlich
den SHA256-Fingerabdruck des SSH-Hostschlüssels vom Serverbetreiber. FTP ist
unverschlüsselt; SFTP verschlüsselt sowohl Bilder als auch Zugangsdaten.

In **Kameras → Kamera → Bild-Upload** nimmt „Jetzt hochladen“ ein neues JPEG
aus dem ausgewählten Kamerastream auf. Der vorhandene
Kamerazugang und die direkte oder Relay-Verbindung werden weiterverwendet.
Die Kameravorschau lädt beim Öffnen automatisch. „Ausschnitt“ erlaubt die Auswahl
eines Rahmens im Originalbild; Prozentwerte stehen unter „Genaue Werte“ zur Verfügung.
Änderungen werden automatisch pro Kamera gespeichert, mit kurzer Status- oder
Fehlermeldung. Der Upload verwendet immer die aktuell angezeigte Auswahl.
Mit „Vollbild“ wird alternativ das gesamte Originalbild
hochgeladen. Die Anzeige-Transforms des Viewers ändern das Upload-Bild nicht.

Unter „Automatisch“ sind pro Kamera Uploads jede Minute, alle 5 oder 15 Minuten
und jede Stunde wählbar. „Aus“ deaktiviert sie. Der Hintergrunddienst verwendet
den gespeicherten Kamerazugang, Stream und Bildbereich und läuft auch bei
geschlossenem Browser. Der erste Lauf erfolgt nach dem gewählten Intervall;
Einstellungen und nächster Termin bleiben bei Neustarts erhalten. Verpasste
Intervalle werden zu höchstens einem Lauf zusammengefasst. Uploads laufen
nacheinander; ein laufender Upload wird beim Deaktivieren noch beendet.

Optional pausiert eine tägliche Ruhezeit die automatischen Uploads. Auch
„22:00 bis 07:00“ über Mitternacht ist möglich. Der Beginn ist eingeschlossen,
das Ende nicht. Maßgeblich ist die angezeigte lokale Gerätezeit, einschließlich
Sommerzeitwechsel; die Docker-Konfiguration übernimmt dafür `/etc/localtime`
vom Host. Bei Beginn der Ruhezeit wird ein laufender automatischer Upload
abgebrochen. Nach der Pause wird ein fälliger Lauf ausgeführt, ohne die ausgelassenen
Bilder nachzuholen. Der manuelle Upload bleibt auch während der Ruhezeit möglich.
Status, letzter Erfolg und Fehler erscheinen in der Kameradetailseite.

Mit „+ Schwärzen“ oder „+ Verpixeln“ lassen sich bis zu 16 rechteckige
Privatbereiche aufziehen. Jeder Bereich ist über seine Nummer auswählbar,
verschiebbar und an der unteren rechten Ecke vergrößerbar. Darstellungsart,
genaue Position und Entfernen stehen unter dem Bild. Die Änderungen werden
automatisch pro Kamera gespeichert. Die Bereiche bleiben am Originalbild
verankert, auch wenn der Upload-Ausschnitt geändert wird. Der Server schwärzt
oder verpixelt jedes frische Bild vor dem Zuschnitt; Schwarz hat bei
Überlappungen Vorrang. Verpixelung verwendet höchstens acht grobe Blöcke auf
der längeren Seite eines Bereichs. Fehler beim Lesen oder Anwenden der
Privatbereiche brechen den Upload ab. Die Maskierung gilt für die Bild-Uploads,
nicht für die Liveansicht oder das ursprüngliche Referenzbild.

„Datum und Uhrzeit einblenden“ ergänzt optional die Aufnahmezeit des Geräts
im Format `05.09.2026 12:34:56`, unten rechts im fertigen Bild. Auch nach einem
Zuschnitt bleibt die Einblendung vollständig im Bild. Sie verwendet weiße
Ziffern auf einem schwarzen Hintergrund und gibt keine maskierten Bildteile
frei. Dafür muss das fertige Bild mindestens 127 × 21 Pixel groß sein; andernfalls
wird der Upload mit einer Fehlermeldung abgebrochen. Masken und Zeitangabe
gelten gleichermaßen für manuelle und automatische FTP-/SFTP-Uploads.

Unter „Dateien“ legt jede Kamera fest, ob jeder Upload eine neue Datei erhält
oder dieselbe Datei ersetzt. Standard bleibt ein eindeutiger Name mit
Kamera-Kennung und UTC-Zeit. Für ein stets aktuelles Bild „Dieselbe Datei ersetzen“
wählen und z. B. `hof.jpg` eingeben. Die Auswahl wird automatisch gespeichert und
gilt für manuelle und automatische Uploads sowie Vollbilder und Ausschnitte.
Der feste Name darf höchstens 120 Zeichen enthalten, beginnt mit einem Buchstaben
oder einer Zahl und verwendet nur `A–Z`, `a–z`, `0–9`, Punkt, Bindestrich und
Unterstrich sowie die Endung `.jpg` oder `.jpeg`. Jede Kamera sollte einen eigenen
festen Namen oder Ordner erhalten, sonst ersetzen sich ihre Bilder gegenseitig.

Im Feld „Verzeichnis“ kann jede Kamera ein eigenes Ziel angeben, z. B.
`/bilder/hof` oder `bilder/garage`. Ein leeres Feld verwendet weiterhin das
Standardverzeichnis unter **Kameras → Bild-Upload**. Ein Kamera-Verzeichnis
ersetzt dieses Ziel vollständig und wird nicht daran angehängt. Relative Pfade
beginnen für FTP und SFTP im Anmeldeverzeichnis des Servers; absolute Pfade
beginnen mit `/`. Unterordner und Leerzeichen innerhalb von Ordnernamen sind
erlaubt. `..`, Backslashes, URLs und Steuerzeichen sind nicht erlaubt; höchstens
1024 Bytes. Die Einstellung wird pro Kamera automatisch gespeichert und gilt
für beide Dateinamenmodi, manuelle Aufnahmen und die Zeitsteuerung. Fehlende oder
nicht beschreibbare Ordner melden einen Fehler und weichen nicht auf das globale
Ziel aus. Die Ordner müssen bereits auf dem Server angelegt sein.

Das Zielverzeichnis muss existieren und Schreiben sowie Umbenennen erlauben.
Eine Übertragung wird zunächst unter einem eindeutigen `.part`-Namen geschrieben
und erst nach vollständiger Übertragung unter dem endgültigen JPEG-Namen
veröffentlicht. FTP muss das Ersetzen per Rename erlauben; für SFTP wird die
Server-Erweiterung `posix-rename@openssh.com` verwendet. Verweigert der Server
die Ersetzung, wird ein Fehler gemeldet. Die bisherige Zieldatei wird nicht vorab
gelöscht. Bei Netzwerkabbruch können
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
