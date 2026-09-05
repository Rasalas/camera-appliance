package relay

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/streamrouting"
)

func (a *Manager) managedRelay(ctx context.Context, id string) (ManagedRelay, map[string]string, error) {
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

func (a *Manager) managedRelays(ctx context.Context) ([]ManagedRelay, map[string]string, error) {
	settings, err := a.store.Settings(ctx)
	if err != nil {
		return nil, nil, err
	}
	bindings, err := a.store.Bindings(ctx)
	if err != nil {
		return nil, nil, err
	}
	bindings = attachSlots(bindings, a.slots)
	return managedRelaysFromSettings(settings, bindings), settings, nil
}

func managedRelaysFromSettings(settings map[string]string, bindings []state.Binding) []ManagedRelay {
	var out []ManagedRelay
	for _, relay := range streamrouting.Relays(settings) {
		out = append(out, ManagedRelay{
			RelayDefinition: relay,
			Endpoints:       relayEndpointsFromSettings(settings, bindings, relay),
		})
	}
	return out
}

func relayEndpointsFromSettings(settings map[string]string, bindings []state.Binding, relay streamrouting.RelayDefinition) []RelayEndpoint {
	var endpoints []RelayEndpoint
	for _, binding := range bindings {
		if binding.DeviceID == "" {
			continue
		}
		if streamrouting.Policy(binding.DeviceID, settings) == streamrouting.PathPolicyDirectOnly {
			continue
		}
		prefix := "camera.relay_endpoint." + binding.DeviceID + "." + relay.ID + "."
		localHost, localPort := streamrouting.RelayEndpoint(settings, binding, relay)
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

func relayHealthHost(bindHost string) string {
	host := streamrouting.RelayBindHost(bindHost)
	switch host {
	case "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return host
	}
}

func relayRuntimeKey(relayID, name string) string {
	return "camera.relay." + relayID + ".runtime." + name
}

func (a *Manager) relayPIDPath(id string) string {
	return filepath.Join(a.relayStateDir(), safeRelayFilename(id)+".pid")
}

func (a *Manager) relayLogPath(id string) string {
	return filepath.Join(a.relayStateDir(), safeRelayFilename(id)+".log")
}

func (a *Manager) relayStateDir() string {
	return filepath.Join(a.config.StateDir, "relays")
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

func displayBindingLabel(binding state.Binding) string {
	if strings.TrimSpace(binding.Label) != "" {
		return binding.Label
	}
	if binding.Slot != nil && strings.TrimSpace(binding.Slot.Label) != "" {
		return binding.Slot.Label
	}
	return binding.DeviceID
}
