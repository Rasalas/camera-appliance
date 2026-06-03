# 001 - Lokale Kamera-Appliance statt Cloud-System

Status: accepted
Datum: 2026-06-03

## Kontext

Das System soll bei einem Kunden auf einem Linux-Mint-Laptop laufen. Der Zielnutzer soll idealerweise nur den Rechner einschalten und die Kameras sehen. Das Kundennetz und reale Kamera-Zugangsdaten sollen nicht von Cloud-Diensten abhängen.

## Entscheidung

Wir bauen eine lokale Appliance:

- Go-Backend mit CLI und lokaler HTTP-API.
- Vue-Frontend, vom Backend ausgeliefert.
- SQLite als lokaler Zustand.
- Docker Compose für go2rtc und optionale Zusatzdienste.
- systemd und Desktop-Launcher für Boot, Kiosk und manuelle Aktionen.

Die Standardadresse bleibt `http://127.0.0.1:8091`.

## Konsequenzen

- Das System funktioniert ohne Internet und ohne externe Konten.
- Installation, Updates und Recovery müssen lokal robust sein.
- Remote-Wartung ist nur über separate, bewusst eingerichtete Wege möglich.

## Offen

Eine spätere Fernwartung kann ergänzt werden, aber nicht als Voraussetzung für Betrieb oder Sicherheit.
