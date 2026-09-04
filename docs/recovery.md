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
