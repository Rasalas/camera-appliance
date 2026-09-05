package relay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/state"
)

const (
	RelayTypeSSHLocalForward = "ssh_local_forward"

	relayDefaultTargetPort = "554"
	relayBackoffDuration   = 60 * time.Second
)

func (a *Manager) Statuses(ctx context.Context) ([]Status, error) {
	relays, settings, err := a.managedRelays(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]Status, 0, len(relays))
	for _, relay := range relays {
		statuses = append(statuses, a.relayStatus(ctx, relay, settings))
	}
	return statuses, nil
}

func (a *Manager) Start(ctx context.Context, id string) (Status, error) {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()
	relay, settings, err := a.managedRelay(ctx, id)
	if err != nil {
		return Status{}, err
	}
	return a.startManagedRelay(ctx, relay, settings)
}

func (a *Manager) Stop(ctx context.Context, id string) (Status, error) {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()
	relay, settings, err := a.managedRelay(ctx, id)
	if err != nil {
		return Status{}, err
	}
	status := a.relayStatus(ctx, relay, settings)
	if status.PID <= 0 || !processAlive(status.PID) {
		_ = os.Remove(a.relayPIDPath(relay.ID))
		status.PID = 0
		status.ProcessState = "stopped"
		status.Message = "Kein verwalteter Relay-Prozess läuft."
		return status, nil
	}
	// PID reuse guard: after a reboot the pidfile may name an unrelated
	// process. Never signal anything that is not our ssh relay.
	if !processLooksLikeSSH(status.PID) {
		_ = os.Remove(a.relayPIDPath(relay.ID))
		status.PID = 0
		status.ProcessState = "stopped"
		status.Message = "Veralteter PID-Eintrag (Prozess gehört nicht zum Relay). Eintrag entfernt."
		return status, nil
	}
	if a.StopProcess != nil {
		err = a.StopProcess(ctx, status)
	} else {
		err = stopProcess(ctx, status.PID)
	}
	if err != nil {
		return status, err
	}
	_ = os.Remove(a.relayPIDPath(relay.ID))
	_ = a.store.PutSettings(ctx, map[string]string{
		relayRuntimeKey(relay.ID, "last_stopped_at"): time.Now().UTC().Format(time.RFC3339),
	})
	_ = a.store.AddEvent(ctx, "info", "relay.stopped", "Relay "+relay.ID+" wurde gestoppt", map[string]any{"relay_id": relay.ID, "pid": status.PID})
	status.PID = 0
	status.ProcessState = "stopped"
	status.Message = "Relay-Prozess gestoppt."
	return status, nil
}

func (a *Manager) Restart(ctx context.Context, id string) (Status, error) {
	if _, err := a.Stop(ctx, id); err != nil {
		return Status{}, err
	}
	return a.Start(ctx, id)
}

func (a *Manager) Ensure(ctx context.Context) ([]Status, error) {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()
	relays, settings, err := a.managedRelays(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	statuses := make([]Status, 0, len(relays))
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
			message := redaction.Text(startErr.Error())
			backoffUntil := now.Add(relayBackoffDuration)
			_ = a.store.PutSettings(ctx, map[string]string{
				relayRuntimeKey(relay.ID, "last_error"):    message,
				relayRuntimeKey(relay.ID, "backoff_until"): backoffUntil.Format(time.RFC3339),
			})
			_ = a.store.AddEvent(ctx, "error", "relay.start_failed", "Relay "+relay.ID+" konnte nicht gestartet werden", map[string]string{"relay_id": relay.ID, "error": message})
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

func (a *Manager) startManagedRelay(ctx context.Context, relay ManagedRelay, settings map[string]string) (Status, error) {
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
	if a.StartProcess != nil {
		pid, err = a.StartProcess(ctx, relay)
	} else {
		pid, err = startSSHRelayProcess(ctx, relay, a.relayLogPath(relay.ID))
	}
	if err != nil {
		return status, err
	}
	if err := writePID(a.relayPIDPath(relay.ID), pid); err != nil {
		return status, err
	}
	_ = a.store.PutSettings(ctx, map[string]string{
		relayRuntimeKey(relay.ID, "last_started_at"): time.Now().UTC().Format(time.RFC3339),
		relayRuntimeKey(relay.ID, "last_error"):      "",
		relayRuntimeKey(relay.ID, "backoff_until"):   "",
	})
	_ = a.store.AddEvent(ctx, "info", "relay.started", "Relay "+relay.ID+" wurde gestartet", map[string]any{"relay_id": relay.ID, "pid": pid})
	status = a.relayStatus(ctx, relay, settings)
	status.PID = pid
	status.ProcessState = "running"
	status.Started = true
	status.Message = "Relay-Prozess gestartet."
	status.LastError = ""
	status.BackoffUntil = ""
	return status, nil
}

// Manager owns relay lifecycle serialization, pidfiles and recovery backoff.
type Manager struct {
	config       config.Config
	store        *state.Store
	slots        []config.Slot
	relayMu      sync.Mutex
	Probe        func(context.Context, string, string) error
	StartProcess func(context.Context, ManagedRelay) (int, error)
	StopProcess  func(context.Context, Status) error
}

func New(cfg config.Config, store *state.Store, slots []config.Slot, probe func(context.Context, string, string) error) *Manager {
	return &Manager{config: cfg, store: store, slots: slots, Probe: probe}
}
