package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/streamrouting"
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
	watchdogPathRestartLastAtKey      = "watchdog.path_restart.last_at"
	watchdogPathRestartPendingKey     = "watchdog.path_restart.pending"
	watchdogPathRestartPendingReason  = "watchdog.path_restart.pending_reason"
	watchdogPathRestartFailuresKey    = "watchdog.path_restart.failures"
	watchdogPathRestartRetryAtKey     = "watchdog.path_restart.retry_at"
	watchdogGo2RTCHealthFailuresKey   = "watchdog.go2rtc.health_failures"
	watchdogGo2RTCRestartAttemptsKey  = "watchdog.go2rtc.restart_attempts"
	watchdogGo2RTCNextRestartAtKey    = "watchdog.go2rtc.next_restart_at"
	pathRestartCooldownSecondsKey     = "camera.path.restart_cooldown_seconds"

	defaultWatchdogFastIntervalSeconds   = 30
	defaultWatchdogCameraIntervalSeconds = 120
	defaultPathRestartCooldownSeconds    = 120

	go2rtcHealthFailureThreshold = 3
	go2rtcRestartBackoffBase     = time.Minute
	go2rtcRestartBackoffMax      = 10 * time.Minute
)

type WatchdogStatus struct {
	Enabled                bool   `json:"enabled"`
	FastIntervalSeconds    int    `json:"fast_interval_seconds"`
	CameraIntervalSeconds  int    `json:"camera_interval_seconds"`
	RestartOnChange        bool   `json:"restart_on_change"`
	RestartGo2RTCOnFailure bool   `json:"restart_go2rtc_on_failure"`
	PathFailThreshold      int    `json:"path_fail_threshold"`
	PathRecoveryThreshold  int    `json:"path_recovery_threshold"`
	PathRestartCooldownSec int    `json:"path_restart_cooldown_seconds"`
	PathRestartLastAt      string `json:"path_restart_last_at,omitempty"`
	PathRestartPending     bool   `json:"path_restart_pending"`
	PathRestartCooldownTo  string `json:"path_restart_cooldown_until,omitempty"`
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
	Reason   string `json:"reason,omitempty"`
}

type watchdogConfig struct {
	Enabled                bool
	FastInterval           time.Duration
	CameraInterval         time.Duration
	RestartOnChange        bool
	RestartGo2RTCOnFailure bool
	PathRestartCooldown    time.Duration
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
				result.Error = redaction.Text(err.Error())
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

	var errs []error
	relayStatuses, relayErr := a.Relays().Ensure(ctx)
	if relayErr != nil {
		result.Error = redaction.Text(relayErr.Error())
		errs = append(errs, relayErr)
	}
	startedRelays := 0
	for _, status := range relayStatuses {
		if status.Started {
			startedRelays++
		}
	}
	if startedRelays > 0 {
		result.Action = fmt.Sprintf("%d Relay-Prozess(e) gestartet", startedRelays)
	}

	// Errors are collected so one failing step never skips the later checks.
	go2rtcStatus := system.Go2RTCStatus(ctx, a.Config)
	result.Go2RTCOnline = go2rtcStatus.Online
	if restarted, err := a.superviseGo2RTC(ctx, cfg, go2rtcStatus, now); err != nil {
		errs = append(errs, err)
	} else if restarted {
		result.Action = "go2rtc neu gestartet"
	}

	if restarted, err := a.restartPendingPathChange(ctx, cfg, now); err != nil {
		errs = append(errs, err)
	} else if restarted && result.Action == "keine Aktion" {
		result.Action = "go2rtc nach Cooldown neu gestartet"
	}

	if cameraCheck {
		changes, err := a.watchdogCheckCameraPaths(ctx, cfg)
		if err != nil {
			errs = append(errs, err)
		}
		result.PathChanges = changes
		if len(changes) > 0 {
			result.Action = fmt.Sprintf("%d Pfadwechsel angewendet", len(changes))
		}
	}
	return result, errors.Join(errs...)
}

// superviseGo2RTC restarts go2rtc only after consecutive failed health checks
// and backs off exponentially while it stays unhealthy.
func (a *App) superviseGo2RTC(ctx context.Context, cfg watchdogConfig, status system.ServiceStatus, now time.Time) (bool, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return false, err
	}
	if status.Online {
		if counterSetting(settings, watchdogGo2RTCHealthFailuresKey) == 0 && counterSetting(settings, watchdogGo2RTCRestartAttemptsKey) == 0 {
			return false, nil
		}
		return false, a.Store.PutSettings(ctx, map[string]string{
			watchdogGo2RTCHealthFailuresKey:  "0",
			watchdogGo2RTCRestartAttemptsKey: "0",
			watchdogGo2RTCNextRestartAtKey:   "",
		})
	}
	failures := counterSetting(settings, watchdogGo2RTCHealthFailuresKey) + 1
	values := map[string]string{watchdogGo2RTCHealthFailuresKey: strconv.Itoa(failures)}
	due := cfg.RestartGo2RTCOnFailure && failures >= go2rtcHealthFailureThreshold
	if nextAt, ok := parseSettingTime(settings[watchdogGo2RTCNextRestartAtKey]); ok && now.Before(nextAt) {
		due = false
	}
	if !due {
		return false, a.Store.PutSettings(ctx, values)
	}
	attempts := counterSetting(settings, watchdogGo2RTCRestartAttemptsKey) + 1
	nextAt := now.Add(restartBackoff(attempts)).Format(time.RFC3339)
	values[watchdogGo2RTCRestartAttemptsKey] = strconv.Itoa(attempts)
	values[watchdogGo2RTCNextRestartAtKey] = nextAt
	// Persist the backoff first so a restart that hangs or kills us still counts.
	if err := a.Store.PutSettings(ctx, values); err != nil {
		return false, err
	}
	a.go2rtcMu.Lock()
	err = a.restartGo2RTC(ctx)
	a.go2rtcMu.Unlock()
	details := map[string]string{
		"reason":          redaction.Text(status.Message),
		"failures":        strconv.Itoa(failures),
		"attempt":         strconv.Itoa(attempts),
		"next_restart_at": nextAt,
	}
	if err != nil {
		details["error"] = redaction.Text(err.Error())
		_ = a.Store.AddEvent(ctx, "error", "watchdog.go2rtc_restart_failed", "Watchdog konnte go2rtc nicht neu starten", details)
		return false, fmt.Errorf("go2rtc restart failed: %w", err)
	}
	_ = a.Store.AddEvent(ctx, "warning", "watchdog.go2rtc_restarted", "Watchdog hat go2rtc neu gestartet", details)
	return true, nil
}

func restartBackoff(attempts int) time.Duration {
	delay := go2rtcRestartBackoffBase
	for i := 1; i < attempts && delay < go2rtcRestartBackoffMax; i++ {
		delay *= 2
	}
	return min(delay, go2rtcRestartBackoffMax)
}

func (a *App) WatchdogStatus(ctx context.Context) WatchdogStatus {
	settings, _ := a.Store.Settings(ctx)
	cfg := watchdogConfigFromSettings(settings)
	pathCfg := streamrouting.StabilityConfig(settings)
	cooldownUntil := ""
	if lastAt, ok := parseSettingTime(settings[watchdogPathRestartLastAtKey]); ok && cfg.PathRestartCooldown > 0 {
		cooldownUntil = lastAt.Add(cfg.PathRestartCooldown).Format(time.RFC3339)
	}
	return WatchdogStatus{
		Enabled:                cfg.Enabled,
		FastIntervalSeconds:    int(cfg.FastInterval / time.Second),
		CameraIntervalSeconds:  int(cfg.CameraInterval / time.Second),
		RestartOnChange:        cfg.RestartOnChange,
		RestartGo2RTCOnFailure: cfg.RestartGo2RTCOnFailure,
		PathFailThreshold:      pathCfg.FailThreshold,
		PathRecoveryThreshold:  pathCfg.RecoveryThreshold,
		PathRestartCooldownSec: int(cfg.PathRestartCooldown / time.Second),
		PathRestartLastAt:      settings[watchdogPathRestartLastAtKey],
		PathRestartPending:     boolSetting(settings, watchdogPathRestartPendingKey, false),
		PathRestartCooldownTo:  cooldownUntil,
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
	now := time.Now().UTC()
	_, assessments, pathStateValues := a.streamEndpointSelectionsWithPathState(ctx, bindings, settings, true, now)
	if len(pathStateValues) > 0 {
		if err := a.Store.PutSettings(ctx, pathStateValues); err != nil {
			return nil, err
		}
	}
	var changes []WatchdogPathChange
	for _, binding := range bindings {
		if binding.DeviceID == "" || binding.Device == nil {
			continue
		}
		assessment, ok := assessments[binding.DeviceID]
		if !ok || assessment.Selected == nil {
			continue
		}
		oldID := strings.TrimSpace(settings[streamrouting.ActivePathKeyPrefix+binding.DeviceID+".id"])
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
				Reason:   assessment.SwitchReason,
			})
		}
	}
	if err := a.saveActiveStreamPaths(ctx, assessments); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	a.go2rtcMu.Lock()
	defer a.go2rtcMu.Unlock()
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		a.markPathRestartPending(ctx, settings, now, "go2rtc-Konfiguration konnte nicht erzeugt werden")
		return changes, err
	}
	restarted, restartMessage, err := a.restartGo2RTCForPathChange(ctx, cfg, settings, now)
	if err != nil {
		return changes, err
	}
	_ = a.Store.AddEvent(ctx, "warning", "watchdog.path_switched", "Watchdog hat aktive Kamera-Pfade gewechselt", map[string]any{"changes": changes, "restart": restarted, "restart_message": restartMessage})
	return changes, nil
}

// restartGo2RTC is the raw restart; callers must hold go2rtcMu.
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
		PathRestartCooldown:    secondsSetting(settings, pathRestartCooldownSecondsKey, defaultPathRestartCooldownSeconds, 0, 7200),
	}
}

func (a *App) restartGo2RTCForPathChange(ctx context.Context, cfg watchdogConfig, settings map[string]string, now time.Time) (bool, string, error) {
	if !cfg.RestartOnChange {
		return false, "go2rtc-Neustart nach Pfadwechsel ist deaktiviert.", nil
	}
	if lastAt, ok := parseSettingTime(settings[watchdogPathRestartLastAtKey]); ok && cfg.PathRestartCooldown > 0 {
		cooldownUntil := lastAt.Add(cfg.PathRestartCooldown)
		if now.Before(cooldownUntil) {
			message := "go2rtc-Neustart wartet bis " + cooldownUntil.Format(time.RFC3339)
			_ = a.Store.PutSettings(ctx, map[string]string{
				watchdogPathRestartPendingKey:    "true",
				watchdogPathRestartPendingReason: message,
			})
			_ = a.Store.AddEvent(ctx, "warning", "watchdog.path_restart_cooldown", "go2rtc-Neustart wegen Cooldown verschoben", map[string]string{"cooldown_until": cooldownUntil.Format(time.RFC3339)})
			return false, message, nil
		}
	}
	if err := a.restartGo2RTC(ctx); err != nil {
		a.markPathRestartPending(ctx, settings, now, "go2rtc-Neustart nach Pfadwechsel fehlgeschlagen")
		return false, "", fmt.Errorf("go2rtc restart failed: %w", err)
	}
	a.clearPathRestartPending(ctx, now)
	return true, "go2rtc wurde neu gestartet.", nil
}

// markPathRestartPending lets restartPendingPathChange retry a failed render
// or restart with backoff; otherwise go2rtc keeps serving the dead path.
func (a *App) markPathRestartPending(ctx context.Context, settings map[string]string, now time.Time, reason string) {
	failures := counterSetting(settings, watchdogPathRestartFailuresKey) + 1
	_ = a.Store.PutSettings(ctx, map[string]string{
		watchdogPathRestartPendingKey:    "true",
		watchdogPathRestartPendingReason: reason,
		watchdogPathRestartFailuresKey:   strconv.Itoa(failures),
		watchdogPathRestartRetryAtKey:    now.Add(restartBackoff(failures)).Format(time.RFC3339),
	})
}

func (a *App) clearPathRestartPending(ctx context.Context, now time.Time) {
	_ = a.Store.PutSettings(ctx, map[string]string{
		watchdogPathRestartLastAtKey:     now.Format(time.RFC3339),
		watchdogPathRestartPendingKey:    "false",
		watchdogPathRestartPendingReason: "",
		watchdogPathRestartFailuresKey:   "0",
		watchdogPathRestartRetryAtKey:    "",
	})
}

func (a *App) restartPendingPathChange(ctx context.Context, cfg watchdogConfig, now time.Time) (bool, error) {
	if !cfg.RestartOnChange {
		return false, nil
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return false, err
	}
	if !boolSetting(settings, watchdogPathRestartPendingKey, false) {
		return false, nil
	}
	if lastAt, ok := parseSettingTime(settings[watchdogPathRestartLastAtKey]); ok && cfg.PathRestartCooldown > 0 && now.Before(lastAt.Add(cfg.PathRestartCooldown)) {
		return false, nil
	}
	if retryAt, ok := parseSettingTime(settings[watchdogPathRestartRetryAtKey]); ok && now.Before(retryAt) {
		return false, nil
	}
	a.go2rtcMu.Lock()
	defer a.go2rtcMu.Unlock()
	// Re-render: the pending flag may stem from a failed render.
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		a.markPathRestartPending(ctx, settings, now, "go2rtc-Konfiguration konnte nicht erzeugt werden")
		return false, err
	}
	if err := a.restartGo2RTC(ctx); err != nil {
		a.markPathRestartPending(ctx, settings, now, "go2rtc-Neustart nach Pfadwechsel fehlgeschlagen")
		return false, fmt.Errorf("go2rtc restart failed: %w", err)
	}
	a.clearPathRestartPending(ctx, now)
	_ = a.Store.AddEvent(ctx, "info", "watchdog.path_restart_after_cooldown", "Ausstehender go2rtc-Neustart nach Pfadwechsel ausgeführt", nil)
	return true, nil
}

func parseSettingTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
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

func counterSetting(settings map[string]string, key string) int {
	value, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func secondsSetting(settings map[string]string, key string, fallback, min, max int) time.Duration {
	value, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil {
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
