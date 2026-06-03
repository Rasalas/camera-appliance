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
- `--serif` (Instrument Serif): große Display-Zahlen/Headlines (`.headline`, `.stat .value`, Slot-Namen).
- `--mono` (JetBrains Mono): alles andere; Labels in `UPPERCASE` mit `letter-spacing`.

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

- **Button** `.btn` (+ `.primary`/`.live`/`.danger`/`.ghost`, `.sm`/`.lg`/`.icon`): gefüllt (`--raised`), randlos, `--radius-sm`, UPPERCASE.
- **Pill** `.pill` (+ `.live`/`.warn`/`.down`): Statusanzeige mit Punkt.
- **Sektion** `.panel`: transparent, nur Überschrift + Inhalt. **Card** `.panel.card`: gefüllte Fläche für Gruppen.
- **Feld** `.field` mit `.lbl` + Input; Inputs gefüllt, randlos, Fokus = grüner Rahmen.
- **Liste** `.result-list`/`.ticker`: zebriert (`nth-child(odd)`), keine Zeilentrenner.
- **Toggle** `.toggle-row`, **Hinweis** `.notice` (+ `.ok`/`.warn`/`.err`), **Modal** `.modal`.
- **Stat/Service-Strip**: gleich große gefüllte Blöcke mit Gap (keine Innenlinien).

## Viewer-Chrome-Modell

Vier Zustände (siehe [013](decisions/013-viewer-redesign.md)):
- **Clean** (Standard): nur Kacheln, full-bleed, `contain`. Auto-ausblendendes HUD (Bearbeiten/Vollbild/Verwaltung).
- **Spotlight**: Klick auf Kamera = groß, Klick/Esc = zurück.
- **Bearbeiten**: Split-Editor (Mosaic-Baum). Rand andocken = teilen, Außenrand = volle Spalte/Reihe, Mitte = tauschen, Trenner = Größe, Rad = Zoom, Shift+Ziehen = Ausschnitt. Default-Layout = gleichmäßiges Raster (`ceil(√n)` Spalten).
- **Vollbild**: schaltet alles Chrome ab.

## Offene Punkte / To-do

Hier sammeln, was am Design noch nicht passt, damit es nicht verloren geht:

- _(offen)_ Einzelne Stellen, die noch nicht gefallen — bitte konkret notieren (Seite + Element).
