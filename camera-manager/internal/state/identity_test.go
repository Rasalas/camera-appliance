package state

import (
	"context"
	"path/filepath"
	"testing"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/fingerprint"
)

func TestReconcilePreservesIDBindingsAndSettings(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertSlots(ctx, config.DefaultSlots()); err != nil {
		t.Fatal(err)
	}
	device := Device{MACAddress: "AA:BB:CC:DD:EE:01", Manufacturer: "Tapo", Model: "C310", LastIP: "192.0.2.10"}
	device.ID = fingerprint.DeviceID(device.Fingerprint())
	if err := store.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertBinding(ctx, Binding{SlotID: "cam1", DeviceID: device.ID}); err != nil {
		t.Fatal(err)
	}
	key := "camera.credentials." + device.ID + ".username"
	if err := store.PutSettings(ctx, map[string]string{key: "camera-user"}); err != nil {
		t.Fatal(err)
	}
	discovered := device
	discovered.SerialNumber = "serial-1"
	discovered.LastIP = "192.0.2.11"
	discovered.ID = fingerprint.DeviceID(discovered.Fingerprint())
	saved, err := store.ReconcileDevice(ctx, discovered)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != device.ID {
		t.Fatalf("identity changed from %s to %s", device.ID, saved.ID)
	}
	// Missing ONVIF on the next scan must not downgrade or duplicate identity.
	discovered.SerialNumber = ""
	discovered.ID = fingerprint.DeviceID(discovered.Fingerprint())
	if _, err := store.ReconcileDevice(ctx, discovered); err != nil {
		t.Fatal(err)
	}
	devices, err := store.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := store.Bindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].SerialNumber != "serial-1" || bindings[0].Device.LastIP != "192.0.2.11" || settings[key] != "camera-user" {
		t.Fatalf("lost camera state: devices=%+v bindings=%+v settings=%+v", devices, bindings, settings)
	}
}

func TestReconcileDoesNotMatchOnlyByIPOrAmbiguousMAC(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, d := range []Device{{ID: "one", MACAddress: "AA:BB:CC:DD:EE:01", LastIP: "192.0.2.10"}, {ID: "two", MACAddress: "AA:BB:CC:DD:EE:01", LastIP: "192.0.2.11"}} {
		if err := store.UpsertDevice(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReconcileDevice(ctx, Device{MACAddress: "AA:BB:CC:DD:EE:01", LastIP: "192.0.2.12"}); err == nil {
		t.Fatal("ambiguous identity accepted")
	}
	saved, err := store.ReconcileDevice(ctx, Device{LastIP: "192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "one" || saved.ID == "two" {
		t.Fatal("matched by IP alone")
	}
}

func TestBindingRejectsUnknownReferences(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertSlots(ctx, config.DefaultSlots()); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertDevice(ctx, Device{ID: "known"}); err != nil {
		t.Fatal(err)
	}
	for _, binding := range []Binding{{SlotID: "cam1", DeviceID: "missing"}, {SlotID: "missing", DeviceID: "known"}} {
		if err := store.UpsertBinding(ctx, binding); err == nil {
			t.Errorf("accepted invalid binding %+v", binding)
		}
	}
	if bindings, err := store.Bindings(ctx); err != nil || len(bindings) != 0 {
		t.Fatalf("bindings corrupted: %+v, %v", bindings, err)
	}
}

func TestLegacyOrphanBindingRemainsReadable(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO bindings(slot_id,device_id,created_at,updated_at) VALUES('cam1','missing','','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	bindings, err := store.Bindings(ctx)
	if err != nil || len(bindings) != 1 || bindings[0].Device != nil {
		t.Fatalf("legacy orphan should be visible as unresolvable: %+v %v", bindings, err)
	}
}
