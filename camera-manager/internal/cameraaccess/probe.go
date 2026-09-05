package cameraaccess

import (
	"context"
	"net"
	"time"

	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/streamrouting"
)

func probeMessage(err error) string {
	if err == nil {
		return "RTSP-Port erreichbar. Passwort wird beim Vorschaubild oder go2rtc-Stream praktisch geprüft."
	}
	return "RTSP-Port nicht erreichbar. Prüfe Netzwerk, Stromversorgung oder ob RTSP/ONVIF aktiv ist."
}

type ProbeResult struct {
	Success     bool   `json:"success"`
	URLRedacted string `json:"url_redacted"`
	Message     string `json:"message"`
}

func (s *Service) Probe(ctx context.Context, deviceID string, req CredentialsInput) (ProbeResult, error) {
	device, err := s.store.Device(ctx, deviceID)
	if err != nil {
		return ProbeResult{}, failure(NotFound, err)
	}
	applySavedCredentials(ctx, s, deviceID, &req.Username, &req.Password, &req.Stream)
	if req.Stream == "" {
		req.Stream = "stream2"
	}
	endpoint, err := s.endpoint(ctx, device)
	if err != nil {
		return ProbeResult{}, err
	}
	rawURL := cameraRTSPURL(req.Username, req.Password, endpoint.Host, endpoint.Port, req.Stream)
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	conn, dialErr := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(streamrouting.ProbeHostForEndpoint(endpoint.Host), endpoint.Port))
	if dialErr == nil {
		_ = conn.Close()
	}
	return ProbeResult{Success: dialErr == nil, URLRedacted: redaction.URL(rawURL), Message: probeMessage(dialErr)}, nil
}
