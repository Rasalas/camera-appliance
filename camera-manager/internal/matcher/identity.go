package matcher

import (
	"fmt"
	"strings"

	"camera-appliance/camera-manager/internal/fingerprint"
)

// ResolveIdentity reuses an existing identifier only when stable attributes
// agree. IP, hostname, model alone and working credentials are not identity.
// Ambiguity is reported instead of silently moving bindings or credentials.
func ResolveIdentity(existing map[string]fingerprint.Fingerprint, discovered fingerprint.Fingerprint) (string, error) {
	discovered = fingerprint.Normalize(discovered)
	match := ""
	equal := func(a, b string) bool { return a != "" && b != "" && strings.EqualFold(a, b) }
	for id, candidate := range existing {
		candidate = fingerprint.Normalize(candidate)
		serialMatch := equal(candidate.SerialNumber, discovered.SerialNumber) && equal(candidate.Manufacturer, discovered.Manufacturer) && equal(candidate.Model, discovered.Model)
		if !serialMatch && !equal(candidate.MACAddress, discovered.MACAddress) && !equal(candidate.ONVIFEndpointRef, discovered.ONVIFEndpointRef) {
			continue
		}
		if candidate.SerialNumber != "" && discovered.SerialNumber != "" && !equal(candidate.SerialNumber, discovered.SerialNumber) {
			return "", fmt.Errorf("widersprüchliche Geräteidentität für %s: Seriennummer stimmt nicht überein", id)
		}
		if match != "" {
			return "", fmt.Errorf("mehrdeutige Geräteidentität: %s und %s", match, id)
		}
		match = id
	}
	return match, nil
}
