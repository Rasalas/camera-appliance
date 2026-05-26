package matcher

import (
	"testing"

	"camera-appliance/camera-manager/internal/fingerprint"
)

func TestScoreAndDecision(t *testing.T) {
	a := fingerprint.Fingerprint{SerialNumber: "123", MACAddress: "AA:BB:CC:DD:EE:FF", Manufacturer: "TP-Link", Model: "Tapo", LastIP: "192.168.1.2"}
	b := fingerprint.Fingerprint{SerialNumber: "123", MACAddress: "AA:BB:CC:DD:EE:FF", Manufacturer: "TP-Link", Model: "Tapo", LastIP: "192.168.1.9"}
	score, reasons := Score(a, b, false)
	if score < 150 {
		t.Fatalf("score too low: %d (%v)", score, reasons)
	}
	if Decide(score) != AutoMatch {
		t.Fatalf("expected auto match")
	}
}

func TestDetectConflicts(t *testing.T) {
	in := []Candidate{
		{SlotID: "cam1", DeviceID: "a", Score: 70, Decision: Suggested},
		{SlotID: "cam1", DeviceID: "b", Score: 65, Decision: Suggested},
	}
	out := DetectConflicts(in)
	if out[0].Decision != Conflict || out[1].Decision != Conflict {
		t.Fatalf("expected conflicts: %+v", out)
	}
}
