package app

import (
	"context"
	"strings"
	"time"

	go2rtcrender "camera-appliance/camera-manager/internal/go2rtc"
	"camera-appliance/camera-manager/internal/state"
)

const (
	PathKindDirect = "direct"
	PathKindRelay  = "relay"

	PathPolicyAuto         = "auto"
	PathPolicyDirectOnly   = "direct_only"
	PathPolicyRelayOnly    = "relay_only"
	PathPolicyPreferDirect = "prefer_direct"
	PathPolicyPreferRelay  = "prefer_relay"

	relayIDsKey         = "camera.relay.ids"
	activePathKeyPrefix = "camera.active_path."
)

type RelayDefinition struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Host      string `json:"host"`
	BindHost  string `json:"bind_host,omitempty"`
	SSHTarget string `json:"ssh_target,omitempty"`
	AutoStart bool   `json:"auto_start"`
	Enabled   bool   `json:"enabled"`
}

type StreamPath struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	RelayID      string `json:"relay_id,omitempty"`
	Host         string `json:"host"`
	Port         string `json:"port"`
	ProbeHost    string `json:"probe_host,omitempty"`
	State        string `json:"state"`
	Message      string `json:"message"`
	Active       bool   `json:"active"`
	Selected     bool   `json:"selected"`
	LastSelected bool   `json:"last_selected"`
}

type StreamPathAssessment struct {
	Policy   string       `json:"policy"`
	Selected *StreamPath  `json:"selected,omitempty"`
	Paths    []StreamPath `json:"paths"`
}

func (a *App) streamEndpointSelections(ctx context.Context, bindings []state.Binding, settings map[string]string) (map[string]go2rtcrender.StreamEndpoint, map[string]StreamPathAssessment) {
	endpoints := map[string]go2rtcrender.StreamEndpoint{}
	assessments := map[string]StreamPathAssessment{}
	for _, binding := range bindings {
		if binding.DeviceID == "" || binding.Device == nil {
			continue
		}
		assessment := a.assessStreamPaths(ctx, binding, settings)
		assessments[binding.DeviceID] = assessment
		if assessment.Selected == nil {
			continue
		}
		endpoints[binding.DeviceID] = go2rtcrender.StreamEndpoint{Host: assessment.Selected.Host, Port: assessment.Selected.Port}
	}
	return endpoints, assessments
}

func (a *App) saveActiveStreamPaths(ctx context.Context, assessments map[string]StreamPathAssessment) error {
	values := map[string]string{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for deviceID, assessment := range assessments {
		if assessment.Selected == nil {
			continue
		}
		prefix := activePathKeyPrefix + deviceID + "."
		values[prefix+"id"] = assessment.Selected.ID
		values[prefix+"kind"] = assessment.Selected.Kind
		values[prefix+"relay_id"] = assessment.Selected.RelayID
		values[prefix+"host"] = assessment.Selected.Host
		values[prefix+"port"] = assessment.Selected.Port
		values[prefix+"checked_at"] = now
	}
	if len(values) == 0 {
		return nil
	}
	return a.Store.PutSettings(ctx, values)
}

func pathWarnings(bindings []state.Binding, assessments map[string]StreamPathAssessment) []string {
	var warnings []string
	for _, binding := range bindings {
		if binding.DeviceID == "" || binding.Device == nil {
			continue
		}
		assessment, ok := assessments[binding.DeviceID]
		if !ok || assessment.Selected != nil {
			continue
		}
		warnings = append(warnings, binding.SlotID+" ("+displayBindingLabel(binding)+") hat keinen erreichbaren RTSP-Pfad")
	}
	return warnings
}

func (a *App) assessStreamPaths(ctx context.Context, binding state.Binding, settings map[string]string) StreamPathAssessment {
	policy := pathPolicy(binding.DeviceID, settings)
	lastPathID := strings.TrimSpace(settings[activePathKeyPrefix+binding.DeviceID+".id"])
	paths := orderStreamPaths(streamPathCandidates(binding, settings), policy, lastPathID)
	assessment := StreamPathAssessment{Policy: policy, Paths: paths}
	for i := range assessment.Paths {
		assessment.Paths[i].LastSelected = assessment.Paths[i].ID == lastPathID
		assessment.Paths[i].ProbeHost = probeHostForEndpoint(assessment.Paths[i].Host)
		if err := a.probeRTSP(ctx, assessment.Paths[i].ProbeHost, assessment.Paths[i].Port); err != nil {
			assessment.Paths[i].State = "failed"
			assessment.Paths[i].Message = rtspProbeDiagnostic(assessment.Paths[i].Port, err)
			continue
		}
		assessment.Paths[i].State = "ok"
		assessment.Paths[i].Message = "RTSP-Pfad erreichbar."
		if assessment.Selected == nil {
			assessment.Paths[i].Selected = true
			assessment.Paths[i].Active = true
			selected := assessment.Paths[i]
			assessment.Selected = &selected
		}
	}
	return assessment
}

func streamPathCandidates(binding state.Binding, settings map[string]string) []StreamPath {
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
	for _, relay := range relayDefinitions(settings) {
		host, port := relayEndpoint(settings, binding.DeviceID, relay)
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

func relayDefinitions(settings map[string]string) []RelayDefinition {
	var relays []RelayDefinition
	for _, id := range settingList(settings[relayIDsKey]) {
		prefix := "camera.relay." + id + "."
		if settings[prefix+"enabled"] == "false" {
			continue
		}
		name := strings.TrimSpace(settings[prefix+"name"])
		if name == "" {
			name = id
		}
		relays = append(relays, RelayDefinition{
			ID:        id,
			Name:      name,
			Type:      relayType(settings[prefix+"type"]),
			Host:      strings.TrimSpace(settings[prefix+"host"]),
			BindHost:  relayBindHost(settings[prefix+"bind_host"]),
			SSHTarget: strings.TrimSpace(settings[prefix+"ssh_target"]),
			AutoStart: boolSetting(settings, prefix+"auto_start", false),
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

func relayBindHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "127.0.0.1"
	}
	return value
}

func relayEndpoint(settings map[string]string, deviceID string, relay RelayDefinition) (string, string) {
	prefix := "camera.relay_endpoint." + deviceID + "." + relay.ID + "."
	host := strings.TrimSpace(settings[prefix+"host"])
	if host == "" {
		host = relay.Host
	}
	port := strings.TrimSpace(settings[prefix+"port"])
	if port == "" {
		port = strings.TrimSpace(settings["camera.relay."+relay.ID+".default_port"])
	}
	if port == "" {
		return "", ""
	}
	return host, port
}

func orderStreamPaths(paths []StreamPath, policy, lastPathID string) []StreamPath {
	var out []StreamPath
	add := func(match func(StreamPath) bool) {
		for _, path := range paths {
			if match(path) && !containsPath(out, path.ID) {
				out = append(out, path)
			}
		}
	}
	if lastPathID != "" && policy == PathPolicyAuto {
		add(func(path StreamPath) bool { return path.ID == lastPathID })
	}
	switch policy {
	case PathPolicyDirectOnly:
		add(func(path StreamPath) bool { return path.Kind == PathKindDirect })
	case PathPolicyRelayOnly:
		add(func(path StreamPath) bool { return path.Kind == PathKindRelay })
	case PathPolicyPreferRelay:
		add(func(path StreamPath) bool { return path.Kind == PathKindRelay })
		add(func(path StreamPath) bool { return path.Kind == PathKindDirect })
	default:
		add(func(path StreamPath) bool { return path.Kind == PathKindDirect })
		add(func(path StreamPath) bool { return path.Kind == PathKindRelay })
	}
	return out
}

func containsPath(paths []StreamPath, id string) bool {
	for _, path := range paths {
		if path.ID == id {
			return true
		}
	}
	return false
}

func pathPolicy(deviceID string, settings map[string]string) string {
	policy := strings.TrimSpace(settings["camera.path_policy."+deviceID])
	switch policy {
	case PathPolicyAuto, PathPolicyDirectOnly, PathPolicyRelayOnly, PathPolicyPreferDirect, PathPolicyPreferRelay:
		return policy
	default:
		return PathPolicyAuto
	}
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

func displayBindingLabel(binding state.Binding) string {
	if strings.TrimSpace(binding.Label) != "" {
		return binding.Label
	}
	if binding.Slot != nil && strings.TrimSpace(binding.Slot.Label) != "" {
		return binding.Slot.Label
	}
	return binding.DeviceID
}
