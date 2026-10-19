package app

import (
	"context"
	"net/url"
	"strings"

	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
)

// Go2RTCFrameURL returns go2rtc's frame endpoint for a bound device's stream while go2rtc is healthy.
// Capturing there reuses go2rtc's camera session instead of opening another one.
func (a *App) Go2RTCFrameURL(ctx context.Context, device state.Device, stream string) (string, bool) {
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return "", false
	}
	stream = strings.Trim(strings.TrimSpace(stream), "/")
	aliases, _ := a.generatedGo2RTCAliases()
	alias := ""
	for _, binding := range bindings {
		if binding.DeviceID != device.ID || !binding.Enabled || binding.SlotID == "" {
			continue
		}
		bound := binding.StreamName
		if bound == "" {
			for _, slot := range a.Slots {
				if slot.ID == binding.SlotID {
					bound = slot.DefaultStream
				}
			}
		}
		// Prefer the viewer alias: its producer is usually already running.
		if strings.Trim(strings.TrimSpace(bound), "/") == stream && aliases[binding.SlotID] {
			alias = binding.SlotID
		} else if profile := viewerProfileAlias(binding.SlotID, stream); aliases[profile] {
			alias = profile
		}
		break
	}
	if alias == "" {
		return "", false
	}
	base, err := url.Parse(strings.TrimSpace(a.Config.Go2RTCURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", false
	}
	if !system.Go2RTCStatus(ctx, a.Config).Online {
		return "", false
	}
	frameURL := base.JoinPath("api", "frame.jpeg")
	frameURL.RawQuery = url.Values{"src": {alias}}.Encode()
	return frameURL.String(), true
}
