#!/usr/bin/env python3
import argparse
import json
import os
import re
import urllib.request
import xml.etree.ElementTree as ET


def post(base_url, path, payload):
    request = urllib.request.Request(
        base_url + path,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=15) as response:
        return json.load(response)


def load_cameras(path):
    raw = open(path, "rb").read().decode("utf-8-sig")
    raw = raw.replace('encoding="utf-16"', 'encoding="utf-8"', 1)
    root = ET.fromstring(raw)
    cameras = []
    for camera in root.findall("./cameras/camera"):
        settings = camera.find("settings")
        if settings is None:
            continue
        source = settings.findtext("substream") or settings.findtext("mainstream") or ""
        match = re.match(r"^rtsp://([^/:]+)(?::(\d+))?/(.+)$", source.strip())
        if not match:
            continue
        cameras.append(
            {
                "ip": match.group(1),
                "username": (settings.findtext("login") or "").strip(),
                "password": settings.findtext("password") or "",
                "stream": match.group(3).strip("/") or "stream2",
                "label": camera.attrib.get("name", "").strip(),
            }
        )
    return cameras


def main():
    parser = argparse.ArgumentParser(description="AgentDVR-Kameras lokal in camera-appliance übernehmen")
    parser.add_argument("--objects", required=True)
    parser.add_argument("--base-url", default="http://127.0.0.1:8091")
    parser.add_argument("--admin-password-file", required=True)
    args = parser.parse_args()

    cameras = load_cameras(args.objects)
    if not cameras:
        raise SystemExit("Keine migrierbaren AgentDVR-Kameras gefunden.")
    if len(cameras) > 5:
        raise SystemExit("Mehr als fünf Kameras gefunden; automatische Slot-Zuordnung abgebrochen.")

    for index, camera in enumerate(cameras, 1):
        result = post(args.base_url, "/api/devices/manual", camera)
        post(
            args.base_url,
            "/api/bindings",
            {
                "slot_id": f"cam{index}",
                "device_id": result["device"]["id"],
                "label": camera["label"] or f"Kamera {index}",
                "username": camera["username"],
                "stream_name": camera["stream"],
                "enabled": True,
            },
        )

    post(args.base_url, "/api/go2rtc/render", {})
    password = open(args.admin_password_file, encoding="utf-8").read().strip()
    if not password:
        raise SystemExit("Admin-Passwortdatei ist leer.")
    post(args.base_url, "/api/auth/password", {"role": "admin", "password": password})
    print(f"{len(cameras)} AgentDVR-Kameras wurden übernommen.")


if __name__ == "__main__":
    main()
