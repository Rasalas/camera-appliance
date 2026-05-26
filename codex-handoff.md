# Camera Appliance - Codex Handoff

## 1. Implementation Directive

Build the complete local MVP described in this document.

Do not stop at scaffolding. Do not only create placeholders unless hardware-specific behavior cannot be implemented without real cameras. Implement everything that can reasonably be implemented and tested locally.

The application should be ready to install on a Linux Mint laptop and then be tested with real Tapo cameras in a customer network.

Hardware-specific and customer-specific setup, such as joining the final Wi-Fi network, entering real camera credentials, assigning real cameras, configuring Tailscale, and validating real AgentDVR streams on-site, can remain manual runtime steps. Everything else should be implemented.

## 2. Project Name

Repository name:

```text
camera-appliance
````

Product/UI name may later become:

```text
WatchDeck
```

For now, use `camera-appliance` consistently in code, CLI, paths, docs, and service names.

## 3. Project Purpose

This project turns a Linux Mint laptop into a local camera viewing appliance.

The target customer has Tapo cameras in a local Wi-Fi network. The laptop should automatically start a camera viewing system after boot. A non-technical user should ideally only need to turn on the laptop and see the cameras.

The system should use:

* AgentDVR as the static camera viewer.
* go2rtc as a stable RTSP stream alias layer.
* A custom Go application named `camera-appliance` as the local manager.
* Vue 3 as the local admin/setup UI.

The camera-manager must discover cameras, store stable device identities, bind physical cameras to logical slots, generate go2rtc configuration, show status, provide setup flows, and support backup/restore.

## 4. Main Goals

The application must provide:

1. A complete Go backend with Cobra CLI.
2. A Vue 3 + TypeScript + Vite frontend.
3. SQLite-based local state.
4. Local camera discovery as far as possible.
5. Device fingerprinting and confidence-based matching.
6. Manual camera-to-slot assignment.
7. go2rtc config generation.
8. AgentDVR integration through stable stream aliases.
9. Docker Compose setup.
10. Linux Mint install scripts.
11. systemd service files.
12. Desktop launcher files.
13. Browser/kiosk launch script.
14. Backup/restore functionality.
15. Clear documentation.
16. Secret-safe logging, UI, and configuration handling.

## 5. High-Level Architecture

```text
Tapo cameras with changing DHCP IPs
  -> camera-appliance discovers devices
  -> camera-appliance resolves stable device identities
  -> camera-appliance stores bindings from physical devices to logical slots
  -> camera-appliance generates go2rtc config
  -> go2rtc exposes stable local stream aliases
  -> AgentDVR uses only stable go2rtc stream aliases
  -> AgentDVR layout stays static
```

AgentDVR must not depend on changing camera IP addresses.

Instead, AgentDVR should always consume stable streams:

```text
rtsp://go2rtc:8554/cam1
rtsp://go2rtc:8554/cam2
rtsp://go2rtc:8554/cam3
rtsp://go2rtc:8554/cam4
rtsp://go2rtc:8554/cam5
```

The real camera IPs are resolved by `camera-appliance` and written into the generated go2rtc config.

## 6. Core Concept

Do not model cameras like this:

```text
cam1 = 192.168.178.42
```

Model cameras like this:

```text
cam1 = physical device identity
current IP = resolved runtime property
```

A physical device identity can include:

* MAC address
* ONVIF endpoint reference
* serial number
* manufacturer
* model
* hardware ID
* hostname
* last known IP

The IP address must never be the primary long-term identity.

## 7. Main Components

### 7.1 AgentDVR

Role:

* Static camera viewer.
* Shows the final camera layout.
* Uses only stable go2rtc URLs.
* Does not manage discovery.
* Does not manage dynamic IP changes.

Initial AgentDVR setup may be manual for MVP, but the repository should support storing notes/templates and operational docs for it.

Expected static streams:

```text
cam1 -> rtsp://go2rtc:8554/cam1
cam2 -> rtsp://go2rtc:8554/cam2
cam3 -> rtsp://go2rtc:8554/cam3
cam4 -> rtsp://go2rtc:8554/cam4
cam5 -> rtsp://go2rtc:8554/cam5
```

### 7.2 go2rtc

Role:

* Stable stream alias layer.
* Receives generated config from `camera-appliance`.
* Exposes predictable stream names.
* Hides changing camera IPs from AgentDVR.

Example generated config:

```yaml
streams:
  cam1:
    - rtsp://tapo_user_1:camera-password@192.168.178.42:554/stream2

  cam2:
    - rtsp://tapo_user_2:camera-password@192.168.178.43:554/stream2
```

Generated configs can contain secrets and must never be committed.

### 7.3 camera-appliance Go Backend

Role:

* CLI application.
* Local HTTP API server.
* Serves the built Vue frontend.
* Owns local state.
* Discovers cameras.
* Stores device fingerprints.
* Matches discovered devices to existing bindings.
* Lets the user assign devices to slots.
* Generates go2rtc config.
* Restarts go2rtc or stack where necessary.
* Provides status, logs, backup, and restore.

### 7.4 Vue Admin UI

Role:

* Local setup and admin interface.
* Runs through the Go backend.
* Should be accessible at:

```text
http://127.0.0.1:8091
```

Default bind address must be localhost.

The UI must be German-language because the target user/customer is German-speaking.

### 7.5 Docker Compose

Role:

* Runs AgentDVR.
* Runs go2rtc.
* Runs camera-appliance if desired.
* Supports local reproducible deployment.

### 7.6 systemd

Role:

* Starts the camera stack automatically after boot.
* Optionally starts the kiosk/browser launcher.

### 7.7 Desktop Launchers

Role:

Provide simple customer/admin actions:

* Open cameras
* Search cameras
* Restart camera server
* Show status

## 8. Functional Requirements

### 8.1 Startup

On boot:

1. Docker stack starts.
2. camera-appliance starts.
3. camera-appliance optionally runs startup discovery.
4. Known cameras are matched by fingerprint.
5. Current camera IPs are resolved.
6. go2rtc config is generated.
7. go2rtc is restarted or reloaded.
8. AgentDVR becomes available.
9. Browser can open AgentDVR in fullscreen/kiosk style.

### 8.2 Discovery

The application must implement local discovery as far as possible:

* Detect local IPv4 subnets.
* Try ONVIF/WS-Discovery if practical.
* Scan likely camera hosts.
* Test RTSP port 554.
* Test Tapo-style RTSP paths:

  * `/stream1`
  * `/stream2`
* Prefer `stream2` for live display.
* Enrich devices with MAC addresses from ARP/neighbor table where possible.
* Store raw discovery data for debugging/future improvements.
* Work gracefully when no cameras are found.

### 8.3 Assignment

The admin UI must allow assigning a discovered physical camera to a logical slot:

* cam1
* cam2
* cam3
* cam4
* cam5

The user should be able to:

* See discovered devices.
* See device information.
* Preview or test a stream if possible.
* Assign a device to a slot.
* Set a display label.
* Set or choose the camera username.
* Choose stream1 or stream2, with stream2 as default.
* Replace an existing assignment.
* Remove an assignment.

### 8.4 Matching

When a camera IP changes, the system must rediscover the camera and keep the same slot binding.

Matching must use a confidence score.

Suggested score:

```text
serial number match: 80
MAC address match: 70
ONVIF endpoint reference match: 60
manufacturer + model match: 20
hostname match: 10
last known IP match: 5
successful username/password for existing binding: 20
```

Decision:

```text
score >= 80:
  auto-match

score >= 40 and score < 80:
  show suggested match in UI

score < 40:
  treat as unknown device
```

If there is a conflict, do not auto-assign. Show the conflict in the UI.

### 8.5 go2rtc Config Generation

The application must generate go2rtc configuration from active bindings.

Input:

* slots
* bindings
* discovered current IPs
* camera usernames
* camera password from local secrets
* selected stream name

Output:

```text
/var/lib/camera-appliance/generated/go2rtc.yaml
```

This file may contain secrets and must not be committed.

The app must redact credentials in logs, CLI output, and UI.

### 8.6 Status

Status should show:

* AgentDVR online/offline
* go2rtc online/offline
* camera-appliance online
* last discovery time
* slot status
* camera status
* current IP
* last seen
* conflicts
* stream check results

### 8.7 Backup and Restore

The app must support backup/restore for local runtime configuration.

Backup should include:

* SQLite state
* local settings
* camera bindings
* slot label overrides
* generated config if useful
* optional AgentDVR config backup location metadata

Backup should not include by default:

* Docker images
* Git repo
* logs
* recordings/media

If encryption is feasible, implement age-based encryption or prepare structure for it. If encryption is not implemented in the first pass, make the backup system modular and clearly document that backups may contain sensitive data.

## 9. Non-Goals for MVP

Do not implement:

* Full network segmentation.
* Router configuration.
* DHCP reservation management.
* Cloud control.
* Native mobile app.
* Own video recording.
* Replacement for AgentDVR.
* Complex multi-user permission system.
* Fully automatic AgentDVR layout creation if that is too fragile.

AgentDVR can be manually configured initially, as long as the app provides the stable stream aliases and documentation.

## 10. Security Requirements

### 10.1 Secrets

Never commit real credentials.

Never log passwords.

Never display unredacted RTSP URLs.

Store local secrets outside Git:

```text
/etc/camera-appliance/secrets.env
```

Runtime state:

```text
/var/lib/camera-appliance
```

Generated config:

```text
/var/lib/camera-appliance/generated/go2rtc.yaml
```

### 10.2 Redaction

Any credential-containing URL must be redacted before output.

Input:

```text
rtsp://user:secret@192.168.178.42:554/stream2
```

Output:

```text
rtsp://user:******@192.168.178.42:554/stream2
```

### 10.3 Local UI

Admin UI must bind to localhost by default:

```text
127.0.0.1:8091
```

Do not expose the admin UI to the LAN by default.

### 10.4 GitHub

The target laptop should not require a personal GitHub PAT.

Preferred deployment model:

* GitHub repo with code only.
* Read-only deploy key on target machine if needed.
* Runtime secrets local only.
* Pushes happen from the developer machine.

## 11. Repository Structure

Create this structure:

```text
camera-appliance/
  README.md
  codex-handoff.md
  compose.yaml
  .env.example
  .gitignore

  camera-manager/
    go.mod
    go.sum
    cmd/
      camera-appliance/
        main.go
    internal/
      app/
      backup/
      cli/
      config/
      discovery/
      fingerprint/
      go2rtc/
      logging/
      matcher/
      redaction/
      state/
      system/
      web/
        api/
        static/
    migrations/

  frontend/
    package.json
    package-lock.json
    index.html
    vite.config.ts
    tsconfig.json
    src/
      main.ts
      App.vue
      router/
      api/
      components/
      layouts/
      pages/
      styles/
      types/

  config/
    slots.yaml
    defaults.yaml

  templates/
    go2rtc.yaml.tmpl
    agentdvr/
      README.md

  systemd/
    camera-appliance.service
    camera-kiosk.service

  desktop/
    open-cameras.desktop
    rediscover-cameras.desktop
    restart-cameras.desktop
    status.desktop

  bin/
    open-cameras
    rediscover-cameras
    restart-cameras
    status
    install
    backup
    restore

  docs/
    setup.md
    recovery.md
    operations.md
    customer-instructions.md
    development.md
```

## 12. Local Paths on Target System

Use these paths:

```text
/opt/camera-appliance
  Git checkout and application code

/etc/camera-appliance
  secrets.env
  local.env

/var/lib/camera-appliance
  state.db
  generated/go2rtc.yaml
  backups/
  logs/
  agentdvr/

/var/log/camera-appliance
  optional host logs
```

## 13. `.gitignore`

Add a `.gitignore` that excludes:

```gitignore
# Local secrets
.env
*.env
secrets/
local.env

# Generated files
generated/
*.generated.yaml
go2rtc.yaml

# Runtime state
data/
state.db
*.sqlite
*.sqlite3

# AgentDVR data and backups
agentdvr-data/
*.backup
*.zip

# Customer-specific files
customer/
local/

# Build outputs
dist/
frontend/dist/
camera-appliance
bin/camera-appliance

# Node
node_modules/

# Go
coverage.out

# OS/editor
.DS_Store
.idea/
.vscode/
```

## 14. Default Slot Config

Create `config/slots.yaml`:

```yaml
slots:
  - id: cam1
    label: "Kamera 1"
    role: "grid"
    default_stream: "stream2"
    required: true
    sort_order: 1

  - id: cam2
    label: "Kamera 2"
    role: "grid"
    default_stream: "stream2"
    required: true
    sort_order: 2

  - id: cam3
    label: "Kamera 3"
    role: "grid"
    default_stream: "stream2"
    required: true
    sort_order: 3

  - id: cam4
    label: "Kamera 4"
    role: "grid"
    default_stream: "stream2"
    required: true
    sort_order: 4

  - id: cam5
    label: "Große Ansicht"
    role: "large"
    default_stream: "stream2"
    required: false
    sort_order: 5
```

## 15. Local Secrets Example

Create `.env.example`:

```env
CAMERA_APPLIANCE_BIND_ADDR=127.0.0.1:8091
CAMERA_APPLIANCE_CONFIG_DIR=/etc/camera-appliance
CAMERA_APPLIANCE_STATE_DIR=/var/lib/camera-appliance
CAMERA_APPLIANCE_AGENTDVR_URL=http://localhost:8090
CAMERA_APPLIANCE_GO2RTC_URL=http://localhost:1984
CAMERA_APPLIANCE_GO2RTC_RTSP_URL=rtsp://localhost:8554

# Do not commit real values.
TAPO_CAMERA_PASSWORD=change-me
ADMIN_SESSION_SECRET=change-me
```

Target file:

```text
/etc/camera-appliance/secrets.env
```

## 16. SQLite Data Model

Implement migrations for the following tables.

### 16.1 `slots`

```sql
CREATE TABLE IF NOT EXISTS slots (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  role TEXT NOT NULL,
  default_stream TEXT NOT NULL,
  required INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

### 16.2 `devices`

```sql
CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  last_ip TEXT,
  mac_address TEXT,
  onvif_endpoint_ref TEXT,
  serial_number TEXT,
  manufacturer TEXT,
  model TEXT,
  hardware_id TEXT,
  hostname TEXT,
  raw_json TEXT
);
```

### 16.3 `bindings`

```sql
CREATE TABLE IF NOT EXISTS bindings (
  slot_id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  label TEXT,
  username TEXT,
  stream_name TEXT NOT NULL DEFAULT 'stream2',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(slot_id) REFERENCES slots(id),
  FOREIGN KEY(device_id) REFERENCES devices(id)
);
```

### 16.4 `scan_runs`

```sql
CREATE TABLE IF NOT EXISTS scan_runs (
  id TEXT PRIMARY KEY,
  started_at TEXT NOT NULL,
  finished_at TEXT,
  status TEXT NOT NULL,
  message TEXT
);
```

### 16.5 `stream_checks`

```sql
CREATE TABLE IF NOT EXISTS stream_checks (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  checked_at TEXT NOT NULL,
  stream_name TEXT NOT NULL,
  url_redacted TEXT NOT NULL,
  success INTEGER NOT NULL,
  latency_ms INTEGER,
  message TEXT,
  FOREIGN KEY(device_id) REFERENCES devices(id)
);
```

### 16.6 `events`

```sql
CREATE TABLE IF NOT EXISTS events (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  level TEXT NOT NULL,
  type TEXT NOT NULL,
  message TEXT NOT NULL,
  details_json TEXT
);
```

### 16.7 `settings`

```sql
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

## 17. Device Fingerprint

Implement a normalized device fingerprint.

Internal shape:

```json
{
  "mac_address": "AA:BB:CC:DD:EE:FF",
  "onvif_endpoint_ref": "urn:uuid:example",
  "serial_number": "123456789",
  "manufacturer": "TP-Link",
  "model": "Tapo C320WS",
  "hardware_id": "1.0",
  "hostname": "Tapo_Camera",
  "last_ip": "192.168.178.42"
}
```

Normalization rules:

* MAC addresses uppercase with `:`.
* Trim all text fields.
* Empty strings become null/empty optional values.
* IP address is not a primary identity.
* Store raw discovery response for debugging.

Device ID priority:

```text
1. manufacturer + model + serial number
2. ONVIF endpoint reference
3. MAC address
4. generated UUID
```

If deterministic data exists, generate a deterministic hash-based ID.

## 18. Discovery Implementation

Implement discovery in package:

```text
camera-manager/internal/discovery
```

### 18.1 Discovery Sources

Implement as many as practical:

1. Local subnet detection.
2. ONVIF/WS-Discovery.
3. RTSP port scan on port 554.
4. ARP/neighbor table enrichment.
5. RTSP stream checks for `/stream1` and `/stream2`.

### 18.2 Local Network Detection

Detect IPv4 interfaces and subnets.

Ignore:

* loopback
* docker bridge networks where appropriate
* down interfaces

Be careful not to scan huge networks. Limit scan ranges. For MVP, scanning `/24` ranges is acceptable.

### 18.3 ARP/MAC Enrichment

Try to read MAC addresses from:

```text
ip neigh show
/proc/net/arp
```

Before reading ARP, contact the IP via ping, TCP connect, ONVIF, or RTSP so the neighbor table can populate.

### 18.4 RTSP Checks

Try:

```text
rtsp://USERNAME:PASSWORD@IP:554/stream1
rtsp://USERNAME:PASSWORD@IP:554/stream2
```

Prefer `stream2` for live view.

Use short timeouts.

Do not hang discovery for a long time.

Use configured usernames from assignments and/or setup input. If no username exists yet, allow the UI to collect candidate usernames.

### 18.5 ONVIF

If a reliable Go ONVIF/WS-Discovery library can be integrated, implement it.

If ONVIF becomes too fragile, keep the interface and implement RTSP scan first.

Do not block the rest of the MVP on perfect ONVIF support.

### 18.6 Discovery Result

Return normalized results like:

```json
{
  "id": "device-id",
  "ip": "192.168.178.42",
  "mac_address": "AA:BB:CC:DD:EE:01",
  "manufacturer": "TP-Link",
  "model": "Tapo C320WS",
  "serial_number": "123456",
  "onvif_endpoint_ref": "urn:uuid:...",
  "hostname": "camera",
  "stream_checks": {
    "stream1": {
      "tested": true,
      "success": true
    },
    "stream2": {
      "tested": true,
      "success": true
    }
  }
}
```

## 19. Matching Implementation

Package:

```text
camera-manager/internal/matcher
```

Implement:

* score calculation
* auto-match decision
* suggested-match decision
* conflict detection

Score:

```text
serial number match: 80
MAC address match: 70
ONVIF endpoint reference match: 60
manufacturer + model match: 20
hostname match: 10
last known IP match: 5
successful username/password for existing binding: 20
```

Behavior:

```text
score >= 80:
  auto-match

score >= 40:
  suggested match

score < 40:
  unknown device
```

Conflict behavior:

* Two devices match one slot: require manual selection.
* One device matches two slots: require manual selection unless there is one exact serial match.
* Never silently replace an existing binding with a low-confidence match.

## 20. go2rtc Config Rendering

Package:

```text
camera-manager/internal/go2rtc
```

Input:

* slots
* bindings
* latest device IPs
* local password
* selected stream name
* username

Output:

```text
/var/lib/camera-appliance/generated/go2rtc.yaml
```

Example:

```yaml
streams:
  cam1:
    - rtsp://tapo_hof:camera-password@192.168.178.42:554/stream2

  cam2:
    - rtsp://tapo_eingang:camera-password@192.168.178.43:554/stream2
```

Rules:

* Do not render disabled bindings.
* Do not render bindings without known current IP.
* Redact URLs in logs.
* Validate generated YAML.
* Return clear warnings for incomplete slots.
* Provide CLI command `render-go2rtc`.

After rendering:

* Restart go2rtc via Docker if configured.
* If reload is implemented later, use reload instead.

## 21. AgentDVR Layout Concept

Target view:

```text
+-----------------------+-------------------+
| cam1       | cam2     |                   |
|            |          |                   |
+------------+----------+       cam5        |
| cam3       | cam4     |                   |
|            |          |                   |
+------------+----------+-------------------+
```

MVP approach:

* AgentDVR can be configured manually.
* The repository should document how to configure the five static streams.
* An AgentDVR backup may be stored outside Git or encrypted.

AgentDVR should use:

```text
rtsp://go2rtc:8554/cam1
rtsp://go2rtc:8554/cam2
rtsp://go2rtc:8554/cam3
rtsp://go2rtc:8554/cam4
rtsp://go2rtc:8554/cam5
```

Do not make AgentDVR depend on actual camera IPs.

## 22. Docker Compose

Create `compose.yaml`.

Expected services:

* agentdvr
* go2rtc
* camera-manager

Suggested structure:

```yaml
services:
  agentdvr:
    image: mekayelanik/ispyagentdvr:stable
    container_name: agentdvr
    restart: unless-stopped
    ports:
      - "8090:8090"
    volumes:
      - /var/lib/camera-appliance/agentdvr/xml:/AgentDVR/Media/XML
      - /var/lib/camera-appliance/agentdvr/media:/AgentDVR/Media/WebServerRoot/Media
    depends_on:
      - go2rtc

  go2rtc:
    image: alexxit/go2rtc:latest
    container_name: go2rtc
    restart: unless-stopped
    volumes:
      - /var/lib/camera-appliance/generated/go2rtc.yaml:/config/go2rtc.yaml:ro
    ports:
      - "127.0.0.1:1984:1984"
      - "127.0.0.1:8554:8554"

  camera-manager:
    build:
      context: .
      dockerfile: camera-manager/Dockerfile
    container_name: camera-manager
    restart: unless-stopped
    network_mode: host
    environment:
      CAMERA_APPLIANCE_BIND_ADDR: 127.0.0.1:8091
      CAMERA_APPLIANCE_CONFIG_DIR: /etc/camera-appliance
      CAMERA_APPLIANCE_STATE_DIR: /var/lib/camera-appliance
    volumes:
      - /etc/camera-appliance:/etc/camera-appliance:ro
      - /var/lib/camera-appliance:/var/lib/camera-appliance
      - /var/run/docker.sock:/var/run/docker.sock
```

`network_mode: host` for camera-manager is acceptable because discovery is easier with host networking.

## 23. CLI Requirements

Use Cobra.

Binary name:

```text
camera-appliance
```

Commands:

```text
camera-appliance serve
camera-appliance status
camera-appliance discover
camera-appliance assign
camera-appliance render-go2rtc
camera-appliance restart-go2rtc
camera-appliance restart-stack
camera-appliance reset-bindings
camera-appliance backup
camera-appliance restore
```

### 23.1 `serve`

Starts API and serves Vue frontend.

Default:

```text
127.0.0.1:8091
```

### 23.2 `status`

Shows system status:

```text
System
  AgentDVR: online
  go2rtc: online
  camera-appliance: online

Cameras
  cam1 Hof: online at 192.168.178.42
  cam2 Eingang: online at 192.168.178.43
  cam3 Seite: offline, last seen at 192.168.178.44
```

### 23.3 `discover`

Runs discovery once.

Should not crash when no cameras are found.

### 23.4 `assign`

Example:

```text
camera-appliance assign --slot cam1 --device DEVICE_ID --username tapo_hof --label Hof --stream stream2
```

### 23.5 `render-go2rtc`

Generates go2rtc config from current bindings.

### 23.6 `restart-go2rtc`

Restarts go2rtc using Docker Compose or Docker API.

### 23.7 `restart-stack`

Restarts AgentDVR, go2rtc, and camera-manager if possible.

### 23.8 `reset-bindings`

Deletes local camera bindings and discovered devices.

Must not delete:

* code
* secrets
* AgentDVR base config
* system services

### 23.9 `backup`

Creates backup archive.

### 23.10 `restore`

Restores backup archive.

## 24. Backend API

The Go backend must expose JSON APIs for Vue.

Base path:

```text
/api
```

Routes:

```text
GET    /api/status
POST   /api/discovery/start
GET    /api/discovery/runs
GET    /api/discovery/runs/:id
GET    /api/devices
GET    /api/devices/:id
GET    /api/devices/:id/preview
GET    /api/slots
GET    /api/bindings
POST   /api/bindings
DELETE /api/bindings/:slotId
POST   /api/bindings/:slotId/replace
POST   /api/go2rtc/render
POST   /api/go2rtc/restart
POST   /api/system/restart-stack
GET    /api/settings
PUT    /api/settings
GET    /api/events
POST   /api/backup
POST   /api/restore
```

All API responses must avoid unredacted secrets.

## 25. Vue Frontend Requirements

Use:

* Vue 3
* TypeScript
* Vite
* Vue Router
* Plain CSS or a small local component system
* No dependency on internet/CDN at runtime

The Go backend should serve the built frontend from `frontend/dist`.

The UI language must be German.

### 25.1 Frontend Pages

Create pages:

```text
Dashboard
Setup Wizard
Discovery
Device Details
Assign Device
Bindings / Cameras
Settings
Logs / Events
Backup / Restore
```

### 25.2 Components

Create reusable components:

```text
AppLayout
PageHeader
StatusBadge
ActionButton
Card
CameraCard
DeviceCard
SlotSelector
ConfirmDialog
Toast
EmptyState
LoadingState
ErrorMessage
```

### 25.3 UI Styleguide

Design goal:

* Calm
* Clear
* Appliance-like
* Large buttons
* High readability
* No visual clutter
* Suitable for non-technical users

Colors:

```text
Background: #F7F8FA
Surface: #FFFFFF
Surface muted: #F1F3F5
Text primary: #111827
Text secondary: #4B5563
Border: #E5E7EB
Primary: #2563EB
Primary dark: #1D4ED8
Success: #16A34A
Warning: #D97706
Danger: #DC2626
Info: #0891B2
```

Typography:

```css
font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
```

Sizes:

```text
Page title: 28px / 34px / 700
Section title: 20px / 28px / 650
Card title: 16px / 24px / 650
Body: 15px / 22px / 400
Small: 13px / 18px / 400
Button: 15px / 20px / 600
```

Layout:

* Max width: 1180px.
* Desktop padding: 24px.
* Small screen padding: 16px.
* Card border radius: 16px.
* Button height: at least 44px.
* Use text labels, not icons only.
* Status must not rely on color alone.

### 25.4 German UI Labels

Use labels like:

```text
Kameras öffnen
Kameras neu suchen
Server neu starten
Status anzeigen
Einrichtung starten
Gerät zuordnen
Vorschau anzeigen
Zuordnung speichern
Zuordnung entfernen
Gerät ersetzen
Backup erstellen
Backup wiederherstellen
Alles funktioniert
Kamera offline
Kamera antwortet nicht
```

Error messages should be helpful:

Bad:

```text
RTSP timeout
```

Good:

```text
Kamera antwortet nicht. Prüfe Stromversorgung oder WLAN-Verbindung.
```

## 26. Wireframes

### 26.1 Dashboard

```text
+--------------------------------------------------------------+
| Camera Appliance                                      Online |
+--------------------------------------------------------------+
|                                                              |
|  [ Kameras öffnen ] [ Kameras neu suchen ] [ Server neu starten ] |
|                                                              |
+----------------------------+---------------------------------+
| Systemstatus               | Kameras                         |
|                            |                                 |
| AgentDVR      Online       | cam1  Hof        Online         |
| go2rtc        Online       | cam2  Eingang    Online         |
| Manager       Online       | cam3  Seite      Offline        |
| Letzte Suche  vor 2 Min.   | cam4  Lager      Online         |
|                            | cam5  Groß       Online         |
+----------------------------+---------------------------------+
|                                                              |
| Letzte Ereignisse                                             |
| - cam3 stream check failed                                    |
| - cam1 IP changed from 192.168.178.42 to 192.168.178.87       |
| - go2rtc config updated                                       |
|                                                              |
+--------------------------------------------------------------+
```

### 26.2 Setup Start

```text
+--------------------------------------------------------------+
| Einrichtung                                                  |
+--------------------------------------------------------------+
|                                                              |
| Dieser Assistent sucht Kameras im lokalen Netzwerk und        |
| ordnet sie festen Anzeigeplätzen zu.                         |
|                                                              |
| Aktueller Stand:                                              |
|  - 5 Plätze konfiguriert                                      |
|  - 3 Kameras zugeordnet                                       |
|  - 2 Kameras fehlen                                           |
|                                                              |
| [ Suche starten ]                                             |
|                                                              |
+--------------------------------------------------------------+
```

### 26.3 Discovery Running

```text
+--------------------------------------------------------------+
| Kameras suchen                                               |
+--------------------------------------------------------------+
|                                                              |
| Das lokale Netzwerk wird durchsucht...                       |
|                                                              |
| [#####-----------------------]                                |
|                                                              |
| Aktueller Schritt: RTSP stream2 testen                        |
| Gefunden bisher: 4 Geräte                                     |
|                                                              |
+--------------------------------------------------------------+
```

### 26.4 Discovery Results

```text
+--------------------------------------------------------------+
| Gefundene Geräte                                             |
+--------------------------------------------------------------+
|                                                              |
| +----------------------------------------------------------+ |
| | Tapo C320WS                              Online          | |
| | IP: 192.168.178.42                                      | |
| | MAC: AA:BB:CC:DD:EE:01                                  | |
| | RTSP stream2: funktioniert                              | |
| | ONVIF: funktioniert                                     | |
| |                                                          | |
| | [ Vorschau ] [ Zuordnen ] [ Details ]                    | |
| +----------------------------------------------------------+ |
|                                                              |
| +----------------------------------------------------------+ |
| | Unbekanntes RTSP-Gerät                  Warnung          | |
| | IP: 192.168.178.51                                      | |
| | MAC: unbekannt                                           | |
| | RTSP stream2: fehlgeschlagen                             | |
| |                                                          | |
| | [ Details ]                                              | |
| +----------------------------------------------------------+ |
|                                                              |
+--------------------------------------------------------------+
```

### 26.5 Assignment

```text
+--------------------------------------------------------------+
| Gerät zuordnen                                               |
+--------------------------------------------------------------+
|                                                              |
| Gerät                                                        |
| Tapo C320WS                                                  |
| IP: 192.168.178.42                                           |
| MAC: AA:BB:CC:DD:EE:01                                       |
|                                                              |
| Vorschau                                                     |
| +----------------------------+                               |
| |                            |                               |
| |        Live preview         |                               |
| |                            |                               |
| +----------------------------+                               |
|                                                              |
| Zuordnen als                                                 |
| ( ) cam1 - Kamera 1                                           |
| ( ) cam2 - Kamera 2                                           |
| ( ) cam3 - Kamera 3                                           |
| ( ) cam4 - Kamera 4                                           |
| ( ) cam5 - Große Ansicht                                      |
|                                                              |
| Anzeigename                                                  |
| [ Hof ]                                                       |
|                                                              |
| Benutzername                                                 |
| [ tapo_hof ]                                                  |
|                                                              |
| Stream                                                       |
| [ stream2 v ]                                                 |
|                                                              |
| [ Zuordnung speichern ] [ Abbrechen ]                         |
|                                                              |
+--------------------------------------------------------------+
```

### 26.6 Existing Binding

```text
+--------------------------------------------------------------+
| Kamera 3 - Seite                                             |
+--------------------------------------------------------------+
|                                                              |
| Status: Offline                                              |
| Zuletzt gesehen: 2026-05-26 17:30                             |
| Letzte IP: 192.168.178.44                                    |
| Identität:                                                   |
|  - MAC: AA:BB:CC:DD:EE:03                                    |
|  - Modell: Tapo C320WS                                       |
|  - Seriennummer: 123456                                      |
|                                                              |
| Aktionen                                                     |
| [ Neu suchen ] [ Gerät ersetzen ] [ Zuordnung entfernen ]     |
|                                                              |
+--------------------------------------------------------------+
```

### 26.7 Conflict

```text
+--------------------------------------------------------------+
| Zuordnung prüfen                                             |
+--------------------------------------------------------------+
|                                                              |
| Zwei Geräte könnten zu cam2 - Eingang passen.                 |
| Bitte wähle die richtige Kamera.                              |
|                                                              |
| Kandidat A                                                   |
| Tapo C320WS, 192.168.178.42, Score 75                         |
| [ Vorschau ] [ Dieses Gerät verwenden ]                       |
|                                                              |
| Kandidat B                                                   |
| Tapo C320WS, 192.168.178.43, Score 70                         |
| [ Vorschau ] [ Dieses Gerät verwenden ]                       |
|                                                              |
+--------------------------------------------------------------+
```

### 26.8 Settings

```text
+--------------------------------------------------------------+
| Einstellungen                                                |
+--------------------------------------------------------------+
|                                                              |
| Kamera-Passwort                                              |
| [ *************** ] [ Ändern ]                                |
|                                                              |
| Suche                                                        |
| [x] Beim Start automatisch suchen                             |
| [x] go2rtc-Konfiguration nach erfolgreicher Suche erzeugen    |
| [x] go2rtc nach Änderungen neu starten                        |
|                                                              |
| AgentDVR                                                     |
| URL: http://localhost:8090                                    |
|                                                              |
| Admin-Oberfläche                                             |
| Adresse: 127.0.0.1:8091                                      |
|                                                              |
| [ Einstellungen speichern ]                                  |
|                                                              |
+--------------------------------------------------------------+
```

### 26.9 Backup

```text
+--------------------------------------------------------------+
| Backup                                                       |
+--------------------------------------------------------------+
|                                                              |
| Erstelle ein Backup der lokalen Appliance-Konfiguration.      |
|                                                              |
| Enthalten:                                                   |
|  - Kamera-Zuordnungen                                        |
|  - Anzeigenamen                                              |
|  - lokale Einstellungen                                      |
|  - optional AgentDVR-Konfiguration                            |
|                                                              |
| Nicht enthalten:                                             |
|  - Docker Images                                             |
|  - Git Repository                                            |
|  - Videoaufzeichnungen                                       |
|                                                              |
| [ Backup erstellen ] [ Backup wiederherstellen ]             |
|                                                              |
+--------------------------------------------------------------+
```

## 27. Install Script

Create:

```text
bin/install
```

The install script should support Linux Mint-like systems.

It should:

* Check for required tools.
* Install or verify Docker and Docker Compose if practical.
* Build Go backend.
* Build Vue frontend.
* Create required directories.
* Copy or install systemd service files.
* Copy desktop launcher files.
* Create example `/etc/camera-appliance/secrets.env` if missing.
* Start or enable services if requested.

Do not overwrite existing secrets.

Use safe prompts or flags.

Suggested flags:

```text
bin/install --user customer
bin/install --enable-systemd
bin/install --install-desktop-launchers
```

## 28. Desktop Launchers

Create:

```text
desktop/open-cameras.desktop
desktop/rediscover-cameras.desktop
desktop/restart-cameras.desktop
desktop/status.desktop
```

### 28.1 Open Cameras

```desktop
[Desktop Entry]
Type=Application
Name=Kameras öffnen
Exec=/opt/camera-appliance/bin/open-cameras
Icon=camera-video
Terminal=false
Categories=Utility;
```

### 28.2 Rediscover Cameras

```desktop
[Desktop Entry]
Type=Application
Name=Kameras neu suchen
Exec=/opt/camera-appliance/bin/rediscover-cameras
Icon=system-search
Terminal=false
Categories=Utility;
```

### 28.3 Restart Cameras

```desktop
[Desktop Entry]
Type=Application
Name=Kamera-Server neu starten
Exec=/opt/camera-appliance/bin/restart-cameras
Icon=view-refresh
Terminal=false
Categories=Utility;
```

### 28.4 Status

```desktop
[Desktop Entry]
Type=Application
Name=Kamera-Status anzeigen
Exec=/opt/camera-appliance/bin/status
Icon=dialog-information
Terminal=true
Categories=Utility;
```

## 29. Helper Scripts

### 29.1 `bin/open-cameras`

```bash
#!/usr/bin/env bash
set -euo pipefail

URL="${CAMERA_APPLIANCE_AGENTDVR_URL:-http://localhost:8090/}"
PROFILE_DIR="$HOME/.config/camera-kiosk-browser"

if command -v chromium >/dev/null 2>&1; then
  BROWSER="chromium"
elif command -v chromium-browser >/dev/null 2>&1; then
  BROWSER="chromium-browser"
elif command -v google-chrome >/dev/null 2>&1; then
  BROWSER="google-chrome"
else
  xdg-open "$URL"
  exit 0
fi

exec "$BROWSER" \
  --user-data-dir="$PROFILE_DIR" \
  --start-fullscreen \
  --no-first-run \
  --disable-session-crashed-bubble \
  "$URL"
```

### 29.2 `bin/restart-cameras`

```bash
#!/usr/bin/env bash
set -euo pipefail

cd /opt/camera-appliance
/usr/bin/docker compose restart agentdvr go2rtc camera-manager
```

### 29.3 `bin/rediscover-cameras`

```bash
#!/usr/bin/env bash
set -euo pipefail

/opt/camera-appliance/bin/camera-appliance discover
```

### 29.4 `bin/status`

```bash
#!/usr/bin/env bash
set -euo pipefail

/opt/camera-appliance/bin/camera-appliance status
```

## 30. systemd

### 30.1 `systemd/camera-appliance.service`

```ini
[Unit]
Description=Camera Appliance Docker Stack
After=docker.service network-online.target
Requires=docker.service
Wants=network-online.target

[Service]
Type=oneshot
WorkingDirectory=/opt/camera-appliance
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
RemainAfterExit=yes
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

### 30.2 `systemd/camera-kiosk.service`

```ini
[Unit]
Description=Camera Kiosk Browser
After=graphical-session.target camera-appliance.service
Wants=camera-appliance.service

[Service]
Type=simple
ExecStart=/opt/camera-appliance/bin/open-cameras
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

Kiosk service may need adaptation for Linux Mint desktop session behavior. If uncertain, also provide an XDG autostart option.

## 31. Logging

Implement structured logging.

Events to log:

```text
scan.started
scan.finished
device.discovered
device.updated
binding.created
binding.updated
binding.removed
binding.matched
binding.conflict
go2rtc.rendered
go2rtc.restarted
stream.check.failed
stream.check.succeeded
backup.created
restore.completed
```

Never log unredacted secrets.

Persist recent events in SQLite for the UI.

## 32. Backup and Restore

Package:

```text
camera-manager/internal/backup
```

Commands:

```text
camera-appliance backup --out FILE
camera-appliance restore --in FILE
```

Backup should include:

```text
/var/lib/camera-appliance/state.db
/var/lib/camera-appliance/generated/go2rtc.yaml
/etc/camera-appliance/local.env
optional metadata
```

Secrets:

* Either exclude by default and warn the user.
* Or include only if explicitly requested.
* If included, document that the backup is sensitive.
* Prefer encrypted backups if possible.

## 33. Development Requirements

### 33.1 Go

Use:

* Cobra for CLI.
* SQLite driver.
* Context-aware functions.
* Clean internal package boundaries.
* No global mutable state unless unavoidable.
* Tests for important logic.

Suggested packages:

```text
internal/app
internal/backup
internal/cli
internal/config
internal/discovery
internal/fingerprint
internal/go2rtc
internal/logging
internal/matcher
internal/redaction
internal/state
internal/system
internal/web/api
```

### 33.2 Frontend

Use:

* Vue 3
* TypeScript
* Vite
* Vue Router
* Local API client wrapper
* No CDN runtime dependencies

Suggested structure:

```text
frontend/src/
  main.ts
  App.vue
  router/
  api/
  components/
  layouts/
  pages/
  styles/
  types/
```

### 33.3 Tests

Implement tests for:

* Redaction.
* Fingerprint normalization.
* Matching scores.
* Conflict detection.
* go2rtc config rendering.
* State persistence.
* Settings loading.
* Basic API handlers where practical.

Acceptance command:

```text
go test ./...
```

Frontend should build:

```text
npm run build
```

## 34. Documentation

Create docs:

```text
docs/setup.md
docs/recovery.md
docs/operations.md
docs/customer-instructions.md
docs/development.md
```

### 34.1 README

README should explain:

* What the project does.
* Architecture.
* Local development.
* Build steps.
* Install steps.
* How secrets are handled.
* How to run discovery.
* How to assign cameras.
* How to generate go2rtc config.
* How to open AgentDVR.
* How to recover a machine.

### 34.2 Customer Instructions

Write simple German instructions:

```text
1. Laptop einschalten.
2. Wenn die Kameras nicht erscheinen, auf „Kameras öffnen“ klicken.
3. Wenn eine Kamera fehlt, auf „Kameras neu suchen“ klicken.
4. Wenn das nicht hilft, auf „Kamera-Server neu starten“ klicken.
5. Wenn das nicht hilft, Laptop neu starten.
6. Wenn eine Kamera weiterhin fehlt, Stromversorgung der Kamera prüfen.
```

## 35. MVP Acceptance Criteria

The implementation is acceptable when:

```text
1. The Go app builds.
2. The Vue frontend builds.
3. The Go backend serves the built Vue frontend.
4. `camera-appliance status` works.
5. `camera-appliance serve` starts a local dashboard on 127.0.0.1:8091.
6. `camera-appliance discover` runs without crashing even when no cameras are found.
7. Discovery can detect local subnets.
8. Discovery can scan RTSP port 554.
9. Discovery can attempt Tapo stream1/stream2 checks when credentials are configured.
10. Discovered devices are stored in SQLite.
11. Devices have normalized fingerprints.
12. Bindings can be created through CLI and UI.
13. Bindings survive restart.
14. Matching logic is implemented and tested.
15. go2rtc config can be generated from bindings.
16. Secret redaction is implemented and tested.
17. Docker Compose config is valid.
18. Basic install script exists.
19. systemd files exist.
20. desktop launcher files exist.
21. backup and restore commands exist.
22. README and docs exist.
23. Secrets are not committed.
24. `go test ./...` passes.
25. The project can be tested with real cameras later without redesign.
```

## 36. Suggested Implementation Order

Implement in this order:

### Phase 1 - Foundation

* Repository structure.
* Go module.
* Cobra CLI.
* Config loading.
* SQLite initialization.
* Slot loading from YAML.
* Basic API server.
* Vue project.
* Dashboard skeleton.

### Phase 2 - State and UI

* Slots API.
* Devices API.
* Bindings API.
* Events API.
* Vue dashboard.
* Vue bindings page.
* Vue settings page.

### Phase 3 - Discovery

* Local subnet detection.
* RTSP port scanner.
* ARP/neighbor enrichment.
* Stream checks.
* Store discovered devices.
* Discovery UI.

### Phase 4 - Assignment and Matching

* Assignment API.
* Assignment UI.
* Fingerprint normalization.
* Matching score.
* Conflict handling.

### Phase 5 - go2rtc

* Config renderer.
* Redaction.
* CLI command.
* API endpoint.
* Restart command.
* UI action.

### Phase 6 - System Integration

* Docker Compose.
* systemd.
* Desktop launchers.
* Kiosk script.
* Install script.

### Phase 7 - Backup and Docs

* Backup command.
* Restore command.
* Documentation.
* Customer instructions.
* Final cleanup.

## 37. Important Implementation Notes

### 37.1 Do not overfit to one network

The application will be tested in one network and later moved to a customer network. The state reset flow must support this.

Implement:

```text
camera-appliance reset-bindings
```

This removes discovered devices and bindings but does not remove code, secrets, or base system configuration.

### 37.2 Keep lab and customer state separate

The developer may test with cameras at home, then reset bindings before customer deployment.

### 37.3 Do not block on perfect ONVIF

ONVIF is useful, but RTSP scanning and ARP enrichment are enough for a useful MVP.

### 37.4 Do not put passwords into generated frontend files

Frontend must fetch runtime state from backend APIs. Do not build secrets into Vue.

### 37.5 Admin UI is local

The UI is for local setup and admin usage. It does not need internet access.

## 38. Final Codex Goal Summary

Build the complete local MVP for `camera-appliance`.

Use Go, Cobra, SQLite, Vue 3, TypeScript, and Vite.

AgentDVR is the static viewer. go2rtc provides stable stream aliases. camera-appliance discovers cameras, stores physical device identities, maps devices to fixed logical slots, generates go2rtc config, exposes a local Vue setup UI, and provides CLI/admin operations.

Implement the app, Docker Compose, install scripts, systemd services, desktop launchers, backup/restore, docs, redaction, matching, and local discovery.

Do not commit secrets. Do not bind identity to IP addresses. Do not stop at scaffolding.

```