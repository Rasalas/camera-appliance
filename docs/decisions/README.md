# Decision Records

Diese Decision Records dokumentieren Projektentscheidungen, die im Verlauf der Umsetzung getroffen wurden. Sie sind bewusst breiter als klassische Architektur-ADRs: Sie halten auch Produkt-, Betriebs-, Sicherheits- und Umsetzungsentscheidungen fest.

Siehe auch: [Design-System / Styleguide](../design-system.md) — Tokens, Prinzipien und UI-Bausteine.

## Statuswerte

- `accepted`: Gilt aktuell.
- `superseded`: Wurde durch eine spätere Entscheidung ersetzt.
- `deferred`: Bewusst vertagt.

## Entscheidungen

- [001 - Lokale Kamera-Appliance statt Cloud-System](001-local-camera-appliance.md)
- [002 - Stabile Geräteidentität statt IP-Adresse](002-stable-device-identity.md)
- [003 - go2rtc als stabile Stream-Alias-Schicht](003-go2rtc-stable-aliases.md)
- [004 - AgentDVR bleibt optional](004-agentdvr-optional.md)
- [005 - Lokale deutsche Admin- und Viewer-Oberfläche](005-local-german-ui.md)
- [006 - Secrets, Redaction und Laufzeitpfade](006-secrets-redaction-runtime-paths.md)
- [007 - SQLite-State, Settings und Support-Bundles](007-state-settings-support-bundles.md)
- [008 - Installation, Boot-Recovery und Updates mit Rollback](008-install-recovery-updates.md)
- [009 - Relays, Pfad-Policies und Watchdog](009-relays-path-policies-watchdog.md)
- [010 - Lokaler Login mit Admin- und Viewer-Rolle](010-local-login-roles.md)
- [011 - Kamera-Transforms, Kiosk-Layouts und Performance-Modi](011-display-layout-performance.md)
- [012 - Keine Aufzeichnung im aktuellen Zielbild](012-no-recording-current-scope.md)
- [013 - Chrome-freier Viewer und entkartetes UI](013-viewer-redesign.md)
