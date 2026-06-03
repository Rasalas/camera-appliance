# 003 - go2rtc als stabile Stream-Alias-Schicht

Status: accepted
Datum: 2026-06-03

## Kontext

Viewer und optionale externe Tools sollen nicht mit wechselnden Kamera-IPs oder Kamera-Zugangsdaten konfiguriert werden.

## Entscheidung

go2rtc stellt stabile lokale Aliase bereit:

- `cam1`
- `cam2`
- `cam3`
- `cam4`
- `cam5`

`camera-appliance` rendert die go2rtc-Konfiguration aus Slots, Gerätebindungen, Zugangsdaten, aktuellen Pfaden und Streamprofilen.

## Konsequenzen

- Der Viewer konsumiert stabile Aliase.
- Die generierte go2rtc-Konfiguration kann Secrets enthalten und bleibt Runtime-State.
- go2rtc wird nach relevanten Änderungen neu gerendert und bei Bedarf neu gestartet.
