package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var editableSettingPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^viewer\.layout\.(id|mode|focus_slot_id|split_percent|gap_px|slot_order|custom|mosaic)$`),
	regexp.MustCompile(`^camera\.display\.[^.]+\.(rotation|mirror|flip|fit_mode|crop_x|crop_y|crop_width|crop_height)$`),
	regexp.MustCompile(`^camera\.path_policy\.[^.]+$`),
	regexp.MustCompile(`^camera\.credentials\.[^.]+\.(username|stream)$`),
	regexp.MustCompile(`^camera\.rtsp_endpoint\.[^.]+\.(host|port)$`),
	regexp.MustCompile(`^camera\.relay\.[^.]+\.(name|type|host|bind_host|ssh_target|auto_start|enabled|port_base|default_port)$`),
	regexp.MustCompile(`^camera\.relay_endpoint\.[^.]+\.[^.]+\.(host|port|target_host|target_port)$`),
}

var booleanSettings = map[string]bool{
	"auto_discover": true, "render_after_discovery": true, "restart_after_render": true,
	NetworkSettingLANAccess: true, AuthSettingViewerPublic: true, AuthSettingLocalAdminBypass: true,
	watchdogEnabledKey: true, watchdogRestartOnChangeKey: true, watchdogRestartGo2RTCOnFailureKey: true,
}
var numericSettings = map[string][2]int{
	AuthSettingSessionHours: {1, 168}, watchdogFastIntervalKey: {5, 3600}, watchdogCameraIntervalKey: {10, 7200},
	pathFailThresholdKey: {1, 20}, pathRecoveryThresholdKey: {1, 20}, pathRestartCooldownSecondsKey: {0, 7200},
}

// UpdateSettings is the configuration write boundary. Runtime state, hashes
// and credential identity metadata have dedicated owners and cannot be written
// through a stale settings form. Ignore read-only keys for older UI clients.
func (a *App) UpdateSettings(ctx context.Context, input map[string]string) error {
	values := map[string]string{}
	for key, value := range input {
		writable := false
		switch {
		case booleanSettings[key]:
			if value != "true" && value != "false" {
				return fmt.Errorf("%s muss true oder false sein", key)
			}
			writable = true
		case numericSettings[key] != [2]int{}:
			limits := numericSettings[key]
			n, err := strconv.Atoi(value)
			if err != nil || n < limits[0] || n > limits[1] {
				return fmt.Errorf("%s muss zwischen %d und %d liegen", key, limits[0], limits[1])
			}
			writable = true
		case key == "capture_ssh_host" || key == relayIDsKey || key == "viewer.layout.mosaic" || key == "viewer.layout.mode" || key == "viewer.layout.order":
			writable = true
		case key == viewerPerformanceSettingMode:
			if value != "quality" && value != "balanced" && value != "low" && value != "diagnostic" {
				return errors.New("unbekannter Performance-Modus")
			}
			writable = true
		default:
			for _, pattern := range editableSettingPatterns {
				if pattern.MatchString(key) {
					writable = true
					break
				}
			}
		}
		if writable {
			values[key] = value
		}
	}
	if values[NetworkSettingLANAccess] == "true" {
		current, err := a.Store.Settings(ctx)
		if err != nil {
			return err
		}
		if current[AuthSettingAdminPasswordHash] == "" {
			return errors.New("Vor dem LAN-Zugriff muss ein Admin-Passwort gesetzt werden")
		}
	}
	return a.Store.PutSettings(ctx, values)
}
