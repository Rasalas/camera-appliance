# Operations

## Daily Use

- `bin/open-cameras` opens the local camera-appliance viewer fullscreen.
- `bin/rediscover-cameras` searches for cameras.
- `bin/restart-cameras` restarts `camera-appliance.service` when systemd is installed, then falls back to the built-in Docker Compose restart path.
- `bin/status` prints local service status, systemd status, Docker Compose status, camera assignments, and active stream paths.

The helper scripts resolve the appliance root automatically. Override with `CAMERA_APPLIANCE_HOME=/opt/camera-appliance` or `CAMERA_APPLIANCE_BIN=/path/to/camera-appliance` when testing from another checkout.

## Boot Recovery

The production install path is:

```bash
sudo bin/install --user customer --enable-systemd --enable-kiosk --install-desktop-launchers
```

This installs:

- `camera-appliance.service`: starts the Docker Compose stack after Docker and network-online.
- `camera-kiosk.service`: user service that waits for `http://127.0.0.1:8091/api/status` before opening the viewer.
- Desktop launchers for manual open, status, rediscovery, and restart.

Useful recovery commands:

```bash
sudo systemctl status camera-appliance.service
sudo systemctl restart camera-appliance.service
bin/status
bin/restart-cameras
```

If systemd is not available or the current user lacks service permissions, `bin/restart-cameras` reports that path and uses `camera-appliance restart-stack` instead.

## Updates and Rollback

Build a release archive on the development machine:

```bash
make release VERSION=0.1.0
```

The archive is written to `.release/` and contains the install tree, `bin/camera-appliance`, `frontend/dist`, `compose.yaml`, `systemd/`, and `manifest.json`. The release target excludes `.private`, `.git`, `data`, `node_modules`, `.env`, `local.env`, and `secrets.env`.

Apply a local archive on the customer appliance:

```bash
sudo /opt/camera-appliance/bin/camera-appliance update --archive /path/to/camera-appliance-0.1.0-COMMIT.tar.gz
```

The update command:

- creates a normal configuration backup first,
- snapshots the current installed files under `/var/lib/camera-appliance/backups/rollback-*`,
- copies the release into `/opt/camera-appliance`,
- runs `docker compose up -d --build --remove-orphans`,
- checks the manager API, go2rtc API, and Viewer API,
- automatically restores the previous files if the healthcheck fails.

Apply from a URL:

```bash
sudo /opt/camera-appliance/bin/camera-appliance update --url https://example.invalid/camera-appliance-release.tar.gz
```

Manual rollback uses the last update snapshot:

```bash
sudo /opt/camera-appliance/bin/camera-appliance update rollback
```

For a dry file-copy test without service restart, add `--no-restart`. The healthcheck still runs unless the command is tested through the internal Go test helpers.

## Logs and Events

The manager stores recent operational events in SQLite. The admin UI shows these under **Logs und Ereignisse**.

## RTSP Relays and Path Policies

If the appliance host cannot reach a camera directly, but another host in the same customer network can, define a relay and map affected cameras to relay ports. On every go2rtc render or go2rtc restart, the manager probes the last successful path first, then direct and relay paths according to the camera policy.

Relay settings:

- `camera.relay.ids`
- `camera.relay.<relay_id>.name`
- `camera.relay.<relay_id>.host`
- `camera.relay_endpoint.<device_id>.<relay_id>.host`
- `camera.relay_endpoint.<device_id>.<relay_id>.port`
- `camera.path_policy.<device_id>`

Supported policies:

- `auto` or empty: last successful path, then direct, then relays.
- `prefer_direct`: direct before relays.
- `prefer_relay`: relays before direct.
- `direct_only`: only direct camera IP.
- `relay_only`: only configured relays.

Example for a local SSH relay:

```bash
ssh -fN -L 15541:192.168.178.101:554 -L 15543:192.168.178.190:554 nas
```

Then define relay `nas` with host `host.docker.internal`, set the affected camera relay ports, and regenerate/restart go2rtc. The viewer still shows the camera's discovered IP, but diagnostics and go2rtc use the selected path.

Legacy per-device overrides remain supported:

- `camera.rtsp_endpoint.<device_id>.host`
- `camera.rtsp_endpoint.<device_id>.port`

## Reset Lab State

Before moving from lab to customer network:

```bash
camera-appliance reset-bindings --yes
```

This removes discovered devices and bindings only. It does not remove secrets, code, services, or generated backups.
