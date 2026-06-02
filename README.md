# camera-appliance

`camera-appliance` turns a Linux Mint laptop into a local camera viewing appliance.

The Go manager discovers cameras, stores stable device identities, binds devices to slots, renders go2rtc config, and serves a local German kiosk viewer/admin UI on `127.0.0.1:8091`. go2rtc exposes stable stream aliases (`cam1` through `cam5`) for the viewer.

## Architecture

```text
Tapo cameras -> camera-appliance discovery/state -> go2rtc stable aliases -> camera-appliance Vue viewer
```

Camera identity is never bound to IP address alone. Devices store MAC, ONVIF endpoint reference, serial number, manufacturer, model, hardware ID, hostname, and last known IP.

## Local Development

```bash
make dev
```

This builds the Vue frontend, builds the Go binary, uses local dev state in `./data`, and serves the admin UI at:

- [http://127.0.0.1:8091](http://127.0.0.1:8091)

For frontend hot reload:

```bash
make dev-hot
```

That starts the Go backend on `127.0.0.1:8091` and Vite on its printed local URL, usually `http://127.0.0.1:5173`.

Useful development commands:

```bash
make test
make build
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

## Optional AgentDVR

AgentDVR is not required for normal install, startup, status, or camera display. It remains available only as an optional Docker Compose profile for NVR experiments:

```bash
sudo docker compose --profile agentdvr up -d agentdvr
```

If used, configure it manually with the same stable go2rtc aliases and never with camera DHCP IPs.

## Linux Mint Install

```bash
sudo bin/install --user customer --enable-systemd --install-desktop-launchers
sudo editor /etc/camera-appliance/secrets.env
cd /opt/camera-appliance
sudo docker compose up -d
```

The install script does not overwrite an existing `/etc/camera-appliance/secrets.env`.
Open `http://127.0.0.1:8091`, discover cameras, assign devices to `cam1` through `cam5`, render go2rtc config, and restart go2rtc.

## Recovery

Use `camera-appliance backup` before customer changes. Use `camera-appliance restore --in FILE` to restore state and generated config, then restart the stack.
