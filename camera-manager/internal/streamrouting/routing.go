package streamrouting

import (
	"context"
	"strings"
	"time"
)

const (
	PathKindDirect = "direct"
	PathKindRelay  = "relay"

	PathPolicyAuto         = "auto"
	PathPolicyDirectOnly   = "direct_only"
	PathPolicyRelayOnly    = "relay_only"
	PathPolicyPreferDirect = "prefer_direct"
	PathPolicyPreferRelay  = "prefer_relay"

	RelayIDsKey         = "camera.relay.ids"
	ActivePathKeyPrefix = "camera.active_path."
	pathStateKeyPrefix  = "camera.path_state."

	PathFailThresholdKey     = "camera.path.fail_threshold"
	PathRecoveryThresholdKey = "camera.path.recovery_threshold"

	defaultPathFailThreshold     = 2
	defaultPathRecoveryThreshold = 2

	// Auto-assigned relay forward ports: relay n uses base+20n, slot m adds m-1,
	// so cameras get relay paths without per-camera endpoint setup.
	RelayPortBaseDefault = 18554
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

func Assess(ctx context.Context, input Input, probe func(context.Context, string, string) error) (StreamPathAssessment, map[string]string) {
	settings, updateState, checkedAt := input.Settings, input.UpdateState, input.CheckedAt
	cfg := StabilityConfig(settings)
	policy := Policy(input.DeviceID, settings)
	lastPathID := strings.TrimSpace(settings[ActivePathKeyPrefix+input.DeviceID+".id"])
	selectedSince := strings.TrimSpace(settings[ActivePathKeyPrefix+input.DeviceID+".selected_since"])
	lastSwitchAt := strings.TrimSpace(settings[ActivePathKeyPrefix+input.DeviceID+".last_switch_at"])
	lastSwitchReason := strings.TrimSpace(settings[ActivePathKeyPrefix+input.DeviceID+".last_switch_reason"])
	paths := orderStreamPaths(input.Paths, policy, lastPathID)
	assessment := StreamPathAssessment{Policy: policy, Paths: paths}
	stateValues := map[string]string{}
	for i := range assessment.Paths {
		assessment.Paths[i].LastSelected = assessment.Paths[i].ID == lastPathID
		assessment.Paths[i].Active = assessment.Paths[i].LastSelected
		assessment.Paths[i].ProbeHost = ProbeHostForEndpoint(assessment.Paths[i].Host)
		if err := probe(ctx, assessment.Paths[i].ProbeHost, assessment.Paths[i].Port); err != nil {
			assessment.Paths[i].State = "failed"
			assessment.Paths[i].Message = ProbeDiagnostic(assessment.Paths[i].Port, err)
		} else {
			assessment.Paths[i].State = "ok"
			assessment.Paths[i].Message = "RTSP-Pfad erreichbar."
		}
		counters := streamPathCountersFromSettings(settings, input.DeviceID, assessment.Paths[i].ID)
		if updateState {
			counters = updateStreamPathCounters(counters, assessment.Paths[i].State, checkedAt)
			for key, value := range streamPathCounterValues(input.DeviceID, assessment.Paths[i].ID, counters) {
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

// Input describes one camera's candidate paths and persisted observations.
// Assess returns proposed runtime values; only the caller decides to persist them.
type Input struct {
	DeviceID    string
	Paths       []StreamPath
	Settings    map[string]string
	UpdateState bool
	CheckedAt   time.Time
}
