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
curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh | sudo bash
```

The bootstrap downloads the latest public release archive and delegates to:

- `camera-appliance install` for a fresh laptop.
- `camera-appliance update` when `/opt/camera-appliance/bin/camera-appliance` already exists.

By default it auto-detects the desktop user and enables the kiosk browser service. Use `--user USER` to override detection or `--no-kiosk` to skip the kiosk service.

A direct install from an already downloaded release archive is also supported:

```bash
sudo ./bin/camera-appliance install --archive /path/to/camera-appliance-latest.tar.gz --user customer --enable-systemd --enable-kiosk --install-desktop-launchers
```

Install sets up:

- `camera-appliance.service`: starts the Docker Compose stack after Docker and network-online with the existing images (`up -d`, no `--build`), so boot does not depend on a successful image build. Stopping the unit stops the containers without removing them. Images are built by the bootstrap installer and by every update; if they are missing, Compose builds them once.
- `camera-kiosk.service`: user service that waits for `http://127.0.0.1:8091/api/health` before opening the viewer at `/?kiosk=1`. The browser is restarted 10 seconds after it exits, including a normal close; after 10 starts within 10 minutes the unit gives up until `systemctl --user restart camera-kiosk.service` or the next login. Without Chromium/Chrome it opens the default browser once and cannot detect that browser closing.
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
- `watchdog.restart_go2rtc_on_failure`: restart go2rtc after three consecutive failed health checks, default `true`. Repeated restarts back off from 1 to at most 10 minutes until go2rtc is healthy again (`watchdog.go2rtc.*` counters).
- `camera.path.fail_threshold`: failed checks required before leaving the active path, default `2`.
- `camera.path.recovery_threshold`: successful checks required before returning to a preferred non-active path, default `2`.
- `camera.path.restart_cooldown_seconds`: minimum seconds between go2rtc restarts caused by path changes, default `120`.

On camera checks, the watchdog evaluates direct and relay paths with the same path policy used by render and viewer diagnostics. It stores per-camera/path success and failure counters under `camera.path_state.*`. A single timeout does not switch away from the active path. Preferred paths are used again only after the recovery threshold is reached.

A real active-path change updates `camera.active_path.*`, renders go2rtc, restarts go2rtc when enabled and outside the cooldown, and writes a `watchdog.path_switched` event with the switch reason. If a restart is inside the cooldown, the watchdog writes `watchdog.path_restart_cooldown`, marks a pending restart, and performs it after the cooldown with `watchdog.path_restart_after_cooldown`. A failed render or restart also marks the restart as pending; the watchdog re-renders and retries it with the same backoff.

## Updates and Rollback

Build a release archive on the development machine:

```bash
make release
```

When `VERSION` is not passed explicitly, `make release` asks `tagger bump --dry-run` for the next Conventional Commits semver version and lets you accept it, force patch/minor/major, edit it, or abort. To bypass the prompt, pass a version explicitly:

```bash
make release VERSION=0.1.0
```

`bin/camera-appliance` in the release is always built for `linux/amd64`, also on a Mac; use `make release RELEASE_GOARCH=arm64` for ARM appliances. Install and update refuse an archive whose CLI is built for another OS or CPU.

The archive is written to `.release/` and contains the install tree, `bin/camera-appliance`, `frontend/dist`, `compose.yaml`, `systemd/`, `install.sh`, and `manifest.json`. The release target also writes `.release/camera-appliance-latest.tar.gz` for the bootstrap URL. It excludes `.private`, `.git`, `data`, `node_modules`, `.env`, `local.env`, and `secrets.env`.

Publishing a tag `vX.Y.Z` triggers the GitHub Actions release workflow. The workflow builds the same archive with `VERSION=X.Y.Z` and uploads these assets to the GitHub Release:

- `camera-appliance-X.Y.Z-COMMIT.tar.gz`
- `camera-appliance-latest.tar.gz`
- `repair-update.sh`
- `SHA256SUMS`, listing the SHA-256 of the three files above

The bootstrap installer downloads `camera-appliance-latest.tar.gz` from the latest GitHub Release by default and verifies it against that release's `SHA256SUMS`.

Every download from a URL is verified before it is used: the bootstrap installer, `camera-appliance install|update --url` and the **Update** button load `SHA256SUMS` from the same release directory and fail if the file is missing, the archive is not listed or its digest differs. Releases published before `SHA256SUMS` existed are refused. An operator can still install such a release from the command line by passing the expected digest explicitly with `--digest sha256:<hex>`. Local `--archive` files are checked only when `--digest` is given.

Apply a local archive on the customer appliance:

```bash
sudo /opt/camera-appliance/bin/camera-appliance update --archive /path/to/camera-appliance-0.1.0-COMMIT.tar.gz
```

The update command hands the work to an independent update worker (see `docs/recovery.md`). The worker:

- deletes rollback snapshots beyond the newest three, keeping the one `update rollback` would use,
- creates a normal configuration backup (`camera-appliance-*.tar.gz`, the newest five are kept),
- downloads the archive to the worker's temp directory and extracts it under `/var/lib/camera-appliance/backups`,
- checks free disk space (see below). If it is short, it stops with a German message before any file is changed,
- snapshots the current installed files under `/var/lib/camera-appliance/backups/rollback-*`,
- tags the running manager image as `<repo>:previous`,
- writes the release-owned trees (`frontend/src`, `frontend/public`, `frontend/tests`, `frontend/dist`, `camera-manager`) to `/opt/camera-appliance/.update-new/` and swaps them in by rename. Other release files are replaced one at a time, atomically,
- runs `docker compose build` while the old manager keeps serving its own UI, then swaps in `frontend/dist`,
- runs `docker compose up -d --build --force-recreate --remove-orphans`, which reuses the build cache,
- checks that the manager's version and commit match the release manifest, then checks the go2rtc API and Viewer API,
- on failure, restores the snapshot and starts the `:previous` image with `up -d --no-build`. It rebuilds the old sources only if that image is missing or does not start,
- on success, replaces changed systemd units that are already installed and removes dangling manager images.

An interrupted swap is repaired at the start of the next update or rollback. A tree left only in `.update-old/` is moved back.

### Disk space

Before the snapshot, the worker checks free space on each filesystem. Needs on the same filesystem are added together:

| Need | Filesystem | Size |
| --- | --- | --- |
| Staged release trees | `/opt/camera-appliance` | size of the extracted release |
| Rollback snapshot | `/var/lib/camera-appliance/backups` | size of the current install tree |
| Docker image build | Docker data root (`docker info --format '{{.DockerRootDir}}'`, usually `/var/lib/docker`) | 5 GiB |
| Headroom | every checked filesystem | 256 MB |

On a laptop with a single root partition, all four needs land on one filesystem. An update then needs about 5.6 GB free. The Docker updater container mounts the data root read-only, without its submounts, and uses it only for this check. If the data root cannot be determined, the worker skips the Docker reservation, and `--no-restart` skips it too. Manual `update rollback` needs only the snapshot size plus 32 MB on the install filesystem. It does not need to build.

Measured on 0.5.x with Docker 29 and BuildKit (October 2026):

- The release archive is 13–14 MB and extracts to 25–26 MB. The download (capped at 512 MiB) and the extraction already exist when free space is measured.
- A rebuild after a source-only change adds 642 MB of build cache: 405 MB for the Go module layer, 195 MB for the `go build` layer including its build cache, 33 MB for the binary layer, and a few small copy layers.
- A rebuild after a dependency change also reruns `npm ci` (195 MB) and `go mod download` (585 MB), about 1.4 GB in total.
- A cold build creates 3.4 GB of build cache with the default Docker builder and 4.2 GB with a `docker-container` builder. The manager image is 938 MB unpacked, 253 MB compressed. The Dockerfile names its base images by tag (`golang:1.26-bookworm`, `node:22-bookworm`, `debian:bookworm-slim`), and BuildKit resolves these tags against the registry on every build. If upstream republished a base image since the last update, the next update rebuilds almost from scratch, so the reservation covers a cold build.
- go2rtc is a pinned image (about 270 MB). It is pulled only when a release changes the pin.

The updater keeps at most three rollback snapshots plus the current one, five backups, and the `:previous` image. It does not prune the Docker build cache, because the fallback rebuild uses it offline. BuildKit's garbage collector trims the cache on its own. If an update reports a lack of Docker space, `sudo docker builder prune` frees the cache. Rollback still works afterwards because it uses the `:previous` image. The fallback rebuild then needs network access.

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
