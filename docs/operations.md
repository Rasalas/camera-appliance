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
curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh | sudo bash -s -- --user customer --enable-kiosk
```

The bootstrap downloads the latest public release archive and delegates to:

- `camera-appliance install` for a fresh laptop.
- `camera-appliance update` when `/opt/camera-appliance/bin/camera-appliance` already exists.

A direct install from an already downloaded release archive is also supported:

```bash
sudo ./bin/camera-appliance install --archive /path/to/camera-appliance-latest.tar.gz --user customer --enable-systemd --enable-kiosk --install-desktop-launchers
```

Install sets up:

- `camera-appliance.service`: starts the Docker Compose stack after Docker and network-online.
- `camera-kiosk.service`: user service that waits for `http://127.0.0.1:8091/api/status` before opening the viewer.
- Desktop launchers for manual open, status, rediscovery, and restart.
- `/etc/camera-appliance/secrets.env`, only when it does not already exist.
- `/var/lib/camera-appliance/generated/go2rtc.yaml`, only when it does not already exist.

Firewall rules are not changed. UI/API and go2rtc bind to localhost by default; normal kiosk operation needs no incoming ports.

Useful recovery commands:

```bash
sudo systemctl status camera-appliance.service
sudo systemctl restart camera-appliance.service
bin/status
bin/restart-cameras
```

If systemd is not available or the current user lacks service permissions, `bin/restart-cameras` reports that path and uses `camera-appliance restart-stack` instead.

## Watchdog

The manager starts a background watchdog with the API server. It stores its last run, next run, last action, and last error in settings so the System page and support bundle can show them.

Settings:

- `watchdog.enabled`: `true` by default.
- `watchdog.fast_interval_seconds`: go2rtc health interval, default `30`.
- `watchdog.camera_interval_seconds`: camera path interval, default `120`.
- `watchdog.restart_on_change`: restart go2rtc after an automatic path switch, default `true`.
- `watchdog.restart_go2rtc_on_failure`: restart go2rtc when its API is unavailable, default `true`.
- `camera.path.fail_threshold`: failed checks required before leaving the active path, default `2`.
- `camera.path.recovery_threshold`: successful checks required before returning to a preferred non-active path, default `2`.
- `camera.path.restart_cooldown_seconds`: minimum seconds between go2rtc restarts caused by path changes, default `120`.

On camera checks, the watchdog evaluates direct and relay paths with the same path policy used by render and viewer diagnostics. It stores per-camera/path success and failure counters under `camera.path_state.*`. A single timeout does not switch away from the active path. Preferred paths are used again only after the recovery threshold is reached.

A real active-path change updates `camera.active_path.*`, renders go2rtc, restarts go2rtc when enabled and outside the cooldown, and writes a `watchdog.path_switched` event with the switch reason. If a restart is inside the cooldown, the watchdog writes `watchdog.path_restart_cooldown`, marks a pending restart, and performs it after the cooldown with `watchdog.path_restart_after_cooldown`.

## Updates and Rollback

Build a release archive on the development machine:

```bash
make release VERSION=0.1.0
```

The archive is written to `.release/` and contains the install tree, `bin/camera-appliance`, `frontend/dist`, `compose.yaml`, `systemd/`, `install.sh`, and `manifest.json`. The release target also writes `.release/camera-appliance-latest.tar.gz` for the bootstrap URL. It excludes `.private`, `.git`, `data`, `node_modules`, `.env`, `local.env`, and `secrets.env`.

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
- `camera.relay.<relay_id>.type`: currently `ssh_local_forward`.
- `camera.relay.<relay_id>.host`
- `camera.relay.<relay_id>.bind_host`: local SSH bind address, default `127.0.0.1`.
- `camera.relay.<relay_id>.ssh_target`: SSH target such as `nas` or `user@nas`.
- `camera.relay.<relay_id>.auto_start`: `true` lets the watchdog start/restart the relay.
- `camera.relay_endpoint.<device_id>.<relay_id>.host`
- `camera.relay_endpoint.<device_id>.<relay_id>.port`
- `camera.relay_endpoint.<device_id>.<relay_id>.target_host`: optional camera target IP; defaults to the discovered camera IP.
- `camera.relay_endpoint.<device_id>.<relay_id>.target_port`: optional camera target port; defaults to `554`.
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

For managed relays, set `camera.relay.nas.type=ssh_local_forward`, `camera.relay.nas.ssh_target=nas`, and `camera.relay.nas.auto_start=true`. The manager starts:

```bash
ssh -N -o ExitOnForwardFailure=yes -o BatchMode=yes -L 127.0.0.1:15541:192.168.178.101:554 nas
```

Useful commands:

```bash
camera-appliance relays status
camera-appliance relays start nas
camera-appliance relays stop nas
camera-appliance relays restart nas
camera-appliance relays ensure
```

The System page exposes the same actions. Auto-Start is checked by the watchdog and uses a short backoff after failures. SSH passwords are never stored by the app; use SSH keys or an SSH agent for managed relay connections.

Legacy per-device overrides remain supported:

- `camera.rtsp_endpoint.<device_id>.host`
- `camera.rtsp_endpoint.<device_id>.port`

## Reset Lab State

Before moving from lab to customer network:

```bash
camera-appliance reset-bindings --yes
```

This removes discovered devices and bindings only. It does not remove secrets, code, services, or generated backups.
