# Native NAS deployment

This deployment runs `camera-appliance` and go2rtc without Docker privileges on an x86_64 NAS.

- Install root: `/volume1/docker/camera-appliance`
- LAN UI: `0.0.0.0:8091`
- go2rtc API, RTSP, and WebRTC: loopback only
- State and generated configuration: `/volume1/docker/camera-appliance/data`
- Logs: `/volume1/docker/camera-appliance/data/log`
- LAN alias: `http://cams.local:8091`

The systemd user units supervise the manager and go2rtc independently with automatic restarts. `native-service.sh` remains available as a non-systemd fallback. The manager uses `restart-go2rtc.sh` for UI and watchdog-triggered go2rtc restarts.

Install `camera-appliance.service` as a user unit and enable lingering so it starts at boot even without an interactive login:

```bash
mkdir -p ~/.config/systemd/user
cp /volume1/docker/camera-appliance/deploy/nas/camera-appliance.service ~/.config/systemd/user/
cp /volume1/docker/camera-appliance/deploy/nas/camera-appliance-go2rtc.service ~/.config/systemd/user/
cp /volume1/docker/camera-appliance/deploy/nas/camera-appliance-mdns.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now camera-appliance-go2rtc.service camera-appliance.service camera-appliance-mdns.service
sudo loginctl enable-linger "$USER"
```

The optional `migrate-agentdvr.py` imports up to five cameras from a local AgentDVR `objects.xml` while the manager is bound to loopback. Set the admin password before exposing the manager to the LAN.

## Updates from a development machine

Commit the intended changes, then run from the repository root:

```bash
make update-nas
# Alternative SSH destination and optional version label:
make update-nas NAS_HOST=tbuck@192.168.178.11 VERSION=0.1.10-test.1
```

`NAS_HOST` defaults to the SSH alias `nas`. The local machine needs Git, Go,
Node/npm, Make, Python 3, tar and rsync. The NAS needs Python 3 and a running
systemd user manager with `systemd-run`. SSH key authentication must work without
an interactive prompt, for example through an SSH agent. The SSH user must own
the existing Camera Appliance user service and be able to update its files.

The command builds committed `HEAD` in an empty temporary directory for the
NAS architecture, Linux amd64 or arm64. Tracked uncommitted changes are rejected;
untracked and ignored files are excluded. The default version is `dev-nas-<commit>`.
Local Mac binaries, camera credentials and runtime data never enter the archive.
The existing NAS go2rtc binary is preserved.

SSH carries the archive directly, and the NAS verifies its SHA-256 digest. No
admin password, HTTP download server or insecure-update opt-in is needed.
Installation paths and restart settings come from the existing user service.
The freshly built CLI starts the independent systemd updater, which creates a
SQLite backup and installation snapshot, applies the release, restarts services
and verifies the running version, go2rtc and viewer endpoint. Failed health checks
trigger the updater's automatic rollback. Success is reported only after the
submitted job completes. This also bootstraps NAS installations with an older CLI.

If the NAS user manager still exports the obsolete `GODEBUG=tlskyber=0`, the
helper writes scoped overrides for Camera Appliance and its update units. Other
user services keep their environment. The overrides live in
`~/.config/systemd/user/camera-appliance.service.d/90-nas-go-runtime.conf` and
`~/.config/systemd/user/camera-appliance-update-.service.d/90-nas-go-runtime.conf`.

After a lost SSH connection, check the durable result instead of immediately
starting another update:

```bash
make update-nas-status
```

Once launched, the update worker survives SSH and manager restarts. A lost
connection before dispatch does not imply that an update started. Failed runs
retain their private staging directory and `update.log` beneath `data/updates/`
for diagnosis; successful runs remove staging. Backups and rollback snapshots
remain beneath `data/backups/`. See [recovery](../../docs/recovery.md) before
restoring runtime data separately.

`make serve-update` remains a manual HTTP archive server for other deployments.
It is not used by the native NAS update command.
