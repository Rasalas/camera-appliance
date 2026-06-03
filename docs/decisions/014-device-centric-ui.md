# 014 - Geräte-zentrierte Bedienung und System-Unterseiten

Status: accepted
Datum: 2026-06-03

## Kontext

Nach dem Viewer-Redesign ([013](013-viewer-redesign.md)) war die übrige Admin-Oberfläche dran.
Die manuelle Slot-Zuordnung (cam1–cam5) ergab keinen Sinn mehr — die Platzierung steuert der
Viewer. Die Einrichtung war umständlich/redundant, die System-Seite unübersichtlich.

## Entscheidung

- **Slots bleiben rein intern.** Die stabilen Stream-Aliasse cam1–cam5 (Identität statt IP,
  [002](002-stable-device-identity.md)/[003](003-go2rtc-stable-aliases.md)) bestehen unverändert
  weiter. Es gibt **keine manuelle Slot-Bedienung** mehr: pro Kamera nur **„Anzeigen" an/aus**;
  beim Aktivieren wird automatisch ein freier Slot zugeordnet (Binding), beim Deaktivieren wieder
  freigegeben. **Wo** eine Kamera erscheint, legt ausschließlich die Kameras-Ansicht fest (Mosaic,
  013). Folge: maximal 5 gleichzeitig sichtbare Kameras (5 Slots).
- **Einrichtung → „Geräte"**: schlichte Kamera-Liste mit Vorschau und Anzeigen-Schalter; Discovery
  per Knopf, RTSP-Kamera manuell hinzufügbar. Konfiguration je Kamera auf der **Detailseite**
  (`/kamera/:id`): Zugang, Pfad, Diagnose und **Anzeige/Zuschnitt direkt am Referenzbild**
  (Ziehen = verschieben, Rad = Zoom, Buttons für Drehen/Fit/Reset), mit Auto-Save.
- **Viewer** zeigt nur aktivierte Kameras.
- **System in Unterseiten**: Allgemein · Zugriff · Netzwerk & Relays · Wartung (eigene Routen unter
  `/system/*`, Tab-Navigation). Gemeinsamer Settings-Zustand über ein Composable; `PUT /api/settings`
  ist ein Upsert pro Key, daher speichert jede Unterseite gefahrlos nur ihre Werte.

## Konsequenzen

- Deutlich weniger Klicks; keine separate Slot-Seite.
- Geräte-Identität und Alias-Schicht (002/003) bleiben technisch unverändert — nur die Bedienung
  ist verlagert.
- Bewusste Grenze: maximal 5 gleichzeitig sichtbare Kameras.

## Offen

- Erneutes Aktivieren einer zuvor ausgeblendeten Kamera kann einen anderen freien Slot bekommen,
  wodurch sich ihre Position im Viewer einmalig zurücksetzen kann. Bei Bedarf später „letzten Slot
  pro Gerät merken".
