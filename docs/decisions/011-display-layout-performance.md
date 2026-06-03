# 011 - Kamera-Transforms, Kiosk-Layouts und Performance-Modi

Status: accepted
Datum: 2026-06-03

## Kontext

Eine Kundenkamera ist vertikal beziehungsweise um 90 Grad gedreht. Zusätzlich braucht der Kunde steuerbare Viewer-Layouts und eine Möglichkeit, bei schwacher Hardware Last zu reduzieren.

## Entscheidung

Pro Kamera gibt es Anzeigeeinstellungen:

- Rotation
- Mirror
- Flip
- Fit Mode
- Crop
- Streamprofil
- Pfad-Policy

Der Viewer unterstützt Kiosk-Layouts und konfigurierbare Tile-Größen. Layouts können über Settings und URL-Parameter ausgewählt werden. Performance-Modi reduzieren bei Bedarf Anzeige- und Streamlast.

## Konsequenzen

- Kamera-Firmware muss nicht geändert werden.
- Vertikale Kameras können ohne Verzerrung angezeigt werden.
- Layouts bleiben nach Reload erhalten.
- Performance kann reduziert werden, ohne Aufzeichnung einzuführen.

## Offen

Weitere Layout-Feinheiten können ergänzt werden, solange die Viewer-Oberfläche scanbar und zuverlässig bleibt.
