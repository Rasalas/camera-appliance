# 004 - AgentDVR bleibt optional

Status: accepted
Datum: 2026-06-03

## Kontext

Ursprünglich war AgentDVR als statischer Viewer vorgesehen. Im Verlauf wurde klar, dass die eigene Viewer-Oberfläche ohnehin Kameraerkennung, Status, Layouts, Login, Transforms und Diagnose abbilden muss.

## Entscheidung

AgentDVR ist für den normalen Betrieb nicht erforderlich. Es bleibt als optionales Docker-Compose-Profil für NVR-Experimente oder Vergleichstests verfügbar.

Der primäre Viewer ist die eigene lokale Oberfläche von `camera-appliance`.

## Konsequenzen

- Weniger Abhängigkeit von AgentDVR-Konfiguration.
- Keine AgentDVR-Pflicht für Installation, Boot, Status oder Kundenbetrieb.
- Wenn AgentDVR genutzt wird, soll es ebenfalls nur stabile go2rtc-Aliase verwenden.

## Offen

Aufzeichnung oder NVR-Funktionen bleiben ein mögliches Zukunftsthema, aber nicht Teil des aktuellen Ziels.
