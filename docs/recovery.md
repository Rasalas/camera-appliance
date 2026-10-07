# Recovery

## Camera Missing

1. Check camera power.
2. Open the admin UI at `http://127.0.0.1:8091`.
3. Run **Kameras neu suchen**.
4. If the camera is found with a new IP, keep the existing slot binding.
5. Render go2rtc config and restart go2rtc.

## Restore Backup

```bash
camera-appliance restore --in /var/lib/camera-appliance/backups/FILE.tar.gz
camera-appliance restart-stack
```

Backups may contain generated RTSP URLs and should be stored securely.

Database backups include committed WAL changes and can be created while the
manager is running. Restore validates the archive before changing files and
uses SQLite to replace database contents safely for open connections. Restart
the stack afterwards to reload restored credentials and stream configuration.
An incomplete or corrupt database backup is rejected.

Discovery preserves the stored device ID when MAC, ONVIF endpoint or qualified
serial identity matches. If several stored devices match, discovery reports a
conflict and keeps their bindings unchanged. An IP address alone is insufficient
to identify a physical camera.

## Update supervision and rollback

### Older Docker installations cannot identify the manager image

The customer installation on 2026-10-07 confirmed another cause of this message:
the Bookworm runtime shipped Docker 20.10.24 with API 1.41, while the host daemon
required at least API 1.44. A container rename cannot fix that incompatibility.
The runtime now copies a pinned Docker CLI, Compose and Buildx from Docker's
official CLI image instead of installing Bookworm's Docker/Compose packages.
Both the `docker compose` plugin and the `docker-compose` helper entrypoint are
available. API versions are negotiated normally; the host daemon is not changed.

For an old installation whose update button cannot start, download
`repair-update.sh` from the latest release assets in the laptop's browser and run:

```bash
sudo bash ~/Downloads/repair-update.sh
```

The repair invokes the downloaded current release CLI on the host, bypassing the old
container's client. It uses the regular update path with backup, rollback and
version healthcheck. It requires an existing installation and does not alter
kiosk/desktop setup. Reload the browser after success; later updates use the
fixed container client through the normal button.

If the update reports `current container image could not be determined`, the
manager may be using the laptop hostname under host networking. Older versions
only pass this hostname to `docker inspect`. Current code also checks the fixed
`camera-manager` container name from `compose.yaml`, for both stack restart and
independent update-worker launch.

Before attempting a repair, inspect the actual Docker error. The old manager
hides it behind the image-discovery message. This command reads the client/server
versions and repeats the old lookup inside the running manager:

```bash
sudo docker exec "$(sudo docker ps --filter label=com.docker.compose.service=camera-manager --format '{{.ID}}')" sh -c 'docker version; docker inspect --format "{{.Image}}" "$(hostname)"'
```

If Docker works and only reports that the hostname is not a container, a one-time
rename can unblock that lookup before the fix is installed:

```bash
sudo docker rename camera-manager "$(sudo docker exec camera-manager hostname)"
```

Then retry the update button. This changes only the running container's name.
Compose identifies the existing service by its labels and restores the configured
name when recreating it. Do not run the rename again after it succeeds. If Docker
reports a socket/permission error or a name conflict, leave the containers in
place and inspect that error before continuing. Renaming cannot repair an
unreachable socket, an incompatible Docker client or a timeout. Current code
includes the underlying inspection errors in the update failure.

Alternatively, the bootstrap installer runs the downloaded release CLI on the
host, bypassing the old manager's container detection:

```bash
curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh | sudo bash
```

### Frontend build fails after an update or rollback

Versions before 0.5.4 overlay source files during update and rollback. An upgrade
from 0.1.9 leaves the removed `MaintenancePage.vue` referring to APIs no longer
present. A failed update then overlays the old snapshot without removing new
pages, producing errors about missing `supportReport`, `downloadSupportBundle`,
`health` and `settingsLoaded`.

The updater now replaces release-owned frontend source, public assets, tests,
build output and Go sources before copying either a release or a rollback
snapshot. Runtime state and customer configuration stay outside these trees.
The Docker frontend stage also includes the shared stamp font and project
license imported by the UI. CI builds the actual Docker frontend stage and
builds the frontend after upgrading from 0.1.9 and after restoring its snapshot.

Run the same downloaded `repair-update.sh` again after 0.5.4 is published. It
always downloads the latest release and uses that release's updater, so it can
repair the source mixture without manual file removal.

### Independent update worker

API installations and regular `camera-appliance update` / `update rollback`
commands hand execution to an independent supervisor. Docker uses a separate
container with host networking and shared installation, configuration and state
volumes. Native systemd deployments use `systemd-run --user`; the service account
needs a working user service manager, as it does for the appliance units.

The supervisor waits for stack recreation and verifies the running manager's
version and commit against the release manifest, then checks go2rtc and the viewer.
A healthy old manager does not count as a successful update. Failure triggers
file rollback unless `--no-auto-rollback` was specified. Recovery has a separate
timeout so it can run after the update deadline expires.

```bash
camera-appliance update status
camera-appliance update rollback
```

Status and results persist under the state directory across manager restarts.
The admin update UI and status command report interrupted workers as failed.
A file lock excludes concurrent API, CLI and worker operations. A queued job has
a one-minute launch window; if the worker never starts, it can be retried after
that window. After an interrupted installation, inspect the result and use the
rollback command before retrying. Host reboot or termination of the supervisor
itself requires this recovery step; only manager restarts are handled automatically.

`--no-restart` performs the file operation synchronously without a network
healthcheck. A manual restart is then required. Release archives used for normal
updates must include version and commit metadata. Rollback restores installation
files; restoring runtime data is a separate explicit backup restore operation.
