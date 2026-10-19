package app

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
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

// viewerProbeCacheTTL keeps live fallback probes from repeating on every viewer poll.
const viewerProbeCacheTTL = 10 * time.Second

var (
	errWatchdogPathFailed = errors.New("watchdog: RTSP path unreachable")
	errViewerProbePending = errors.New("viewer: probe pending")
)

type viewerProbeCache struct {
	// mu is held for a whole probe batch so concurrent viewer requests reuse its results.
	mu      sync.Mutex
	entries map[string]viewerProbeResult
}

type viewerProbeResult struct {
	err error
	at  time.Time
}

// viewerPathAssessments reads watchdog observations and probes only paths without a fresh one.
// It never returns state values, so viewer reads cannot advance watchdog counters.
func (a *App) viewerPathAssessments(ctx context.Context, bindings []state.Binding, settings map[string]string, now time.Time) map[string]streamrouting.StreamPathAssessment {
	type pendingAssessment struct {
		input    streamrouting.Input
		observed map[string]error
	}
	maxAge := viewerObservationMaxAge(settings)
	assessments := map[string]streamrouting.StreamPathAssessment{}
	missing := map[string]bool{}
	var pending []pendingAssessment
	for _, binding := range bindings {
		if binding.SlotID == "" || binding.DeviceID == "" || !binding.Enabled || binding.Device == nil || strings.TrimSpace(binding.Device.LastIP) == "" {
			continue
		}
		input := streamrouting.Input{DeviceID: binding.DeviceID, Paths: streamrouting.Candidates(binding, settings), Settings: settings, CheckedAt: now}
		observed := watchdogObservations(input, maxAge, now)
		complete := true
		assessment, _ := streamrouting.Assess(ctx, input, func(_ context.Context, host, port string) error {
			key := net.JoinHostPort(host, port)
			if err, ok := observed[key]; ok {
				return err
			}
			missing[key] = true
			complete = false
			return errViewerProbePending
		})
		if complete {
			assessments[binding.DeviceID] = assessment
			continue
		}
		pending = append(pending, pendingAssessment{input: input, observed: observed})
	}
	if len(pending) == 0 {
		return assessments
	}
	live := a.viewerProbe(ctx, missing)
	for _, item := range pending {
		assessment, _ := streamrouting.Assess(ctx, item.input, func(_ context.Context, host, port string) error {
			key := net.JoinHostPort(host, port)
			if err, ok := item.observed[key]; ok {
				return err
			}
			return live[key]
		})
		assessments[item.input.DeviceID] = assessment
	}
	return assessments
}

// viewerObservationMaxAge treats watchdog results as current for three camera check intervals.
func viewerObservationMaxAge(settings map[string]string) time.Duration {
	cfg := watchdogConfigFromSettings(settings)
	if !cfg.Enabled {
		return 0
	}
	return 3 * cfg.CameraInterval
}

// watchdogObservations maps probe host:port to the last persisted result of each fresh path.
func watchdogObservations(input streamrouting.Input, maxAge time.Duration, now time.Time) map[string]error {
	observed := map[string]error{}
	if maxAge <= 0 {
		return observed
	}
	for _, path := range input.Paths {
		observation, ok := streamrouting.LastObservation(input.Settings, input.DeviceID, path.ID)
		if !ok || now.Sub(observation.At) > maxAge {
			continue
		}
		key := net.JoinHostPort(streamrouting.ProbeHostForEndpoint(path.Host), path.Port)
		if observation.OK {
			observed[key] = nil
		} else {
			observed[key] = errWatchdogPathFailed
		}
	}
	return observed
}

// viewerProbe dials the given host:port keys concurrently, reusing results younger than viewerProbeCacheTTL.
func (a *App) viewerProbe(ctx context.Context, keys map[string]bool) map[string]error {
	c := &a.viewerProbes
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	results := make(map[string]error, len(keys))
	var toProbe []string
	for key := range keys {
		if entry, ok := c.entries[key]; ok && now.Sub(entry.at) < viewerProbeCacheTTL {
			results[key] = entry.err
			continue
		}
		toProbe = append(toProbe, key)
	}
	probed := make([]error, len(toProbe))
	var wg sync.WaitGroup
	for i, key := range toProbe {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			host, port, err := net.SplitHostPort(key)
			if err != nil {
				probed[i] = err
				return
			}
			probed[i] = a.probeRTSP(ctx, host, port)
		}(i, key)
	}
	wg.Wait()
	if c.entries == nil {
		c.entries = map[string]viewerProbeResult{}
	}
	for key, entry := range c.entries {
		if now.Sub(entry.at) >= viewerProbeCacheTTL {
			delete(c.entries, key)
		}
	}
	for i, key := range toProbe {
		results[key] = probed[i]
		// A canceled request says nothing about the camera.
		if ctx.Err() == nil {
			c.entries[key] = viewerProbeResult{err: probed[i], at: now}
		}
	}
	return results
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
