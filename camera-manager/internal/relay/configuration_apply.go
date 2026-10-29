package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

type configurationSnapshot struct {
	Relays []ManagedRelay  `json:"relays"`
	Dirty  map[string]bool `json:"dirty,omitempty"`
}

// ConfigurationUpdate holds the lifecycle lock across a desired-state write and
// its apply. Callers acquire their renderer lock inside this scope, matching the
// watchdog's relay-before-renderer lock order. Before is opaque durable retry data.
type ConfigurationUpdate struct {
	manager *Manager
	Before  string
}

func (a *Manager) Configure(ctx context.Context, change func(*ConfigurationUpdate) error) error {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()
	relays, _, err := a.managedRelays(ctx)
	if err != nil {
		return err
	}
	before, err := json.Marshal(configurationSnapshot{Relays: relays})
	if err != nil {
		return err
	}
	return change(&ConfigurationUpdate{manager: a, Before: string(before)})
}

// RetryData remembers relays already changed during an earlier incomplete apply.
// This also handles a superseding save that returns to the original settings:
// the running process might already have adopted the intermediate settings.
func (u *ConfigurationUpdate) RetryData(before string) (string, error) {
	var prior, current configurationSnapshot
	if err := json.Unmarshal([]byte(before), &prior); err != nil {
		return "", err
	}
	if err := json.Unmarshal([]byte(u.Before), &current); err != nil {
		return "", err
	}
	if prior.Dirty == nil {
		prior.Dirty = map[string]bool{}
	}
	old := map[string]ManagedRelay{}
	for _, r := range prior.Relays {
		old[r.ID] = r
	}
	for _, r := range current.Relays {
		if !sameForwards(old[r.ID], r) {
			prior.Dirty[r.ID] = true
		}
	}
	data, err := json.Marshal(prior)
	return string(data), err
}

func sameForwards(a, b ManagedRelay) bool {
	args, err := sshRelayArgs(a)
	other, otherErr := sshRelayArgs(b)
	return reflect.DeepEqual(args, other) && fmt.Sprint(err) == fmt.Sprint(otherErr)
}

// Apply reconciles only forwards whose process arguments changed. The caller
// retains before until the complete camera apply succeeds, including go2rtc.
func (u *ConfigurationUpdate) Apply(ctx context.Context, before string) ([]string, error) {
	var previous configurationSnapshot
	if err := json.Unmarshal([]byte(before), &previous); err != nil {
		return nil, fmt.Errorf("Relay-Wiederholungsdaten ungültig: %w", err)
	}
	old := map[string]ManagedRelay{}
	for _, r := range previous.Relays {
		old[r.ID] = r
	}
	a := u.manager
	relays, settings, err := a.managedRelays(ctx)
	if err != nil {
		return nil, err
	}
	var warnings []string
	for _, r := range relays {
		prior := old[r.ID]
		_, argErr := sshRelayArgs(r)
		if !previous.Dirty[r.ID] && sameForwards(prior, r) {
			continue
		}
		if !r.Enabled || r.Type != RelayTypeSSHLocalForward {
			continue
		}
		if !r.AutoStart {
			warnings = append(warnings, "Relay "+r.ID+": Auto-Start ist deaktiviert; geänderte Weiterleitungen müssen manuell übernommen werden.")
			continue
		}
		status := a.relayStatus(ctx, r, settings)
		// Probe the previous ports too: a changed local port must not turn an
		// external forward into a managed process owned by this save.
		if status.ProcessState == "external" || (status.PID == 0 && a.relayStatus(ctx, prior, settings).ProcessState == "external") {
			warnings = append(warnings, "Relay "+r.ID+": Externe Weiterleitungen müssen manuell angepasst werden.")
			continue
		}
		// Validate before disrupting the old process. An empty forward set means
		// stop, never start an ssh connection with no useful forwards.
		hasForwards := false
		for _, endpoint := range r.Endpoints {
			if endpoint.LocalPort != "" && endpoint.TargetHost != "" {
				hasForwards = true
			}
		}
		if hasForwards && argErr != nil {
			return warnings, fmt.Errorf("Relay %s: %w", r.ID, argErr)
		}
		if _, err := a.stopManagedRelay(ctx, r, settings); err != nil {
			return warnings, fmt.Errorf("Relay %s stoppen: %w", r.ID, err)
		}
		if !hasForwards {
			continue
		}
		started, err := a.startManagedRelay(ctx, r, settings)
		if err == nil {
			_, err = a.confirmStarted(ctx, started)
		}
		if err != nil {
			return warnings, fmt.Errorf("Relay %s starten: %w", r.ID, err)
		}
	}
	return warnings, nil
}
