# Setup

1. Install Linux Mint updates.
2. Install Docker, Docker Compose plugin, Go, Node.js/npm, and rsync.
3. Clone or copy the repository.
4. Run `sudo bin/install --user customer --enable-systemd --enable-kiosk --install-desktop-launchers`.
   - `--enable-systemd` installs and starts `camera-appliance.service`.
   - `--enable-kiosk` installs a user service that opens the browser after the local API is reachable.
   - `--target-dir /path` can override the default `/opt/camera-appliance`.
   - `--no-start` installs and enables services without starting them immediately.
5. Edit `/etc/camera-appliance/secrets.env` and set the real Tapo camera password.
   Alternatively set the camera password later in the local UI under **System**. The app first tries the OS keyring via `secret-tool` and falls back to `/etc/camera-appliance/local.env`.
6. Check `bin/status` or `camera-appliance status`.
7. Open `http://127.0.0.1:8091`, start camera discovery, and assign devices to `cam1` through `cam5`.
8. Render go2rtc config and restart go2rtc.
9. Reboot once and confirm `camera-appliance.service`, go2rtc, and the kiosk browser recover automatically.

## macOS Testbetrieb

For local development on macOS, do not install systemd services. Use:

```bash
make build
CAMERA_APPLIANCE_STATE_DIR=$PWD/data CAMERA_APPLIANCE_CONFIG_DIR=$PWD CAMERA_APPLIANCE_FRONTEND_DIST=$PWD/frontend/dist ./bin/camera-appliance serve
```

The helper scripts use the local repository by default. Set `CAMERA_APPLIANCE_HOME=/opt/camera-appliance` to point them at an installed Linux appliance.
