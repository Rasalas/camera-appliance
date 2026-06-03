# 010 - Lokaler Login mit Admin- und Viewer-Rolle

Status: accepted
Datum: 2026-06-03

## Kontext

Die Admin-Oberfläche darf im Kundennetz nicht offen sein. Gleichzeitig soll es Viewer-Zugänge geben, die nur Streams sehen, besonders für Geräte hinter einer Theke oder in allgemein zugänglichen Bereichen.

## Entscheidung

Es gibt lokale Rollen:

- `admin`: vollständige Konfiguration und Admin-API.
- `viewer`: Kameraansicht ohne Admin-Funktionen.

Passwörter werden gehasht gespeichert. Sessions laufen über HttpOnly/SameSite-Cookies mit Ablaufzeit. Ohne gesetztes Admin-Passwort bleibt die Bestandsinstallation offen; sobald ein Admin-Passwort gesetzt ist, greifen die Rollen.

Optional kann der Viewer öffentlich bleiben oder loginpflichtig werden. Ein lokaler Host-Bypass kann bewusst aktiviert werden.

## Konsequenzen

- Admin-API ist nach Aktivierung geschützt.
- Viewer-only Betrieb ist möglich.
- Der Login schützt nicht direkt Drittports; deshalb bleiben go2rtc-Ports in Compose lokal gebunden.
