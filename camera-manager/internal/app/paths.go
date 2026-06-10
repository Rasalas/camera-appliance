package app

import (
	"context"
	"fmt"
	"strconv"
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
	pathStateKeyPrefix  = "camera.path_state."

	pathFailThresholdKey     = "camera.path.fail_threshold"
	pathRecoveryThresholdKey = "camera.path.recovery_threshold"

	defaultPathFailThreshold     = 2
	defaultPathRecoveryThreshold = 2

	// Auto-assigned relay forward ports: relay n uses base+20n, slot m adds m-1,
	// so cameras get relay paths without per-camera endpoint setup.
	relayPortBaseDefault = 18554
	relayPortBaseSpacing = 20
)

type RelayDefinition struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Host      string `json:"host"`
	BindHost  string `json:"bind_host,omitempty"`
	SSHTarget string `json:"ssh_target,omitempty"`
	PortBase  int    `json:"port_base"`
	AutoStart bool   `json:"auto_start"`
	Enabled   bool   `json:"enabled"`
}

type StreamPath struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Kind             string `json:"kind"`
	RelayID          string `json:"relay_id,omitempty"`
	Host             string `json:"host"`
	Port             string `json:"port"`
	ProbeHost        string `json:"probe_host,omitempty"`
	State            string `json:"state"`
	Message          string `json:"message"`
	Active           bool   `json:"active"`
	Selected         bool   `json:"selected"`
	LastSelected     bool   `json:"last_selected"`
	SuccessCount     int    `json:"success_count"`
	FailureCount     int    `json:"failure_count"`
	LastSuccessAt    string `json:"last_success_at,omitempty"`
	LastFailureAt    string `json:"last_failure_at,omitempty"`
	SelectedSince    string `json:"selected_since,omitempty"`
	LastSwitchAt     string `json:"last_switch_at,omitempty"`
	LastSwitchReason string `json:"last_switch_reason,omitempty"`
	Stability        string `json:"stability"`
	StabilityMessage string `json:"stability_message"`
}

type StreamPathAssessment struct {
	Policy       string       `json:"policy"`
	Selected     *StreamPath  `json:"selected,omitempty"`
	Paths        []StreamPath `json:"paths"`
	SwitchReason string       `json:"switch_reason,omitempty"`
}

type PathStabilityConfig struct {
	FailThreshold     int `json:"fail_threshold"`
	RecoveryThreshold int `json:"recovery_threshold"`
}

func (a *App) StreamEndpointForDevice(ctx context.Context, device state.Device) (go2rtcrender.StreamEndpoint, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return go2rtcrender.StreamEndpoint{}, err
	}
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return go2rtcrender.StreamEndpoint{}, err
	}
	var binding state.Binding
	for _, candidate := range bindings {
		if candidate.DeviceID == device.ID {
			binding = candidate
			break
		}
	}
	if binding.DeviceID == "" {
		binding = state.Binding{DeviceID: device.ID, Device: &device, Enabled: true}
	} else if binding.Device == nil {
		binding.Device = &device
	}
	endpoints, _ := a.streamEndpointSelections(ctx, []state.Binding{binding}, settings)
	if endpoint, ok := endpoints[device.ID]; ok {
		return endpoint, nil
	}
	return go2rtcrender.StreamEndpoint{Host: strings.TrimSpace(device.LastIP), Port: "554"}, nil
}

func (a *App) streamEndpointSelections(ctx context.Context, bindings []state.Binding, settings map[string]string) (map[string]go2rtcrender.StreamEndpoint, map[string]StreamPathAssessment) {
	endpoints, assessments, _ := a.streamEndpointSelectionsWithPathState(ctx, bindings, settings, false, time.Now().UTC())
	return endpoints, assessments
}

func (a *App) streamEndpointSelectionsWithPathState(ctx context.Context, bindings []state.Binding, settings map[string]string, updateState bool, checkedAt time.Time) (map[string]go2rtcrender.StreamEndpoint, map[string]StreamPathAssessment, map[string]string) {
	endpoints := map[string]go2rtcrender.StreamEndpoint{}
	assessments := map[string]StreamPathAssessment{}
	stateValues := map[string]string{}
	cfg := pathStabilityConfigFromSettings(settings)
	for _, binding := range bindings {
		if binding.DeviceID == "" || binding.Device == nil {
			continue
		}
		assessment, values := a.assessStreamPathsWithPathState(ctx, binding, settings, cfg, updateState, checkedAt)
		assessments[binding.DeviceID] = assessment
		for key, value := range values {
			stateValues[key] = value
		}
		if assessment.Selected == nil {
			continue
		}
		endpoints[binding.DeviceID] = go2rtcrender.StreamEndpoint{Host: assessment.Selected.Host, Port: assessment.Selected.Port}
	}
	return endpoints, assessments, stateValues
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
		if assessment.Selected.SelectedSince != "" {
			values[prefix+"selected_since"] = assessment.Selected.SelectedSince
		}
		if assessment.Selected.LastSwitchAt != "" {
			values[prefix+"last_switch_at"] = assessment.Selected.LastSwitchAt
		}
		if assessment.Selected.LastSwitchReason != "" {
			values[prefix+"last_switch_reason"] = assessment.Selected.LastSwitchReason
		}
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
	assessment, _ := a.assessStreamPathsWithPathState(ctx, binding, settings, pathStabilityConfigFromSettings(settings), false, time.Now().UTC())
	return assessment
}

func (a *App) assessStreamPathsWithPathState(ctx context.Context, binding state.Binding, settings map[string]string, cfg PathStabilityConfig, updateState bool, checkedAt time.Time) (StreamPathAssessment, map[string]string) {
	policy := pathPolicy(binding.DeviceID, settings)
	lastPathID := strings.TrimSpace(settings[activePathKeyPrefix+binding.DeviceID+".id"])
	selectedSince := strings.TrimSpace(settings[activePathKeyPrefix+binding.DeviceID+".selected_since"])
	lastSwitchAt := strings.TrimSpace(settings[activePathKeyPrefix+binding.DeviceID+".last_switch_at"])
	lastSwitchReason := strings.TrimSpace(settings[activePathKeyPrefix+binding.DeviceID+".last_switch_reason"])
	paths := orderStreamPaths(streamPathCandidates(binding, settings), policy, lastPathID)
	assessment := StreamPathAssessment{Policy: policy, Paths: paths}
	stateValues := map[string]string{}
	for i := range assessment.Paths {
		assessment.Paths[i].LastSelected = assessment.Paths[i].ID == lastPathID
		assessment.Paths[i].Active = assessment.Paths[i].LastSelected
		assessment.Paths[i].ProbeHost = ProbeHostForEndpoint(assessment.Paths[i].Host)
		if err := a.probeRTSP(ctx, assessment.Paths[i].ProbeHost, assessment.Paths[i].Port); err != nil {
			assessment.Paths[i].State = "failed"
			assessment.Paths[i].Message = rtspProbeDiagnostic(assessment.Paths[i].Port, err)
		} else {
			assessment.Paths[i].State = "ok"
			assessment.Paths[i].Message = "RTSP-Pfad erreichbar."
		}
		counters := streamPathCountersFromSettings(settings, binding.DeviceID, assessment.Paths[i].ID)
		if updateState {
			counters = updateStreamPathCounters(counters, assessment.Paths[i].State, checkedAt)
			for key, value := range streamPathCounterValues(binding.DeviceID, assessment.Paths[i].ID, counters) {
				stateValues[key] = value
			}
		}
		applyStreamPathCounters(&assessment.Paths[i], counters)
	}
	selectedIndex, switchReason := selectStableStreamPath(assessment.Paths, lastPathID, cfg)
	annotateStreamPathStability(assessment.Paths, lastPathID, cfg)
	if selectedIndex >= 0 {
		assessment.SwitchReason = switchReason
		if switchReason != "" {
			lastSwitchAt = checkedAt.Format(time.RFC3339)
			lastSwitchReason = switchReason
			selectedSince = lastSwitchAt
		} else if selectedSince == "" {
			selectedSince = checkedAt.Format(time.RFC3339)
		}
		assessment.Paths[selectedIndex].Selected = true
		assessment.Paths[selectedIndex].SelectedSince = selectedSince
		assessment.Paths[selectedIndex].LastSwitchAt = lastSwitchAt
		assessment.Paths[selectedIndex].LastSwitchReason = lastSwitchReason
		selected := assessment.Paths[selectedIndex]
		assessment.Selected = &selected
	}
	return assessment, stateValues
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
		host, port := relayEndpoint(settings, binding, relay)
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
	for index, id := range settingList(settings[relayIDsKey]) {
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
			BindHost:  relayBindHost(settings[prefix+"bind_host"]),
			SSHTarget: sshTarget,
			PortBase:  intSetting(settings, prefix+"port_base", relayPortBaseDefault+relayPortBaseSpacing*index, 1024, 65000),
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

func relayBindHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "127.0.0.1"
	}
	return value
}

func relayEndpoint(settings map[string]string, binding state.Binding, relay RelayDefinition) (string, string) {
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
		port = relayAutoPort(relay, binding.SlotID)
	}
	if port == "" {
		return "", ""
	}
	return host, port
}

// relayAutoPort derives a stable local forward port from the camera's slot
// (cam1 → PortBase, cam2 → PortBase+1, …); without a slot there is no auto port.
func relayAutoPort(relay RelayDefinition, slotID string) string {
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

type streamPathCounters struct {
	SuccessCount  int
	FailureCount  int
	LastSuccessAt string
	LastFailureAt string
}

func pathStabilityConfigFromSettings(settings map[string]string) PathStabilityConfig {
	return PathStabilityConfig{
		FailThreshold:     intSetting(settings, pathFailThresholdKey, defaultPathFailThreshold, 1, 20),
		RecoveryThreshold: intSetting(settings, pathRecoveryThresholdKey, defaultPathRecoveryThreshold, 1, 20),
	}
}

func streamPathCountersFromSettings(settings map[string]string, deviceID, pathID string) streamPathCounters {
	return streamPathCounters{
		SuccessCount:  intSetting(settings, pathStateKey(deviceID, pathID, "success_count"), 0, 0, 1000000),
		FailureCount:  intSetting(settings, pathStateKey(deviceID, pathID, "failure_count"), 0, 0, 1000000),
		LastSuccessAt: strings.TrimSpace(settings[pathStateKey(deviceID, pathID, "last_success_at")]),
		LastFailureAt: strings.TrimSpace(settings[pathStateKey(deviceID, pathID, "last_failure_at")]),
	}
}

func updateStreamPathCounters(counters streamPathCounters, state string, checkedAt time.Time) streamPathCounters {
	if state == "ok" {
		counters.SuccessCount++
		counters.FailureCount = 0
		counters.LastSuccessAt = checkedAt.Format(time.RFC3339)
		return counters
	}
	counters.FailureCount++
	counters.SuccessCount = 0
	counters.LastFailureAt = checkedAt.Format(time.RFC3339)
	return counters
}

func streamPathCounterValues(deviceID, pathID string, counters streamPathCounters) map[string]string {
	values := map[string]string{
		pathStateKey(deviceID, pathID, "success_count"): strconv.Itoa(counters.SuccessCount),
		pathStateKey(deviceID, pathID, "failure_count"): strconv.Itoa(counters.FailureCount),
	}
	if counters.LastSuccessAt != "" {
		values[pathStateKey(deviceID, pathID, "last_success_at")] = counters.LastSuccessAt
	}
	if counters.LastFailureAt != "" {
		values[pathStateKey(deviceID, pathID, "last_failure_at")] = counters.LastFailureAt
	}
	return values
}

func applyStreamPathCounters(path *StreamPath, counters streamPathCounters) {
	path.SuccessCount = counters.SuccessCount
	path.FailureCount = counters.FailureCount
	path.LastSuccessAt = counters.LastSuccessAt
	path.LastFailureAt = counters.LastFailureAt
}

func selectStableStreamPath(paths []StreamPath, activePathID string, cfg PathStabilityConfig) (int, string) {
	firstOK := firstPathIndex(paths, func(path StreamPath) bool { return path.State == "ok" })
	activeIndex := pathIndex(paths, activePathID)
	if activePathID == "" {
		if firstOK >= 0 {
			return firstOK, "initial_selection"
		}
		return -1, ""
	}
	if activeIndex < 0 {
		if firstOK >= 0 {
			return firstOK, "policy_selected"
		}
		return -1, ""
	}
	active := paths[activeIndex]
	if active.State != "ok" {
		if active.FailureCount < cfg.FailThreshold {
			return activeIndex, ""
		}
		if firstOK >= 0 && firstOK != activeIndex {
			return firstOK, fmt.Sprintf("active_failed_%d", cfg.FailThreshold)
		}
		return activeIndex, ""
	}
	preferredIndex := firstPathIndex(paths, func(path StreamPath) bool { return path.State == "ok" })
	if preferredIndex >= 0 && preferredIndex != activeIndex && preferredIndex < activeIndex {
		if paths[preferredIndex].SuccessCount >= cfg.RecoveryThreshold {
			return preferredIndex, fmt.Sprintf("preferred_recovered_%d", cfg.RecoveryThreshold)
		}
	}
	return activeIndex, ""
}

func annotateStreamPathStability(paths []StreamPath, activePathID string, cfg PathStabilityConfig) {
	activeIndex := pathIndex(paths, activePathID)
	for i := range paths {
		path := &paths[i]
		switch {
		case path.ID == activePathID && path.State == "ok":
			path.Stability = "stable"
			path.StabilityMessage = "Aktiver Pfad ist stabil."
		case path.ID == activePathID && path.State != "ok" && path.FailureCount < cfg.FailThreshold:
			path.Stability = "failing"
			path.StabilityMessage = fmt.Sprintf("Aktiver Pfad wird gehalten: %d/%d Fehler.", path.FailureCount, cfg.FailThreshold)
		case path.ID == activePathID && path.State != "ok":
			path.Stability = "unstable"
			path.StabilityMessage = fmt.Sprintf("Aktiver Pfad hat die Fehlerschwelle erreicht: %d/%d.", path.FailureCount, cfg.FailThreshold)
		case path.State == "ok" && activeIndex >= 0 && i < activeIndex && path.SuccessCount < cfg.RecoveryThreshold:
			path.Stability = "warming"
			path.StabilityMessage = fmt.Sprintf("Pfad wartet auf stabile Erholung: %d/%d Erfolge.", path.SuccessCount, cfg.RecoveryThreshold)
		case path.State == "ok":
			path.Stability = "stable"
			path.StabilityMessage = "Pfad ist erreichbar."
		default:
			path.Stability = "failed"
			path.StabilityMessage = "Pfad ist nicht erreichbar."
		}
	}
}

func firstPathIndex(paths []StreamPath, match func(StreamPath) bool) int {
	for i, path := range paths {
		if match(path) {
			return i
		}
	}
	return -1
}

func pathIndex(paths []StreamPath, id string) int {
	if id == "" {
		return -1
	}
	for i, path := range paths {
		if path.ID == id {
			return i
		}
	}
	return -1
}

func pathStateKey(deviceID, pathID, name string) string {
	return pathStateKeyPrefix + safePathStateID(deviceID) + "." + safePathStateID(pathID) + "." + name
}

func safePathStateID(value string) string {
	var out strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('_')
	}
	cleaned := strings.Trim(out.String(), "_-")
	if cleaned == "" {
		return "path"
	}
	return cleaned
}

func intSetting(settings map[string]string, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(settings[key])
	value, err := strconv.Atoi(raw)
	if err != nil {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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
