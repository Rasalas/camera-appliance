package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	go2rtcrender "camera-appliance/camera-manager/internal/go2rtc"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/streamrouting"
)

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

func (a *App) streamEndpointSelections(ctx context.Context, bindings []state.Binding, settings map[string]string) (map[string]go2rtcrender.StreamEndpoint, map[string]streamrouting.StreamPathAssessment) {
	endpoints, assessments, _ := a.streamEndpointSelectionsWithPathState(ctx, bindings, settings, false, time.Now().UTC())
	return endpoints, assessments
}

func (a *App) streamEndpointSelectionsWithPathState(ctx context.Context, bindings []state.Binding, settings map[string]string, updateState bool, checkedAt time.Time) (map[string]go2rtcrender.StreamEndpoint, map[string]streamrouting.StreamPathAssessment, map[string]string) {
	endpoints := map[string]go2rtcrender.StreamEndpoint{}
	assessments := map[string]streamrouting.StreamPathAssessment{}
	stateValues := map[string]string{}
	for _, binding := range bindings {
		if binding.DeviceID == "" || binding.Device == nil {
			continue
		}
		assessment, values := streamrouting.Assess(ctx, streamrouting.Input{DeviceID: binding.DeviceID, Paths: streamrouting.Candidates(binding, settings), Settings: settings, UpdateState: updateState, CheckedAt: checkedAt}, a.probeRTSP)
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

func (a *App) saveActiveStreamPaths(ctx context.Context, assessments map[string]streamrouting.StreamPathAssessment) error {
	values := map[string]string{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for deviceID, assessment := range assessments {
		if assessment.Selected == nil {
			continue
		}
		prefix := streamrouting.ActivePathKeyPrefix + deviceID + "."
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

func pathWarnings(bindings []state.Binding, assessments map[string]streamrouting.StreamPathAssessment) []string {
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

func (a *App) assessStreamPaths(ctx context.Context, binding state.Binding, settings map[string]string) streamrouting.StreamPathAssessment {
	assessment, _ := streamrouting.Assess(ctx, streamrouting.Input{DeviceID: binding.DeviceID, Paths: streamrouting.Candidates(binding, settings), Settings: settings, CheckedAt: time.Now().UTC()}, a.probeRTSP)
	return assessment
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
