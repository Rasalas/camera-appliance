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
