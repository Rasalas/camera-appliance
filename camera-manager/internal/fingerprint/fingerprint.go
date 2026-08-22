package fingerprint

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
)

type Fingerprint struct {
	MACAddress       string `json:"mac_address,omitempty"`
	ONVIFEndpointRef string `json:"onvif_endpoint_ref,omitempty"`
	SerialNumber     string `json:"serial_number,omitempty"`
	Manufacturer     string `json:"manufacturer,omitempty"`
	Model            string `json:"model,omitempty"`
	HardwareID       string `json:"hardware_id,omitempty"`
	Hostname         string `json:"hostname,omitempty"`
	LastIP           string `json:"last_ip,omitempty"`
}

func Normalize(fp Fingerprint) Fingerprint {
	fp.MACAddress = normalizeMAC(fp.MACAddress)
	fp.ONVIFEndpointRef = clean(fp.ONVIFEndpointRef)
	fp.SerialNumber = clean(fp.SerialNumber)
	fp.Manufacturer = clean(fp.Manufacturer)
	fp.Model = clean(fp.Model)
	fp.HardwareID = clean(fp.HardwareID)
	fp.Hostname = clean(fp.Hostname)
	fp.LastIP = clean(fp.LastIP)
	return fp
}

func DeviceID(fp Fingerprint) string {
	fp = Normalize(fp)
	switch {
	case fp.Manufacturer != "" && fp.Model != "" && fp.SerialNumber != "":
		return hashID("serial", fp.Manufacturer+"|"+fp.Model+"|"+fp.SerialNumber)
	case fp.ONVIFEndpointRef != "":
		return hashID("onvif", fp.ONVIFEndpointRef)
	case fp.MACAddress != "":
		return hashID("mac", fp.MACAddress)
	default:
		var b [16]byte
		_, _ = rand.Read(b[:])
		return "device_" + hex.EncodeToString(b[:])
	}
}

// normalizeMAC canonicalizes a MAC address. Values that cannot be a real MAC
// (unresolved ARP entries like "<incomplete>", the all-zero placeholder, or
// arbitrary garbage) are mapped to "" so they never feed the device identity.
func normalizeMAC(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if hw, err := net.ParseMAC(value); err == nil {
		if hw.String() == zeroMAC {
			return ""
		}
		parts := strings.Split(hw.String(), ":")
		for i := range parts {
			parts[i] = strings.ToUpper(parts[i])
		}
		return strings.Join(parts, ":")
	}
	value = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(value, "-", ":"), ".", ""))
	if len(value) != 12 || strings.Contains(value, ":") || !isHex(value) {
		return ""
	}
	parts := make([]string, 0, 6)
	for i := 0; i < len(value); i += 2 {
		parts = append(parts, value[i:i+2])
	}
	normalized := strings.Join(parts, ":")
	if normalized == zeroMAC {
		return ""
	}
	return normalized
}

func isHex(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// ValidMAC reports whether the given normalized MAC is usable as an identity
// attribute. Callers can use it to filter ARP tables and discovery output.
func ValidMAC(normalized string) bool {
	if normalized == "" || normalized == zeroMAC {
		return false
	}
	hw, err := net.ParseMAC(normalized)
	if err != nil {
		return false
	}
	return hw.String() != zeroMAC && len(hw) == 6
}

func clean(value string) string {
	return strings.TrimSpace(value)
}

func hashID(prefix, value string) string {
	sum := sha256.Sum256([]byte(prefix + ":" + strings.ToLower(strings.TrimSpace(value))))
	return "device_" + hex.EncodeToString(sum[:12])
}

const zeroMAC = "00:00:00:00:00:00"
