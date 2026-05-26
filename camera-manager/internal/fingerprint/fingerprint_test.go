package fingerprint

import "testing"

func TestNormalizeMAC(t *testing.T) {
	fp := Normalize(Fingerprint{MACAddress: "aa-bb-cc-dd-ee-ff", Manufacturer: " TP-Link ", LastIP: " 192.168.1.2 "})
	if fp.MACAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("mac not normalized: %q", fp.MACAddress)
	}
	if fp.Manufacturer != "TP-Link" || fp.LastIP != "192.168.1.2" {
		t.Fatalf("text not trimmed: %+v", fp)
	}
}

func TestDeviceIDDeterministic(t *testing.T) {
	fp := Fingerprint{Manufacturer: "TP-Link", Model: "Tapo C320WS", SerialNumber: "123"}
	if DeviceID(fp) != DeviceID(fp) {
		t.Fatal("device id should be deterministic")
	}
}
