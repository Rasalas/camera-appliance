package state

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/config"
)

func TestNormalizeDeviceName(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		invalid  bool
	}{
		{in: "  Einfahrt  ", want: "Einfahrt"},
		{in: "", want: ""},
		{in: "   ", want: ""},
		{in: strings.Repeat("ä", DeviceNameMaxLength), want: strings.Repeat("ä", DeviceNameMaxLength)},
		{in: strings.Repeat("a", DeviceNameMaxLength+1), invalid: true},
		{in: "Hof\nhinten", invalid: true},
	} {
		got, err := NormalizeDeviceName(tc.in)
		if tc.invalid {
			if !errors.Is(err, ErrInvalidDeviceName) {
				t.Fatalf("%q: expected invalid name error, got %q, %v", tc.in, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestDeviceNameSurvivesRediscoveryAndAppearsInBindings(t *testing.T) {
	ctx := context.Background()
	store := openNameTestStore(t)
	if err := store.UpsertDevice(ctx, Device{ID: "dev1", LastIP: "192.168.1.20", Manufacturer: "Tapo", Model: "C200"}); err != nil {
		t.Fatal(err)
	}
	device, err := store.SetDeviceName(ctx, "dev1", "  Einfahrt ")
	if err != nil || device.Name != "Einfahrt" {
		t.Fatalf("set name: %+v, %v", device, err)
	}
	// A rediscovery with a new DHCP address must keep the name.
	if err := store.UpsertDevice(ctx, Device{ID: "dev1", LastIP: "192.168.1.99", Manufacturer: "Tapo", Model: "C200"}); err != nil {
		t.Fatal(err)
	}
	device, err = store.Device(ctx, "dev1")
	if err != nil || device.Name != "Einfahrt" || device.LastIP != "192.168.1.99" {
		t.Fatalf("after rediscovery: %+v, %v", device, err)
	}
	if err := store.UpsertBinding(ctx, Binding{SlotID: "cam1", DeviceID: "dev1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	bindings, err := store.Bindings(ctx)
	if err != nil || len(bindings) != 1 || bindings[0].Device == nil || bindings[0].Device.Name != "Einfahrt" {
		t.Fatalf("binding device name missing: %+v, %v", bindings, err)
	}
	if device, err = store.SetDeviceName(ctx, "dev1", ""); err != nil || device.Name != "" {
		t.Fatalf("clear name: %+v, %v", device, err)
	}
}

func TestSetDeviceNameRejectsUnknownDeviceAndInvalidName(t *testing.T) {
	ctx := context.Background()
	store := openNameTestStore(t)
	if _, err := store.SetDeviceName(ctx, "missing", "Hof"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
	if err := store.UpsertDevice(ctx, Device{ID: "dev1", Manufacturer: "Tapo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetDeviceName(ctx, "dev1", strings.Repeat("x", 41)); !errors.Is(err, ErrInvalidDeviceName) {
		t.Fatalf("expected invalid name error, got %v", err)
	}
	device, _ := store.Device(ctx, "dev1")
	if device.Name != "" {
		t.Fatalf("invalid name was stored: %q", device.Name)
	}
}

func TestOpenAndRestoreAddDeviceNameToOlderDatabases(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	old := filepath.Join(dir, "old.db")
	createPreNameDatabase(t, old)

	store, err := Open(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetDeviceName(ctx, "dev1", "Garten"); err != nil {
		t.Fatalf("migrated database: %v", err)
	}
	_ = store.Close()

	backup := filepath.Join(dir, "backup.db")
	createPreNameDatabase(t, backup)
	live := openNameTestStoreAt(t, filepath.Join(dir, "live.db"))
	if err := RestoreSnapshot(ctx, filepath.Join(dir, "live.db"), backup); err != nil {
		t.Fatal(err)
	}
	if _, err := live.Devices(ctx); err != nil {
		t.Fatalf("live store after restoring an older backup: %v", err)
	}
	if _, err := live.SetDeviceName(ctx, "dev1", "Garten"); err != nil {
		t.Fatalf("rename after restore: %v", err)
	}
}

func createPreNameDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteURI(path, "rwc"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE slots (id TEXT PRIMARY KEY, label TEXT NOT NULL, role TEXT NOT NULL, default_stream TEXT NOT NULL, required INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE devices (id TEXT PRIMARY KEY, first_seen_at TEXT NOT NULL, last_seen_at TEXT NOT NULL, last_ip TEXT, mac_address TEXT, onvif_endpoint_ref TEXT, serial_number TEXT, manufacturer TEXT, model TEXT, hardware_id TEXT, hostname TEXT, raw_json TEXT);`,
		`CREATE TABLE bindings (slot_id TEXT PRIMARY KEY, device_id TEXT NOT NULL, label TEXT, username TEXT, stream_name TEXT NOT NULL DEFAULT 'stream2', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`INSERT INTO devices (id,first_seen_at,last_seen_at,manufacturer,model) VALUES ('dev1','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','Tapo','C200');`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func openNameTestStore(t *testing.T) *Store {
	return openNameTestStoreAt(t, filepath.Join(t.TempDir(), "state.db"))
}

func openNameTestStoreAt(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.UpsertSlots(context.Background(), config.DefaultSlots()); err != nil {
		t.Fatal(err)
	}
	return store
}
