package streamrouting

import (
	"strconv"
	"strings"

	"camera-appliance/camera-manager/internal/state"
)

func Candidates(binding state.Binding, settings map[string]string) []StreamPath {
	if binding.Device == nil {
		return nil
	}
	var paths []StreamPath
	if strings.TrimSpace(binding.Device.LastIP) != "" {
		paths = append(paths, StreamPath{
			ID:    "direct",
			Label: "Direkt",
			Kind:  PathKindDirect,
			Host:  strings.TrimSpace(binding.Device.LastIP),
			Port:  "554",
		})
	}
	for _, relay := range Relays(settings) {
		host, port := RelayEndpoint(settings, binding, relay)
		if host == "" || port == "" {
			continue
		}
		paths = append(paths, StreamPath{
			ID:      "relay:" + relay.ID,
			Label:   relay.Name,
			Kind:    PathKindRelay,
			RelayID: relay.ID,
			Host:    host,
			Port:    port,
		})
	}
	if host := strings.TrimSpace(settings["camera.rtsp_endpoint."+binding.DeviceID+".host"]); host != "" {
		port := strings.TrimSpace(settings["camera.rtsp_endpoint."+binding.DeviceID+".port"])
		if port == "" {
			port = "554"
		}
		paths = append(paths, StreamPath{
			ID:      "relay:manual",
			Label:   "Manueller Relay",
			Kind:    PathKindRelay,
			RelayID: "manual",
			Host:    host,
			Port:    port,
		})
	}
	return paths
}

func Relays(settings map[string]string) []RelayDefinition {
	var relays []RelayDefinition
	for index, id := range settingList(settings[RelayIDsKey]) {
		prefix := "camera.relay." + id + "."
		if settings[prefix+"enabled"] == "false" {
			continue
		}
		name := strings.TrimSpace(settings[prefix+"name"])
		if name == "" {
			name = id
		}
		sshTarget := strings.TrimSpace(settings[prefix+"ssh_target"])
		if sshTarget == "" {
			sshTarget = id
		}
		relays = append(relays, RelayDefinition{
			ID:        id,
			Name:      name,
			Type:      relayType(settings[prefix+"type"]),
			Host:      strings.TrimSpace(settings[prefix+"host"]),
			BindHost:  RelayBindHost(settings[prefix+"bind_host"]),
			SSHTarget: sshTarget,
			PortBase:  intSetting(settings, prefix+"port_base", RelayPortBaseDefault+relayPortBaseSpacing*index, 1024, 65000),
			AutoStart: boolSetting(settings, prefix+"auto_start", true),
			Enabled:   true,
		})
	}
	return relays
}

func relayType(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return RelayTypeSSHLocalForward
	}
	return value
}

func RelayBindHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "127.0.0.1"
	}
	return value
}

func RelayEndpoint(settings map[string]string, binding state.Binding, relay RelayDefinition) (string, string) {
	prefix := "camera.relay_endpoint." + binding.DeviceID + "." + relay.ID + "."
	host := strings.TrimSpace(settings[prefix+"host"])
	if host == "" {
		host = relay.Host
	}
	port := strings.TrimSpace(settings[prefix+"port"])
	if port == "" {
		port = strings.TrimSpace(settings["camera.relay."+relay.ID+".default_port"])
	}
	if port == "" {
		port = RelayAutoPort(relay, binding.SlotID)
	}
	if port == "" {
		return "", ""
	}
	return host, port
}

// RelayAutoPort derives a stable local forward port from the camera's slot
// (cam1 → PortBase, cam2 → PortBase+1, …); without a slot there is no auto port.
func RelayAutoPort(relay RelayDefinition, slotID string) string {
	slot := slotNumber(slotID)
	if slot <= 0 || relay.PortBase <= 0 {
		return ""
	}
	port := relay.PortBase + slot - 1
	if port > 65535 {
		return ""
	}
	return strconv.Itoa(port)
}

func slotNumber(slotID string) int {
	trimmed := strings.TrimSpace(slotID)
	start := len(trimmed)
	for start > 0 && trimmed[start-1] >= '0' && trimmed[start-1] <= '9' {
		start--
	}
	if start == len(trimmed) {
		return 0
	}
	number, err := strconv.Atoi(trimmed[start:])
	if err != nil || number <= 0 {
		return 0
	}
	return number
}

func settingList(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func boolSetting(settings map[string]string, key string, fallback bool) bool {
	raw := strings.TrimSpace(settings[key])
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1" || raw == "yes" || raw == "on"
}

const RelayTypeSSHLocalForward = "ssh_local_forward"
