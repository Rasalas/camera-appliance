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
