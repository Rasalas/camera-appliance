# 006 - Secrets, Redaction und Laufzeitpfade

Status: accepted
Datum: 2026-06-03

## Kontext

Kamera-Zugangsdaten und generierte RTSP-URLs sind sensibel. Gleichzeitig braucht der Kunde Backup, Support-Bundles und Diagnose.

## Entscheidung

Secrets liegen außerhalb von Git:

- `/etc/camera-appliance/secrets.env`
- `/etc/camera-appliance/local.env`
- lokale Secret-Dateien oder Keyring für Kamera- und Identitätspasswörter

Runtime-State liegt unter:

- `/var/lib/camera-appliance/state.db`
- `/var/lib/camera-appliance/generated/go2rtc.yaml`
- `/var/lib/camera-appliance/backups`

Alle CLI-, API-, UI- und Support-Ausgaben redigieren Passwort-URLs und sensitive Settings.

## Konsequenzen

- Generierte go2rtc-Konfiguration wird nie committed.
- Support-Bundles sind redigiert, enthalten aber weiterhin Netzwerk- und Systemdiagnose.
- Backup kann sensitive Daten enthalten und muss entsprechend behandelt werden.
