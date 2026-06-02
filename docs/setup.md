# Setup

1. Install Linux Mint updates.
2. Install Docker, Docker Compose plugin, Go, and Node.js/npm.
3. Clone or copy the repository to `/opt/camera-appliance`.
4. Run `sudo bin/install --user customer --enable-systemd --install-desktop-launchers`.
5. Edit `/etc/camera-appliance/secrets.env` and set the real Tapo camera password.
   Alternatively set the camera password later in the local UI under **System**. The app first tries the OS keyring via `secret-tool` and falls back to `/etc/camera-appliance/local.env`.
6. Start the stack with `sudo docker compose up -d`.
7. Open `http://127.0.0.1:8091`, start camera discovery, and assign devices to `cam1` through `cam5`.
8. Render go2rtc config and restart go2rtc.
9. Open the camera view at `http://127.0.0.1:8091` or use the desktop launcher **Kameras öffnen**.
