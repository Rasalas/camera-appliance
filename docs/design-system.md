# Design-System / Styleguide

Referenz für die Oberfläche der camera-appliance. Quelle der Wahrheit für Tokens ist
`frontend/src/styles/base.css` (`:root`); dieses Dokument erklärt die Prinzipien und wie die
Bausteine zu verwenden sind. Bei Designänderungen beide Stellen synchron halten.

Verwandt: [005 – Lokale deutsche UI](decisions/005-local-german-ui.md),
[013 – Chrome-freier Viewer](decisions/013-viewer-redesign.md).

## Prinzipien

1. **Borderless / trennlinienfrei.** Gruppierung über Whitespace und Überschriften, nicht
   über Linien. Keine `border`/`border-bottom`-Hairlines als Trenner.
2. **Cards nur wo nötig.** Eine Card (`.panel.card`, gefüllte Fläche) nur für
   zusammengehörige, interaktive Gruppen (Formulare, dichte Settings, Modals). Reine
   Abschnitte sind transparente Sektionen (`.panel`) mit Überschrift.
3. **Kameras zuerst.** Der Viewer zeigt im Normalzustand nur Kameras — kein Chrome, keine
   Overlays. Bedienung erscheint nur auf Bedarf (Hover / Bearbeiten).
4. **Identität, Ruhe, Signal.** Dunkle „Control-Room"-Fläche, ruhige Typografie, Farbe nur
   als Statussignal (live/warn/down).
5. **Deutsche UI-Texte**, englische Bezeichner/Klassen/Kommentare im Code.

## Tokens

### Farbe
- Fläche: `--bg` (Seite), `--surface` (Card/Block), `--raised` (Buttons/Inputs), `--hairline(-strong)` nur noch punktuell (Fokus, Resize-Griffe).
- Text: `--ink` (primär) → `--ink-soft` → `--ink-mute` → `--ink-dim`.
- Signal: `--live` (phosphorgrün, on-air), `--warn` (amber), `--danger` (rec-rot) je mit `*-bg`-Tint. Farbe **nur** für Status/Akzent, nicht für Dekor.

### Typografie
- `--mono` (JetBrains Mono): die Arbeitsschrift — Headlines, Labels (UPPERCASE + `letter-spacing`), Inhalte. Seitentitel (`.headline`, `.modal-head h2`) sind **kompakt und mono**, keine großen Serifen.
- `--serif` (Instrument Serif): nur noch **dekorative Zahlen/Marke** (`.stat .value`, Brand-Logo), nicht für Seitentitel.

### Radien — konzentrische Regel
Ein gerundetes Element **in** einem gerundeten Container nimmt den **nächstkleineren** Token,
damit verschachtelte Ecken parallel laufen.
- `--radius-tile: 20px` — Kamera-Kacheln (außen, groß).
- `--radius: 12px` — eigenständige Cards, Sektionen, Modals.
- `--radius-sm: 8px` — Inputs, Buttons, Chips **und alles, was in einer Card liegt** (z. B. `.bay`, `.relay-config`, `.assignment-display`, `.transform-preview-stage`).
- Pills/Punkte: `999px`.

### Abstand
- `--shell-pad` (Seitenrand), `--gutter` (Abstand zwischen Sektionen). Trennung über Abstand statt Linien.

## Bausteine

- **Button** `.btn` (+ `.primary`/`.live`/`.danger`/`.ghost`, `.sm`/`.lg`/`.icon`): gefüllt (`--raised`), randlos, `--radius-sm`. Im Viewer UPPERCASE; in der Verwaltung (`admin.css`) normale Groß-/Kleinschreibung, 14 px, mindestens 44 px hoch.
- **Aktion statt Status**: Schalter, die etwas auslösen, heißen nach der Aktion („Ausblenden“/„Einblenden“). Der Zustand steht daneben als `.pill` („In der Live-Ansicht“/„Ausgeblendet“). Aktionen, die die Live-Ansicht kurz unterbrechen, werden vorher bestätigt.
- **Pill** `.pill` (+ `.live`/`.warn`/`.down`): Statusanzeige mit Punkt.
- **Sektion** `.panel`: transparent, nur Überschrift + Inhalt. **Card** `.panel.card`: gefüllte Fläche für Gruppen.
- **Feld** `.field` mit `.lbl` + Input; Inputs gefüllt, randlos, Fokus = grüner Rahmen.
- **Liste** `.result-list`/`.ticker`: zebriert (`nth-child(odd)`), keine Zeilentrenner.
- **Toggle** `.toggle-row`, **Hinweis** `.notice` (+ `.ok`/`.warn`/`.err`), **Modal** `.modal`.
- **Fehler im Dialog**: Fehler eines offenen Dialogs erscheinen als `.notice.err` mit `role="alert"` im Dialog selbst; der Dialog bleibt offen und die Eingaben bleiben erhalten.
- **Ladefehler**: Seiten zeigen bei fehlgeschlagenem Laden einen `.notice.err` mit „Erneut versuchen“ statt des leeren Zustands.
- **Technische Details**: MAC, Seriennummer, Hostname, Geräte-ID, erkannte Dienste (RTSP/ONVIF) und Rohdaten liegen eingeklappt in `details.advanced` „Technische Details“.
- **Stat/Service-Strip**: gleich große gefüllte Blöcke mit Gap (keine Innenlinien).

## Viewer-Chrome-Modell

Vier Zustände (siehe [013](decisions/013-viewer-redesign.md)):
- **Clean** (Standard): nur Kacheln, full-bleed, `contain`. Auto-ausblendendes HUD (Bearbeiten/Vollbild/Verwaltung).
- **Spotlight**: Klick auf Kamera = groß, Klick/Esc = zurück.
- **Bearbeiten**: Split-Editor (Mosaic-Baum). Rand andocken = teilen, Außenrand = volle Spalte/Reihe, Mitte = tauschen, Trenner = Größe, Rad = Zoom, Shift+Ziehen = Ausschnitt. Default-Layout = gleichmäßiges Raster (`ceil(√n)` Spalten).
- **Vollbild**: schaltet alles Chrome ab.

## Navigation / IA

Seitenleiste (`frontend/src/navigation.ts`, `layouts/AppLayout.vue`):

- **Live-Ansicht** (`/`, full-bleed Viewer).
- **Kameras** (`/einrichtung`): Kameraliste, Detailseite `/kamera/:id`; darunter **Bild-Upload**
  (`/kameras/bild-upload`).
- **System** (`/system`, allgemeine Einstellungen); darunter **Zugriff** (`/system/zugriff`),
  **Weiterleitungen** (`/system/relays`, SSH-Relays) und **Kamera-Zugänge**
  (`/system/identitaeten`, gemeinsame Kamera-Logins).
- **Wartung** (`/system/wartung`, Übersicht); darunter **Überwachung** (`/system/wartung/watchdog`),
  **Sicherung** und **Support**.
- **Über Watchdeck** (`/system/ueber`, inklusive Updates).
- Mobil: Bottom-Navigation (Home `/verwaltung`, Kameras, Live-Ansicht); Suche unter dem Logo
  (Cmd/Ctrl+K).

Die Routen behalten ihre bisherigen Pfade, damit Lesezeichen und Anleitungen gültig bleiben;
umbenannt wurden nur die Beschriftungen. Slots werden automatisch im Hintergrund vergeben
(kein Slot-UI).

### Begriffe

- Kameras tragen einen frei wählbaren **Namen** (max. 40 Zeichen), gespeichert zur stabilen
  Geräte-ID. Ohne Namen wird „Hersteller Modell“ angezeigt.
- **Gerätedaten** = Hersteller, Modell, IP-Adresse einer Kamera. **Kamera-Zugang** = gespeicherter
  Login (früher „Identität“).
- **Videodienst** statt „go2rtc“, **Bildqualität** „Standard (empfohlen)“/„Hohe Auflösung“ statt
  `stream2`/`stream1`, **Einpassen**/**Ausfüllen** statt `contain`/`cover`, **Weiterleitung**
  statt „Relay“, **Überwachung** statt „Watchdog“, **Sicherung** statt „Backup“.
- Zählwörter mit Singular/Plural („1 Kamera“, „3 Kameras“), keine „Gerät(e)“-Klammern.

## Offene Punkte / To-do

Hier sammeln, was am Design noch nicht passt, damit es nicht verloren geht:

- [erledigt] Viewer-HUD: „Fertig"/Buttons jetzt pill-förmig (passend zur Pill-Leiste).
- [erledigt] Verwaltung: große Serifen-Titel durch kompakte Mono-Header ersetzt.
- [erledigt] **Einrichtung → „Geräte"**: Kamera-Liste + Anzeigen-Schalter, Detailseite mit
  Konfiguration und Zuschnitt direkt am Bild; Slot-Bedienung entfernt (siehe
  [014](decisions/014-device-centric-ui.md)). Der Schalter heißt jetzt „Ausblenden“/„Einblenden“ mit Bestätigung.
- [erledigt] **System** in Unterseiten geteilt: Allgemein · Zugriff · Weiterleitungen · Kamera-Zugänge; Wartung eigener Bereich.
- [offen] Die Verwaltung (`admin.css`) trennt Listenzeilen (`.device-card`, `.relay-row`, `.result-row`)
  und Bearbeiten-Sektionen noch mit Hairlines. Das weicht von Prinzip 1 ab; Wartungsübersicht,
  Support-Protokoll und Über-Seite nutzen bereits Abstand und Flächen statt Linien.
- [erledigt] System → Allgemein, Weiterleitungen, Sicherung und Update-Hinweis nutzen dieselben Begriffe (Videodienst, Weiterleitung, Sicherung).
