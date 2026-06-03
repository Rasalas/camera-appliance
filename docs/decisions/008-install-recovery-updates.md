# 008 - Installation, Boot-Recovery und Updates mit Rollback

Status: accepted
Datum: 2026-06-03

## Kontext

Die Appliance soll beim Kunden installierbar sein und auch nach weiterer Entwicklung aktualisiert werden können. Ein kaputtes Update darf den Kundenbetrieb nicht dauerhaft blockieren.

## Entscheidung

Es gibt Installations- und Betriebswerkzeuge:

- `bin/install`
- systemd-Service für die Appliance
- optionaler Kiosk-Service
- Desktop-Launcher
- Backup und Restore
- Release-Archive
- Update mit vorherigem Backup, Dateisnapshot, Healthcheck und Rollback

## Konsequenzen

- Eine Version kann beim Kunden übergeben werden, während weiterentwickelt wird.
- Updates können Dienste neu starten und bei Fehlern zurückrollen.
- Release-Archive schließen `.private`, `.git`, `data`, Secrets und lokale Laufzeitdateien aus.
