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
