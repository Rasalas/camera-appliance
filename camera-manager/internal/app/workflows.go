package app

import (
	"context"
	"time"

	go2rtcrender "camera-appliance/camera-manager/internal/go2rtc"
)

// RunStartup performs the optional initial discovery before the watchdog starts.
func (a *App) RunStartup(ctx context.Context) error {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return err
	}
	if !boolSetting(settings, "auto_discover", false) {
		return nil
	}
	scanCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_, err = a.Discover(scanCtx)
	return err
}

// RenderConfiguredGo2RTC is used by user-triggered render and discovery flows.
// The watchdog retains its own restart policy and cooldown.
func (a *App) RenderConfiguredGo2RTC(ctx context.Context) (go2rtcrender.RenderResult, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return go2rtcrender.RenderResult{}, err
	}
	result, err := a.RenderGo2RTC(ctx)
	if err != nil {
		return result, err
	}
	if boolSetting(settings, "restart_after_render", false) {
		err = a.restartGo2RTC(ctx)
	}
	return result, err
}

func (a *App) RestartStreams(ctx context.Context) error {
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		return err
	}
	return a.restartGo2RTC(ctx)
}
