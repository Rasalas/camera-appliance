package matcher

import (
	"camera-appliance/camera-manager/internal/fingerprint"
	"testing"
)

func TestResolveIdentityRejectsContradictorySerial(t *testing.T) {
	existing := map[string]fingerprint.Fingerprint{"camera": {MACAddress: "AA:BB:CC:DD:EE:01", SerialNumber: "old"}}
	if _, err := ResolveIdentity(existing, fingerprint.Fingerprint{MACAddress: "AA:BB:CC:DD:EE:01", SerialNumber: "different"}); err == nil {
		t.Fatal("conflicting serial number accepted")
	}
}

func TestResolveIdentityUsesEndpointWithoutIP(t *testing.T) {
	existing := map[string]fingerprint.Fingerprint{"camera": {ONVIFEndpointRef: "urn:uuid:123", LastIP: "192.0.2.1"}}
	id, err := ResolveIdentity(existing, fingerprint.Fingerprint{ONVIFEndpointRef: "urn:uuid:123", LastIP: "192.0.2.2"})
	if err != nil || id != "camera" {
		t.Fatalf("endpoint identity lost: %s, %v", id, err)
	}
}
