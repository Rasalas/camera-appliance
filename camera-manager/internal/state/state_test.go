package state

import (
	"context"
	"path/filepath"
	"testing"

	"camera-appliance/camera-manager/internal/config"
)

func TestStatePersistence(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertSlots(ctx, config.DefaultSlots()); err != nil {
		t.Fatal(err)
	}
	device := Device{ID: "dev1", LastIP: "192.168.1.20", MACAddress: "AA:BB:CC:DD:EE:FF"}
	if err := store.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertBinding(ctx, Binding{SlotID: "cam1", DeviceID: "dev1", Label: "Hof", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	bindings, err := store.Bindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Device == nil || bindings[0].Device.LastIP != "192.168.1.20" {
		t.Fatalf("unexpected bindings: %+v", bindings)
	}
	settings := map[string]string{"auto_discover": "true"}
	if err := store.PutSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	got, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["auto_discover"] != "true" {
		t.Fatalf("settings not persisted: %+v", got)
	}
}

func TestEmptyListsEncodeAsArrays(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertSlots(ctx, config.DefaultSlots()); err != nil {
		t.Fatal(err)
	}
	devices, err := store.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if devices == nil {
		t.Fatal("empty devices should be a non-nil slice")
	}
	bindings, err := store.Bindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bindings == nil {
		t.Fatal("empty bindings should be a non-nil slice")
	}
	events, err := store.Events(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if events == nil {
		t.Fatal("empty events should be a non-nil slice")
	}
}
