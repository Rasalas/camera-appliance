package app

import (
	"context"
	"fmt"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	go2rtcrender "camera-appliance/camera-manager/internal/go2rtc"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

func (a *App) RenderGo2RTC(ctx context.Context) (go2rtcrender.RenderResult, error) {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return go2rtcrender.RenderResult{}, err
	}
	settings, _ := a.Store.Settings(ctx)
	passwords := map[string]string{}
	for i := range bindings {
		deviceID := bindings[i].DeviceID
		if strings.TrimSpace(bindings[i].Username) == "" {
			bindings[i].Username = settings["camera.credentials."+deviceID+".username"]
		}
		secret := secrets.LoadCamera(a.Config.ConfigDir, deviceID)
		if secret.Value != "" {
			passwords[deviceID] = secret.Value
		}
	}
	endpoints, assessments := a.streamEndpointSelections(ctx, bindings, settings)
	renderPassword, _ := a.CameraCredentials()
	result, err := go2rtcrender.Render(ctx, go2rtcrender.RenderInput{
		Slots:     a.Slots,
		Bindings:  bindings,
		Password:  renderPassword,
		Passwords: passwords,
		Endpoints: endpoints,
		Output:    a.Config.Go2RTCConfigPath(),
	})
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, pathWarnings(bindings, assessments)...)
	if err := a.saveActiveStreamPaths(ctx, assessments); err != nil {
		return result, err
	}
	_ = a.Store.AddEvent(ctx, "info", "go2rtc.rendered", fmt.Sprintf("go2rtc-Konfiguration erzeugt: %d Streams", result.RenderedStreams), map[string]any{"warnings": result.Warnings})
	return result, nil
}

func attachSlots(bindings []state.Binding, slots []config.Slot) []state.Binding {
	slotMap := map[string]config.Slot{}
	for _, slot := range slots {
		slotMap[slot.ID] = slot
	}
	for i := range bindings {
		if slot, ok := slotMap[bindings[i].SlotID]; ok {
			local := slot
			bindings[i].Slot = &local
		}
	}
	return bindings
}
