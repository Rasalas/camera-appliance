package app

import (
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

// viewerSlotLabel prefers the customer's camera name, then the binding label
// (historically manufacturer and model), then the internal slot label.
func viewerSlotLabel(slot config.Slot, binding state.Binding) string {
	if binding.Device != nil && strings.TrimSpace(binding.Device.Name) != "" {
		return strings.TrimSpace(binding.Device.Name)
	}
	if strings.TrimSpace(binding.Label) != "" {
		return strings.TrimSpace(binding.Label)
	}
	return slot.Label
}
