# 013 - Chrome-freier Viewer und entkartetes UI

Status: accepted
Datum: 2026-06-03

## Kontext

Der Kiosk-Viewer war mit Bedienelementen überladen (Buttons, Layout- und
Performance-Auswahl, Diagnose-Panels) und jede Kachel trug ein dauerhaftes Overlay. Das
verschenkte Platz und widersprach dem Ziel einer reinen Kameraansicht. Zusätzlich gab es
app-weite Redundanzen (Scan/Render/Restart an mehreren Stellen, doppelte Ereignis- und
Layout-Bedienung) und ein linien-/cardlastiges Design.

## Entscheidung

- Der Viewer (`/`) zeigt standardmäßig **nur Kameras** im konfigurierbaren Raster: keine
  Buttons, keine Overlays, große runde Ecken, randlos (full-bleed).
- Bedienung erfolgt über drei Zustände:
  - **Clean** (Standard): nur Kacheln; ein automatisch ausblendendes Steuer-Cluster
    (erscheint bei Mausbewegung) bietet Bearbeiten, Vollbild und Verwaltung.
  - **Spotlight**: Klick auf eine Kamera vergrößert sie, erneuter Klick/Esc zurück.
  - **Bearbeiten** (nur Admin): freies Raster — Kameras per Drag verschieben/tauschen, auf
    Zonen platzieren, Spalten/Zeilen frei skalieren; Zuschnitt per Zoom (Rad) und Pan
    (Shift+Ziehen) direkt an der Kachel. Gesten: Ziehen = verschieben/tauschen,
    Trenner ziehen = Größe.
  - **Vollbild** schaltet sämtliche Bedienelemente ab (Kiosk).
- Operative Steuerung (Discovery, go2rtc erzeugen/neu starten, Performance-Modus) liegt
  **nur noch auf den Admin-Seiten**, nicht mehr im Viewer.
- Standard-Bildmodus ist **`contain`** (ganze Kamera sichtbar); `cover`/Zuschnitt ist
  per-Kamera-Opt-in.
- Informationsarchitektur konsolidiert: die Seite „Übersicht" entfällt (Redirect auf
  Einrichtung); Statusüberblick und „Braucht Aufmerksamkeit" leben in der Einrichtung; die
  Kiosk-Layout-Bedienung verlässt die System-Seite.
- Design ist borderless/trennlinienfrei; Cards nur dort, wo eine Gruppierung nötig ist
  (Formulare, Modals).

## Konsequenzen

- Die Kameraansicht nutzt den vollen Bildschirm ohne Chrome.
- Die benannten Kiosk-Layout-Presets aus [011] (2x2, 4 plus groß, Vertikal plus Raster,
  Große Ansicht) entfallen in der Bedienung zugunsten **eines einzigen frei konfigurierbaren
  Rasters**. Die zugrunde liegende Custom-Layout-Engine aus [011] bleibt technisch erhalten
  und trägt das freie Raster; Min-/Max-Grenzen der Sektionsgrößen wurden gelockert.
- Bestandskameras ohne explizite Anzeige-Einstellung zeigen ab jetzt das ganze Bild
  (`contain`) statt formatfüllend (`cover`).
- Weniger Redundanz: Scan/Render/Restart und Ereignisprotokoll haben je eine Heimat.

## Offen

Die Zuschnitt-Bedienung im Viewer kann verfeinert werden (z. B. Mirror/Flip inline). Die
tiefe Kamera-Diagnose bleibt auf der Gerätedetail-Seite.

[011]: 011-display-layout-performance.md
