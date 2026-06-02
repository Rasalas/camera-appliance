package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"camera-appliance/camera-manager/internal/state"
)

const (
	RelayTypeSSHLocalForward = "ssh_local_forward"

	relayDefaultTargetPort = "554"
	relayBackoffDuration   = 60 * time.Second
)

type ManagedRelay struct {
	RelayDefinition
	Endpoints []RelayEndpoint `json:"endpoints"`
}

type RelayEndpoint struct {
	DeviceID   string `json:"device_id"`
	SlotID     string `json:"slot_id,omitempty"`
	Label      string `json:"label,omitempty"`
	LocalHost  string `json:"local_host,omitempty"`
	LocalPort  string `json:"local_port"`
	BindHost   string `json:"bind_host"`
	HealthHost string `json:"health_host"`
	TargetHost string `json:"target_host"`
	TargetPort string `json:"target_port"`
}

type RelayEndpointStatus struct {
	DeviceID   string `json:"device_id"`
	SlotID     string `json:"slot_id,omitempty"`
	Label      string `json:"label,omitempty"`
	LocalHost  string `json:"local_host,omitempty"`
	LocalPort  string `json:"local_port"`
	BindHost   string `json:"bind_host"`
	HealthHost string `json:"health_host"`
	TargetHost string `json:"target_host"`
	TargetPort string `json:"target_port"`
	State      string `json:"state"`
	Message    string `json:"message"`
}

type RelayStatus struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Type         string                `json:"type"`
	Host         string                `json:"host"`
	BindHost     string                `json:"bind_host"`
	SSHTarget    string                `json:"ssh_target,omitempty"`
	AutoStart    bool                  `json:"auto_start"`
	Enabled      bool                  `json:"enabled"`
	PID          int                   `json:"pid,omitempty"`
	ProcessState string                `json:"process_state"`
	Message      string                `json:"message"`
	LastError    string                `json:"last_error,omitempty"`
	BackoffUntil string                `json:"backoff_until,omitempty"`
	LogPath      string                `json:"log_path,omitempty"`
	Endpoints    []RelayEndpointStatus `json:"endpoints"`
}

func (a *App) RelayStatuses(ctx context.Context) ([]RelayStatus, error) {
	relays, settings, err := a.managedRelays(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]RelayStatus, 0, len(relays))
	for _, relay := range relays {
		statuses = append(statuses, a.relayStatus(ctx, relay, settings))
	}
	return statuses, nil
}

func (a *App) StartRelay(ctx context.Context, id string) (RelayStatus, error) {
	relay, settings, err := a.managedRelay(ctx, id)
	if err != nil {
		return RelayStatus{}, err
	}
	return a.startManagedRelay(ctx, relay, settings)
}

func (a *App) StopRelay(ctx context.Context, id string) (RelayStatus, error) {
	relay, settings, err := a.managedRelay(ctx, id)
	if err != nil {
		return RelayStatus{}, err
	}
	status := a.relayStatus(ctx, relay, settings)
	if status.PID <= 0 || !processAlive(status.PID) {
		_ = os.Remove(a.relayPIDPath(relay.ID))
		status.PID = 0
		status.ProcessState = "stopped"
		status.Message = "Kein verwalteter Relay-Prozess läuft."
		return status, nil
	}
	if a.RelayStop != nil {
		err = a.RelayStop(ctx, status)
	} else {
		err = stopProcess(ctx, status.PID)
	}
	if err != nil {
		return status, err
	}
	_ = os.Remove(a.relayPIDPath(relay.ID))
	_ = a.Store.PutSettings(ctx, map[string]string{
		relayRuntimeKey(relay.ID, "last_stopped_at"): time.Now().UTC().Format(time.RFC3339),
	})
	_ = a.Store.AddEvent(ctx, "info", "relay.stopped", "Relay "+relay.ID+" wurde gestoppt", map[string]any{"relay_id": relay.ID, "pid": status.PID})
	status.PID = 0
	status.ProcessState = "stopped"
	status.Message = "Relay-Prozess gestoppt."
	return status, nil
}

func (a *App) RestartRelay(ctx context.Context, id string) (RelayStatus, error) {
	if _, err := a.StopRelay(ctx, id); err != nil {
		return RelayStatus{}, err
	}
	return a.StartRelay(ctx, id)
}

func (a *App) EnsureManagedRelays(ctx context.Context) ([]RelayStatus, error) {
	relays, settings, err := a.managedRelays(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	statuses := make([]RelayStatus, 0, len(relays))
	var failures []string
	for _, relay := range relays {
		status := a.relayStatus(ctx, relay, settings)
		if !relay.AutoStart || status.ProcessState == "running" || status.ProcessState == "external" || status.ProcessState == "disabled" || status.ProcessState == "unsupported" || status.ProcessState == "not_configured" {
			statuses = append(statuses, status)
			continue
		}
		if backoffUntil, ok := relayBackoffUntil(settings, relay.ID); ok && now.Before(backoffUntil) {
			status.ProcessState = "backoff"
			status.BackoffUntil = backoffUntil.Format(time.RFC3339)
			status.Message = "Auto-Start wartet nach dem letzten Fehler."
			statuses = append(statuses, status)
			continue
		}
		started, startErr := a.startManagedRelay(ctx, relay, settings)
		if startErr != nil {
			message := startErr.Error()
			backoffUntil := now.Add(relayBackoffDuration)
			_ = a.Store.PutSettings(ctx, map[string]string{
				relayRuntimeKey(relay.ID, "last_error"):    message,
				relayRuntimeKey(relay.ID, "backoff_until"): backoffUntil.Format(time.RFC3339),
			})
			_ = a.Store.AddEvent(ctx, "error", "relay.start_failed", "Relay "+relay.ID+" konnte nicht gestartet werden", map[string]string{"relay_id": relay.ID, "error": message})
			status.LastError = message
			status.BackoffUntil = backoffUntil.Format(time.RFC3339)
			status.ProcessState = "error"
			status.Message = message
			statuses = append(statuses, status)
			failures = append(failures, relay.ID+": "+message)
			continue
		}
		statuses = append(statuses, started)
	}
	if len(failures) > 0 {
		return statuses, errors.New(strings.Join(failures, "; "))
	}
	return statuses, nil
}

func (a *App) managedRelay(ctx context.Context, id string) (ManagedRelay, map[string]string, error) {
	id = strings.TrimSpace(id)
	relays, settings, err := a.managedRelays(ctx)
	if err != nil {
		return ManagedRelay{}, nil, err
	}
	for _, relay := range relays {
		if relay.ID == id {
			return relay, settings, nil
		}
	}
	return ManagedRelay{}, settings, fmt.Errorf("relay %q ist nicht konfiguriert", id)
}

func (a *App) managedRelays(ctx context.Context) ([]ManagedRelay, map[string]string, error) {
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return nil, nil, err
	}
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		return nil, nil, err
	}
	bindings = attachSlots(bindings, a.Slots)
	return managedRelaysFromSettings(settings, bindings), settings, nil
}

func managedRelaysFromSettings(settings map[string]string, bindings []state.Binding) []ManagedRelay {
	var out []ManagedRelay
	for _, relay := range relayDefinitions(settings) {
		out = append(out, ManagedRelay{
			RelayDefinition: relay,
			Endpoints:       relayEndpointsFromSettings(settings, bindings, relay),
		})
	}
	return out
}

func relayEndpointsFromSettings(settings map[string]string, bindings []state.Binding, relay RelayDefinition) []RelayEndpoint {
	var endpoints []RelayEndpoint
	for _, binding := range bindings {
		if binding.DeviceID == "" {
			continue
		}
		prefix := "camera.relay_endpoint." + binding.DeviceID + "." + relay.ID + "."
		localHost, localPort := relayEndpoint(settings, binding.DeviceID, relay)
		targetHost := strings.TrimSpace(settings[prefix+"target_host"])
		if targetHost == "" && binding.Device != nil {
			targetHost = strings.TrimSpace(binding.Device.LastIP)
		}
		targetPort := strings.TrimSpace(settings[prefix+"target_port"])
		if targetPort == "" {
			targetPort = relayDefaultTargetPort
		}
		if localPort == "" && targetHost == "" {
			continue
		}
		endpoints = append(endpoints, RelayEndpoint{
			DeviceID:   binding.DeviceID,
			SlotID:     binding.SlotID,
			Label:      displayBindingLabel(binding),
			LocalHost:  localHost,
			LocalPort:  localPort,
			BindHost:   relay.BindHost,
			HealthHost: relayHealthHost(relay.BindHost),
			TargetHost: targetHost,
			TargetPort: targetPort,
		})
	}
	return endpoints
}

func (a *App) relayStatus(ctx context.Context, relay ManagedRelay, settings map[string]string) RelayStatus {
	status := RelayStatus{
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

func (a *App) relayEndpointStatuses(ctx context.Context, endpoints []RelayEndpoint) []RelayEndpointStatus {
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
				status.Message = rtspProbeDiagnostic(endpoint.LocalPort, err)
			} else {
				status.State = "ok"
				status.Message = "Lokaler Relay-Port erreichbar."
			}
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func (a *App) startManagedRelay(ctx context.Context, relay ManagedRelay, settings map[string]string) (RelayStatus, error) {
	status := a.relayStatus(ctx, relay, settings)
	if status.ProcessState == "running" {
		return status, nil
	}
	if status.ProcessState == "external" {
		return status, errors.New("lokale Relay-Ports sind bereits ohne verwalteten PID-Eintrag erreichbar")
	}
	if relay.Type != RelayTypeSSHLocalForward {
		return status, fmt.Errorf("relay-typ %q wird nicht unterstützt", relay.Type)
	}
	if relay.SSHTarget == "" {
		return status, errors.New("SSH-Ziel fehlt")
	}
	if len(relay.Endpoints) == 0 {
		return status, errors.New("keine Kamera-Endpunkte mit lokalem Relay-Port konfiguriert")
	}
	if _, err := sshRelayArgs(relay); err != nil {
		return status, err
	}
	if status.ProcessState == "stale" {
		_ = os.Remove(a.relayPIDPath(relay.ID))
	}
	var (
		pid int
		err error
	)
	if a.RelayStart != nil {
		pid, err = a.RelayStart(ctx, relay)
	} else {
		pid, err = startSSHRelayProcess(ctx, relay, a.relayLogPath(relay.ID))
	}
	if err != nil {
		return status, err
	}
	if err := writePID(a.relayPIDPath(relay.ID), pid); err != nil {
		return status, err
	}
	_ = a.Store.PutSettings(ctx, map[string]string{
		relayRuntimeKey(relay.ID, "last_started_at"): time.Now().UTC().Format(time.RFC3339),
		relayRuntimeKey(relay.ID, "last_error"):      "",
		relayRuntimeKey(relay.ID, "backoff_until"):   "",
	})
	_ = a.Store.AddEvent(ctx, "info", "relay.started", "Relay "+relay.ID+" wurde gestartet", map[string]any{"relay_id": relay.ID, "pid": pid})
	status = a.relayStatus(ctx, relay, settings)
	status.PID = pid
	status.ProcessState = "running"
	status.Message = "Relay-Prozess gestartet."
	status.LastError = ""
	status.BackoffUntil = ""
	return status, nil
}

func startSSHRelayProcess(ctx context.Context, relay ManagedRelay, logPath string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		return 0, errors.New("SSH ist nicht installiert; installiere openssh-client oder starte den Relay manuell")
	}
	args, err := sshRelayArgs(relay)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	_ = logFile.Close()
	return pid, nil
}

func sshRelayArgs(relay ManagedRelay) ([]string, error) {
	if relay.SSHTarget == "" {
		return nil, errors.New("SSH-Ziel fehlt")
	}
	args := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
	}
	seenLocalPorts := map[string]bool{}
	for _, endpoint := range relay.Endpoints {
		if err := validatePort("lokaler Relay-Port", endpoint.LocalPort); err != nil {
			return nil, err
		}
		if endpoint.TargetHost == "" {
			return nil, fmt.Errorf("%s: Ziel-IP fehlt", endpoint.DeviceID)
		}
		if err := validatePort("Ziel-Port", endpoint.TargetPort); err != nil {
			return nil, err
		}
		bindHost := relayBindHost(endpoint.BindHost)
		local := bindHost + ":" + endpoint.LocalPort
		if seenLocalPorts[local] {
			return nil, fmt.Errorf("lokaler Relay-Port doppelt belegt: %s", local)
		}
		seenLocalPorts[local] = true
		args = append(args, "-L", local+":"+endpoint.TargetHost+":"+endpoint.TargetPort)
	}
	if len(seenLocalPorts) == 0 {
		return nil, errors.New("keine Relay-Ports konfiguriert")
	}
	args = append(args, relay.SSHTarget)
	return args, nil
}

func validatePort(label, raw string) error {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s ist ungültig: %q", label, raw)
	}
	return nil
}

func relayHealthHost(bindHost string) string {
	host := relayBindHost(bindHost)
	switch host {
	case "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return host
	}
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

func relayRuntimeKey(relayID, name string) string {
	return "camera.relay." + relayID + ".runtime." + name
}

func (a *App) relayPIDPath(id string) string {
	return filepath.Join(a.relayStateDir(), safeRelayFilename(id)+".pid")
}

func (a *App) relayLogPath(id string) string {
	return filepath.Join(a.relayStateDir(), safeRelayFilename(id)+".log")
}

func (a *App) relayStateDir() string {
	return filepath.Join(a.Config.StateDir, "relays")
}

func safeRelayFilename(id string) string {
	var out strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			out.WriteRune(r)
			continue
		}
		out.WriteByte('_')
	}
	value := strings.Trim(out.String(), "._-")
	if value == "" {
		return "relay"
	}
	return value
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("ungültige PID-Datei %s", path)
	}
	return pid, nil
}

func writePID(path string, pid int) error {
	if pid <= 0 {
		return errors.New("ungültige Relay-PID")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func stopProcess(ctx context.Context, pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		if !waitForWatchdog(ctx, 100*time.Millisecond) {
			return ctx.Err()
		}
	}
	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func relayStatusReport(statuses []RelayStatus) string {
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
