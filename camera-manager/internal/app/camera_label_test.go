package app

import (
	"context"
	"testing"

	"camera-appliance/camera-manager/internal/state"
)

func TestViewerLabelPrefersCustomDeviceName(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "")
	for _, device := range []state.Device{
		{ID: "named", LastIP: "192.168.1.20", Manufacturer: "Tapo", Model: "C200"},
		{ID: "unnamed", LastIP: "192.168.1.21", Manufacturer: "Tapo", Model: "C200"},
		{ID: "bare", LastIP: "192.168.1.22"},
	} {
		if err := a.Store.UpsertDevice(ctx, device); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Store.SetDeviceName(ctx, "named", "Einfahrt"); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []state.Binding{
		{SlotID: "cam1", DeviceID: "named", Label: "Tapo C200", Enabled: true},
		{SlotID: "cam2", DeviceID: "unnamed", Label: "Tapo C200", Enabled: true},
		{SlotID: "cam3", DeviceID: "bare", Enabled: true},
	} {
		if err := a.Store.UpsertBinding(ctx, binding); err != nil {
			t.Fatal(err)
		}
	}

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"cam1": "Einfahrt", "cam2": "Tapo C200", "cam3": "Kamera 3", "cam4": "Kamera 4"}
	for _, slot := range viewer.Slots {
		if expected, ok := want[slot.Alias]; ok && slot.Label != expected {
			t.Fatalf("%s: label %q, want %q", slot.Alias, slot.Label, expected)
		}
	}
}
