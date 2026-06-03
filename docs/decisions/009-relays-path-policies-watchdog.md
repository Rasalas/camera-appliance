# 009 - Relays, Pfad-Policies und Watchdog

Status: accepted
Datum: 2026-06-03

## Kontext

Im lokalen Netz können Kameras je nach Repeater, Route oder Host unterschiedlich erreichbar sein. Eine Kamera kann direkt vom Appliance-Rechner erreichbar sein oder nur über einen anderen Host.

## Entscheidung

Das System unterstützt direkte Kamera-Pfade und SSH-Relays. Pro Kamera gibt es Pfad-Policies:

- `auto`
- `prefer_direct`
- `prefer_relay`
- `direct_only`
- `relay_only`

Der Watchdog prüft go2rtc und Kamera-Pfade regelmäßig. Er versucht bei Reconnects nicht nur den bisherigen Pfad, sondern bewertet direkte und Relay-Pfade gemäß Policy und Stabilitätszählern.

## Konsequenzen

- Relays sind konfigurierbar, aber nicht zwingend.
- Bei Repeater-Wechseln kann das System auf den funktionierenden Pfad wechseln.
- Pfadwechsel werden stabilisiert, damit einzelne kurze Aussetzer nicht sofort flappen.
- go2rtc wird bei echten Pfadwechseln gerendert und abhängig von Cooldown/Setting neu gestartet.
