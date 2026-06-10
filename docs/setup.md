# Setup

1. Install Linux Mint updates.
2. Make sure the release archive is publicly reachable from GitHub Releases.
3. Run the bootstrap installer:

   ```bash
   curl -fsSL https://raw.githubusercontent.com/Rasalas/camera-appliance/main/install.sh | sudo bash
   ```

   - The bootstrap installs missing apt bootstrap dependencies where possible.
   - On a fresh laptop it downloads the release and calls `camera-appliance install`.
   - On an existing appliance it calls `camera-appliance update`.
   - It auto-detects the desktop user and enables the kiosk browser service by default.
   - `--user USER` overrides the detected desktop/kiosk user.
   - `--no-kiosk` skips the kiosk browser user service.
   - `--no-start` installs and enables services without starting them immediately.
   - Firewall rules are not changed; normal kiosk operation uses localhost bindings only.
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
