package matcher

import "camera-appliance/camera-manager/internal/fingerprint"

type Decision string

const (
	AutoMatch Decision = "auto"
	Suggested Decision = "suggested"
	Unknown   Decision = "unknown"
	Conflict  Decision = "conflict"
)

type Candidate struct {
	SlotID   string   `json:"slot_id"`
	DeviceID string   `json:"device_id"`
	Score    int      `json:"score"`
	Decision Decision `json:"decision"`
	Reasons  []string `json:"reasons"`
}

func Score(existing, discovered fingerprint.Fingerprint, credentialsOK bool) (int, []string) {
	existing = fingerprint.Normalize(existing)
	discovered = fingerprint.Normalize(discovered)
	score := 0
	reasons := make([]string, 0)
	if existing.SerialNumber != "" && existing.SerialNumber == discovered.SerialNumber {
		score += 80
		reasons = append(reasons, "serial number")
	}
	if existing.MACAddress != "" && existing.MACAddress == discovered.MACAddress {
		score += 70
		reasons = append(reasons, "mac address")
	}
	if existing.ONVIFEndpointRef != "" && existing.ONVIFEndpointRef == discovered.ONVIFEndpointRef {
		score += 60
		reasons = append(reasons, "onvif endpoint")
	}
	if existing.Manufacturer != "" && existing.Model != "" &&
		existing.Manufacturer == discovered.Manufacturer && existing.Model == discovered.Model {
		score += 20
		reasons = append(reasons, "manufacturer and model")
	}
	if existing.Hostname != "" && existing.Hostname == discovered.Hostname {
		score += 10
		reasons = append(reasons, "hostname")
	}
	if existing.LastIP != "" && existing.LastIP == discovered.LastIP {
		score += 5
		reasons = append(reasons, "last known ip")
	}
	if credentialsOK {
		score += 20
		reasons = append(reasons, "credentials")
	}
	return score, reasons
}

func Decide(score int) Decision {
	if score >= 80 {
		return AutoMatch
	}
	if score >= 40 {
		return Suggested
	}
	return Unknown
}

func DetectConflicts(candidates []Candidate) []Candidate {
	bySlot := map[string]int{}
	byDevice := map[string]int{}
	for _, c := range candidates {
		if c.Decision == Unknown {
			continue
		}
		bySlot[c.SlotID]++
		byDevice[c.DeviceID]++
	}
	out := make([]Candidate, len(candidates))
	copy(out, candidates)
	for i := range out {
		if out[i].Decision != Unknown && (bySlot[out[i].SlotID] > 1 || byDevice[out[i].DeviceID] > 1) {
			out[i].Decision = Conflict
		}
	}
	return out
}
