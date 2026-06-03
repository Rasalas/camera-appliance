# 005 - Lokale deutsche Admin- und Viewer-Oberfläche

Status: accepted
Datum: 2026-06-03

## Kontext

Der Kunde ist deutschsprachig und die Appliance soll lokal bedienbar sein. Setup, Status, Diagnose und Viewer sollen in einer Oberfläche erreichbar sein.

## Entscheidung

Die UI ist deutschsprachig und wird vom Go-Backend ausgeliefert. Sie enthält:

- Kameraansicht
- Einrichtung und Zuordnung
- Kameradetail-Diagnose
- Systemseite
- Login und Rollen
- Layout- und Performance-Steuerung

Die Oberfläche bindet standardmäßig an `127.0.0.1:8091`.

## Konsequenzen

- Texte, Fehlermeldungen und Bedienflächen sind auf lokale Nutzung ausgerichtet.
- Admin-Funktionen werden geschützt, sobald ein Admin-Passwort gesetzt ist.
- Viewer-only Nutzung ist möglich, ohne Admin-Konfiguration offenzulegen.
