# Kundenanleitung

## Kameras öffnen

1. Laptop einschalten.
2. Wenn die Kameras nicht erscheinen, auf **Kameras öffnen** klicken.
3. Wenn eine Kamera fehlt, auf **Kameras neu suchen** klicken.
4. Wenn das nicht hilft, auf **Kamera-Server neu starten** klicken.
5. Wenn das nicht hilft, Laptop neu starten.
6. Wenn eine Kamera weiterhin fehlt, Stromversorgung der Kamera prüfen.

## Anzeige auf einem anderen Gerät

1. Unter **System → Zugriff** zuerst ein Admin-Passwort setzen.
2. **Zugriff aus dem lokalen Netzwerk erlauben** einschalten.
3. **Speichern und anwenden** auswählen und den Neustart abwarten.
4. Auf dem anderen Gerät `http://LOKALE-IP:8091/` öffnen.
5. Falls die Anmeldung länger gespeichert werden soll, beim Login ausdrücklich **Angemeldet bleiben** auswählen. Das Login gilt dann 30 Tage für dieses Gerät.

Die Netzwerkfreigabe bleibt ohne Admin-Passwort grundsätzlich geschlossen. Eine lokale Firewall muss TCP-Port 8091 gegebenenfalls zusätzlich für das lokale Subnetz erlauben.

## Vollbild

Über die Vollbild-Schaltfläche in der Kameraansicht wird – soweit der Browser dies unterstützt – gleichzeitig die Bildschirmsperre verhindert. Der lokale Kiosk-Modus fordert diese Funktion automatisch an. Browser können den Zugriff über eine unverschlüsselte LAN-Adresse ablehnen; dann müssen die Energiespareinstellungen des verwendeten Geräts zusätzlich angepasst werden.

## Aktualisierung

Aktualisierungen werden unter **System → Wartung** gestartet. Konfiguration und Kameraeinstellungen bleiben erhalten. Bei einer vorhandenen Version, deren Aktualisierung mit `unknown flag: --env-file` scheitert, ist einmalig dieses Übergangsupdate erforderlich:

```bash
sudo docker exec camera-manager camera-appliance update --no-restart
sudo docker compose --env-file /opt/camera-appliance/release.env -f /opt/camera-appliance/compose.yaml up -d --build --force-recreate --remove-orphans
```

Danach führt bei zukünftigen Updates ein vom Manager unabhängiger Hilfscontainer den Neustart aus.
