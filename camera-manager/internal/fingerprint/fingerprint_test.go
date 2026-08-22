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

func TestNormalizeMACRejectsUnresolvableEntries(t *testing.T) {
	cases := map[string]string{
		"<incomplete>":      "",
		"(incomplete)":      "",
		"00:00:00:00:00:00": "",
		"":                  "",
		"garbage":           "",
		"aabbccddeeff":      "AA:BB:CC:DD:EE:FF",
		"AA-BB-CC-DD-EE-FF": "AA:BB:CC:DD:EE:FF",
	}
	for input, want := range cases {
		if got := Normalize(Fingerprint{MACAddress: input}).MACAddress; got != want {
			t.Errorf("normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidMAC(t *testing.T) {
	if ValidMAC("") || ValidMAC("00:00:00:00:00:00") || ValidMAC("<I:NC:OM:PL:ET:E>") {
		t.Fatal("placeholder MACs must not count as valid identity attributes")
	}
	if !ValidMAC("AA:BB:CC:DD:EE:FF") {
		t.Fatal("regular MAC should be valid")
	}
}

func TestDeviceIDFallsThroughWithoutMAC(t *testing.T) {
	// Devices without any stable attribute get a random ID; two calls must
	// differ so unrelated devices never share an identity.
	fp := Fingerprint{Manufacturer: "RTSP", Model: "Unknown"}
	first, second := DeviceID(fp), DeviceID(fp)
	if first == second {
		t.Fatal("random fallback should produce distinct ids")
	}
}
