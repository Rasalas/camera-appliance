package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/system"
)

const (
	watchdogEnabledKey                = "watchdog.enabled"
	watchdogFastIntervalKey           = "watchdog.fast_interval_seconds"
	watchdogCameraIntervalKey         = "watchdog.camera_interval_seconds"
	watchdogRestartOnChangeKey        = "watchdog.restart_on_change"
	watchdogRestartGo2RTCOnFailureKey = "watchdog.restart_go2rtc_on_failure"
	watchdogLastRunAtKey              = "watchdog.last_run_at"
	watchdogNextRunAtKey              = "watchdog.next_run_at"
	watchdogLastActionKey             = "watchdog.last_action"
	watchdogLastErrorKey              = "watchdog.last_error"

	defaultWatchdogFastIntervalSeconds   = 30
	defaultWatchdogCameraIntervalSeconds = 120
)

type WatchdogStatus struct {
	Enabled                bool   `json:"enabled"`
	FastIntervalSeconds    int    `json:"fast_interval_seconds"`
	CameraIntervalSeconds  int    `json:"camera_interval_seconds"`
	RestartOnChange        bool   `json:"restart_on_change"`
	RestartGo2RTCOnFailure bool   `json:"restart_go2rtc_on_failure"`
	LastRunAt              string `json:"last_run_at,omitempty"`
	NextRunAt              string `json:"next_run_at,omitempty"`
	LastAction             string `json:"last_action,omitempty"`
	LastError              string `json:"last_error,omitempty"`
}

type WatchdogRunResult struct {
	CheckedAt    time.Time            `json:"checked_at"`
	NextRunAt    time.Time            `json:"next_run_at"`
	CameraCheck  bool                 `json:"camera_check"`
	Go2RTCOnline bool                 `json:"go2rtc_online"`
	PathChanges  []WatchdogPathChange `json:"path_changes"`
	Action       string               `json:"action"`
	Error        string               `json:"error,omitempty"`
}

type WatchdogPathChange struct {
	SlotID   string `json:"slot_id"`
	DeviceID string `json:"device_id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Policy   string `json:"policy"`
}

type watchdogConfig struct {
	Enabled                bool
	FastInterval           time.Duration
	CameraInterval         time.Duration
	RestartOnChange        bool
	RestartGo2RTCOnFailure bool
}

func (a *App) RunWatchdog(ctx context.Context) {
	nextCameraCheck := time.Now().UTC()
	if !waitForWatchdog(ctx, 5*time.Second) {
		return
	}
	for {
		cfg := a.watchdogConfig(ctx)
		now := time.Now().UTC()
		runCameraCheck := cfg.Enabled && !now.Before(nextCameraCheck)
		nextRunAt := now.Add(cfg.FastInterval)
		if cfg.Enabled {
			if runCameraCheck {
				nextCameraCheck = now.Add(cfg.CameraInterval)
			}
			if nextCameraCheck.Before(nextRunAt) {
				nextRunAt = nextCameraCheck
			}
			result, err := a.RunWatchdogOnce(ctx, runCameraCheck)
			if err != nil {
				result.Error = err.Error()
			}
			result.NextRunAt = nextRunAt
			_ = a.saveWatchdogRun(ctx, result)
		} else {
			_ = a.saveWatchdogDisabled(ctx, nextRunAt)
		}
		if !waitForWatchdog(ctx, cfg.FastInterval) {
			return
		}
	}
}

func (a *App) RunWatchdogOnce(ctx context.Context, cameraCheck bool) (WatchdogRunResult, error) {
	cfg := a.watchdogConfig(ctx)
	now := time.Now().UTC()
	result := WatchdogRunResult{
		CheckedAt:   now,
		NextRunAt:   now.Add(cfg.FastInterval),
		CameraCheck: cameraCheck,
		Action:      "keine Aktion",
	}
	if !cfg.Enabled {
		result.Action = "deaktiviert"
		return result, nil
	}

	go2rtcStatus := system.Check(ctx, a.Config).Go2RTC
	result.Go2RTCOnline = go2rtcStatus.Online
	if !go2rtcStatus.Online && cfg.RestartGo2RTCOnFailure {
		if err := a.restartGo2RTC(ctx); err != nil {
			return result, fmt.Errorf("go2rtc restart failed: %w", err)
		}
		result.Action = "go2rtc neu gestartet"
		_ = a.Store.AddEvent(ctx, "warning", "watchdog.go2rtc_restarted", "Watchdog hat go2rtc neu gestartet", map[string]string{"reason": go2rtcStatus.Message})
	}

	if !cameraCheck {
		return result, nil
	}
	changes, err := a.watchdogCheckCameraPaths(ctx, cfg)
	if err != nil {
		return result, err
	}
	result.PathChanges = changes
	if len(changes) > 0 {
		result.Action = fmt.Sprintf("%d Pfadwechsel angewendet", len(changes))
	}
	return result, nil
}

func (a *App) WatchdogStatus(ctx context.Context) WatchdogStatus {
	settings, _ := a.Store.Settings(ctx)
	cfg := watchdogConfigFromSettings(settings)
	return WatchdogStatus{
		Enabled:                cfg.Enabled,
		FastIntervalSeconds:    int(cfg.FastInterval / time.Second),
		CameraIntervalSeconds:  int(cfg.CameraInterval / time.Second),
		RestartOnChange:        cfg.RestartOnChange,
		RestartGo2RTCOnFailure: cfg.RestartGo2RTCOnFailure,
		LastRunAt:              settings[watchdogLastRunAtKey],
		NextRunAt:              settings[watchdogNextRunAtKey],
		LastAction:             settings[watchdogLastActionKey],
		LastError:              settings[watchdogLastErrorKey],
	}
}

func (a *App) watchdogCheckCameraPaths(ctx context.Context, cfg watchdogConfig) ([]WatchdogPathChange, error) {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return nil, err
	}
	bindings = attachSlots(bindings, a.Slots)
	settings, _ := a.Store.Settings(ctx)
	_, assessments := a.streamEndpointSelections(ctx, bindings, settings)
	var changes []WatchdogPathChange
	for _, binding := range bindings {
		if binding.DeviceID == "" || binding.Device == nil {
			continue
		}
		assessment, ok := assessments[binding.DeviceID]
		if !ok || assessment.Selected == nil {
			continue
		}
		oldID := strings.TrimSpace(settings[activePathKeyPrefix+binding.DeviceID+".id"])
		if oldID == "" {
			continue
		}
		if oldID != assessment.Selected.ID {
			changes = append(changes, WatchdogPathChange{
				SlotID:   binding.SlotID,
				DeviceID: binding.DeviceID,
				From:     oldID,
				To:       assessment.Selected.ID,
				Policy:   assessment.Policy,
			})
		}
	}
	if err := a.saveActiveStreamPaths(ctx, assessments); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		return changes, err
	}
	if cfg.RestartOnChange {
		if err := a.restartGo2RTC(ctx); err != nil {
			return changes, err
		}
	}
	_ = a.Store.AddEvent(ctx, "warning", "watchdog.path_switched", "Watchdog hat aktive Kamera-Pfade gewechselt", map[string]any{"changes": changes, "restart": cfg.RestartOnChange})
	return changes, nil
}

func (a *App) restartGo2RTC(ctx context.Context) error {
	if a.Go2RTCRestart != nil {
		return a.Go2RTCRestart(ctx)
	}
	return system.RestartGo2RTC(ctx, a.Config)
}

func (a *App) watchdogConfig(ctx context.Context) watchdogConfig {
	settings, _ := a.Store.Settings(ctx)
	return watchdogConfigFromSettings(settings)
}

func watchdogConfigFromSettings(settings map[string]string) watchdogConfig {
	return watchdogConfig{
		Enabled:                boolSetting(settings, watchdogEnabledKey, true),
		FastInterval:           secondsSetting(settings, watchdogFastIntervalKey, defaultWatchdogFastIntervalSeconds, 5, 3600),
		CameraInterval:         secondsSetting(settings, watchdogCameraIntervalKey, defaultWatchdogCameraIntervalSeconds, 10, 7200),
		RestartOnChange:        boolSetting(settings, watchdogRestartOnChangeKey, true),
		RestartGo2RTCOnFailure: boolSetting(settings, watchdogRestartGo2RTCOnFailureKey, true),
	}
}

func (a *App) saveWatchdogRun(ctx context.Context, result WatchdogRunResult) error {
	values := map[string]string{
		watchdogLastRunAtKey:  result.CheckedAt.Format(time.RFC3339),
		watchdogNextRunAtKey:  result.NextRunAt.Format(time.RFC3339),
		watchdogLastActionKey: result.Action,
		watchdogLastErrorKey:  result.Error,
	}
	return a.Store.PutSettings(ctx, values)
}

func (a *App) saveWatchdogDisabled(ctx context.Context, nextRunAt time.Time) error {
	return a.Store.PutSettings(ctx, map[string]string{
		watchdogNextRunAtKey:  nextRunAt.Format(time.RFC3339),
		watchdogLastActionKey: "deaktiviert",
		watchdogLastErrorKey:  "",
	})
}

func boolSetting(settings map[string]string, key string, fallback bool) bool {
	raw := strings.TrimSpace(settings[key])
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1" || raw == "yes" || raw == "on"
}

func secondsSetting(settings map[string]string, key string, fallback, min, max int) time.Duration {
	value, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil || value <= 0 {
		value = fallback
	}
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	return time.Duration(value) * time.Second
}

func waitForWatchdog(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
