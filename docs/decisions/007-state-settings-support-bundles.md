# 007 - SQLite-State, Settings und Support-Bundles

Status: accepted
Datum: 2026-06-03

## Kontext

Die Appliance braucht lokale Persistenz für Geräte, Bindings, Settings, Events, Sessions, Pfadzustände und Diagnose.

## Entscheidung

SQLite ist der lokale State-Store. Settings werden als Key-Value-Werte gespeichert. Support-Bundles sammeln redigierte Diagnose aus Status, Viewer, Events, Settings, Relays, Docker und go2rtc.

## Konsequenzen

- Kein externer Datenbankdienst nötig.
- Settings können schrittweise erweitert werden.
- Support-Bundles sind ein zentrales Werkzeug für Fernanalyse, ohne Passwörter auszugeben.

## Offen

Wenn Settings stark strukturierter werden, kann später eine eigene Tabelle oder ein versioniertes JSON-Modell sinnvoll sein.
