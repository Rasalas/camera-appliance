# 002 - Stabile Geräteidentität statt IP-Adresse

Status: accepted
Datum: 2026-06-03

## Kontext

Kameras können im WLAN andere DHCP-Adressen bekommen oder zwischen Repeatern wechseln. Eine feste Zuordnung `cam1 = 192.168.x.y` wäre im Kundenbetrieb fragil.

## Entscheidung

Kameras werden als physische Geräte modelliert. Die IP-Adresse ist nur eine aktuelle Laufzeiteigenschaft.

Geräteidentität kann enthalten:

- MAC-Adresse
- ONVIF Endpoint Reference
- Seriennummer
- Hersteller und Modell
- Hardware-ID
- Hostname
- letzte bekannte IP

Slot-Bindings zeigen auf Geräteidentitäten, nicht auf IP-Adressen.

## Konsequenzen

- Discovery und Matching sind zentrale Funktionen.
- Reconnects, Rendering und Watchdog dürfen aktuelle IPs neu bewerten.
- Konflikte müssen sichtbar bleiben und dürfen nicht blind überschrieben werden.
