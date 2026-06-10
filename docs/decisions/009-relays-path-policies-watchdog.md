# 009 - Relays, Pfad-Policies und Watchdog

Status: accepted
Datum: 2026-06-03

## Kontext

Im lokalen Netz können Kameras je nach Repeater, Route oder Host unterschiedlich erreichbar sein. Eine Kamera kann direkt vom Appliance-Rechner erreichbar sein oder nur über einen anderen Host.

## Entscheidung

Das System unterstützt direkte Kamera-Pfade und SSH-Relays. Pro Kamera gibt es Pfad-Policies:

- `auto`
- `prefer_direct`
- `prefer_relay`
- `direct_only`
- `relay_only`

Der Watchdog prüft go2rtc und Kamera-Pfade regelmäßig. Er versucht bei Reconnects nicht nur den bisherigen Pfad, sondern bewertet direkte und Relay-Pfade gemäß Policy und Stabilitätszählern.

## Konsequenzen

- Relays sind konfigurierbar, aber nicht zwingend.
- Bei Repeater-Wechseln kann das System auf den funktionierenden Pfad wechseln.
- Pfadwechsel werden stabilisiert, damit einzelne kurze Aussetzer nicht sofort flappen.
- go2rtc wird bei echten Pfadwechseln gerendert und abhängig von Cooldown/Setting neu gestartet.

## Update 2026-06-10: Relay standardmäßig nutzbar, Ports automatisch

Die ursprüngliche Umsetzung verlangte pro Kamera × Relay einen manuell gepflegten lokalen
Forward-Port (`camera.relay_endpoint.<gerät>.<relay>.port`) — ohne ihn existierte der
Relay-Pfad für die Kamera nicht. Das machte die Netzwerk-Seite unübersichtlich und
widersprach dem Ziel, dass ein einmal eingerichtetes Relay einfach als Ersatzpfad bereitsteht.

Neu:

- **Automatische Portvergabe**: Fehlt der explizite Port, wird er aus dem Kameraplatz
  abgeleitet: `port_base` des Relays (Standard 18554, je Relay +20) plus Slot-Nummer −1
  (`cam1` → 18554, `cam2` → 18555 …). Explizit gesetzte Ports gewinnen weiterhin.
- **Auto-Start ist Standard** (`camera.relay.<id>.auto_start` unset ⇒ `true`): Der Watchdog
  hält das Relay am Laufen, sobald es definiert ist.
- **`direct_only`-Kameras werden nicht getunnelt**: Sie tauchen nicht in den SSH-Forwards auf.
- **Unvollständige Endpunkte blockieren den Tunnel nicht mehr**: Endpunkte ohne Ziel-IP werden
  beim Aufbau übersprungen statt den Start des gesamten Relays zu verhindern.

Damit reduziert sich die Bedienung auf: Relay einmal anlegen (Name, SSH-Ziel, go2rtc-Host),
pro Kamera optional „Muss über Relay“ (`relay_only`) oder „Nur direkt“ (`direct_only`) erzwingen —
sonst gilt `auto` mit selbständigem Failover und Rückwechsel. `prefer_direct`/`prefer_relay`
bleiben unterstützt, werden in der UI aber nur noch als Alt-Werte angezeigt.

Bedienoberfläche entsprechend Entscheidung 014 (geräte-zentriert): Die Systemseite
`/system/relays` verwaltet ausschließlich die Relays selbst (inkl. Forward-Status je Kamera);
der Verbindungsweg einer Kamera samt Endpunkt-Feinjustage liegt auf ihrer Detailseite
unter „Verbindung“.
