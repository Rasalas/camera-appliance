package relay

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/streamrouting"
)

func (a *Manager) relayStatus(ctx context.Context, relay ManagedRelay, settings map[string]string) Status {
	status := Status{
		ID:           relay.ID,
		Name:         relay.Name,
		Type:         relay.Type,
		Host:         relay.Host,
		BindHost:     relay.BindHost,
		SSHTarget:    relay.SSHTarget,
		AutoStart:    relay.AutoStart,
		Enabled:      relay.Enabled,
		ProcessState: "unmanaged",
		Message:      "Auto-Start ist deaktiviert.",
		LastError:    strings.TrimSpace(settings[relayRuntimeKey(relay.ID, "last_error")]),
		BackoffUntil: strings.TrimSpace(settings[relayRuntimeKey(relay.ID, "backoff_until")]),
		LogPath:      a.relayLogPath(relay.ID),
		Endpoints:    a.relayEndpointStatuses(ctx, relay.Endpoints),
	}
	if !relay.Enabled {
		status.ProcessState = "disabled"
		status.Message = "Relay ist deaktiviert."
		return status
	}
	if relay.Type != RelayTypeSSHLocalForward {
		status.ProcessState = "unsupported"
		status.Message = "Relay-Typ wird noch nicht unterstützt."
		return status
	}
	pid, err := readPID(a.relayPIDPath(relay.ID))
	if err != nil {
		status.ProcessState = "error"
		status.Message = err.Error()
		return status
	}
	status.PID = pid
	if pid > 0 && processAlive(pid) {
		status.ProcessState = "running"
		status.Message = "Relay-Prozess läuft."
		return status
	}
	if pid > 0 {
		status.ProcessState = "stale"
		status.Message = "PID-Datei vorhanden, Prozess läuft aber nicht mehr."
		return status
	}
	if relayHasReachableLocalPort(status.Endpoints) {
		status.ProcessState = "external"
		status.Message = "Lokale Relay-Ports sind erreichbar, aber nicht durch diese App gestartet."
		return status
	}
	if relay.SSHTarget == "" {
		status.ProcessState = "not_configured"
		status.Message = "SSH-Ziel fehlt."
		return status
	}
	if len(relay.Endpoints) == 0 {
		status.ProcessState = "not_configured"
		status.Message = "Keine Kamera-Endpunkte mit lokalem Relay-Port konfiguriert."
		return status
	}
	if relay.AutoStart {
		status.ProcessState = "stopped"
		status.Message = "Auto-Start ist aktiv, Prozess läuft aber nicht."
	}
	return status
}

func (a *Manager) relayEndpointStatuses(ctx context.Context, endpoints []RelayEndpoint) []RelayEndpointStatus {
	statuses := make([]RelayEndpointStatus, 0, len(endpoints))
	for _, endpoint := range endpoints {
		status := RelayEndpointStatus{
			DeviceID:   endpoint.DeviceID,
			SlotID:     endpoint.SlotID,
			Label:      endpoint.Label,
			LocalHost:  endpoint.LocalHost,
			LocalPort:  endpoint.LocalPort,
			BindHost:   endpoint.BindHost,
			HealthHost: endpoint.HealthHost,
			TargetHost: endpoint.TargetHost,
			TargetPort: endpoint.TargetPort,
			State:      "missing_config",
			Message:    "Relay-Endpunkt unvollständig.",
		}
		switch {
		case endpoint.LocalPort == "":
			status.Message = "Lokaler Relay-Port fehlt."
		case endpoint.TargetHost == "":
			status.Message = "Ziel-IP der Kamera fehlt."
		default:
			if err := a.probeRTSP(ctx, endpoint.HealthHost, endpoint.LocalPort); err != nil {
				status.State = "failed"
				status.Message = streamrouting.ProbeDiagnostic(endpoint.LocalPort, err)
			} else {
				status.State = "ok"
				status.Message = "Lokaler Relay-Port erreichbar."
			}
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func relayHasReachableLocalPort(endpoints []RelayEndpointStatus) bool {
	for _, endpoint := range endpoints {
		if endpoint.State == "ok" {
			return true
		}
	}
	return false
}

func relayBackoffUntil(settings map[string]string, relayID string) (time.Time, bool) {
	raw := strings.TrimSpace(settings[relayRuntimeKey(relayID, "backoff_until")])
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func Report(statuses []Status) string {
	var out strings.Builder
	for _, status := range statuses {
		fmt.Fprintf(&out, "[%s] %s state=%s auto_start=%t pid=%d\n", status.ID, status.Name, status.ProcessState, status.AutoStart, status.PID)
		if status.Message != "" {
			fmt.Fprintf(&out, "message=%s\n", status.Message)
		}
		if status.LastError != "" {
			fmt.Fprintf(&out, "last_error=%s\n", status.LastError)
		}
		if status.BackoffUntil != "" {
			fmt.Fprintf(&out, "backoff_until=%s\n", status.BackoffUntil)
		}
		for _, endpoint := range status.Endpoints {
			target := net.JoinHostPort(endpoint.TargetHost, endpoint.TargetPort)
			local := net.JoinHostPort(endpoint.HealthHost, endpoint.LocalPort)
			fmt.Fprintf(&out, "endpoint=%s slot=%s local=%s target=%s state=%s message=%s\n", endpoint.DeviceID, endpoint.SlotID, local, target, endpoint.State, endpoint.Message)
		}
		fmt.Fprintln(&out)
	}
	if out.Len() == 0 {
		return "Keine Relays konfiguriert.\n"
	}
	return out.String()
}

func (a *Manager) probeRTSP(ctx context.Context, host, port string) error {
	if a.Probe != nil {
		return a.Probe(ctx, host, port)
	}
	timeout := a.config.RequestTimeout
	if timeout <= 0 {
		timeout = 700 * time.Millisecond
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(probeCtx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
