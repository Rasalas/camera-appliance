package state

import (
	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/migrations"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestVersionedMigrationOfLegacyDatabaseAndRestore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", sqliteURI(path, "rwc"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := migrations.Files.ReadFile("001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(data)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO devices(id,first_seen_at,last_seen_at) VALUES ('legacy','',''); INSERT INTO slots VALUES ('cam1','Kamera','camera','stream2',0,0,'',''); INSERT INTO bindings VALUES ('cam1','legacy','','old-user','stream2',1,'',''); INSERT INTO settings VALUES ('camera.credentials.legacy.username','saved-user','')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	backup := filepath.Join(t.TempDir(), "old.db")
	if err := Snapshot(ctx, path, backup); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		store, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		bindings, err := store.Bindings(ctx)
		if err != nil || len(bindings) != 1 || bindings[0].Username != "saved-user" {
			t.Fatalf("legacy binding: %+v %v", bindings, err)
		}
		if _, err := store.SetDeviceName(ctx, "legacy", "Hof"); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveAuthSession(ctx, AuthSession{TokenHash: "hash", Role: "admin", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AuthSession(ctx, "hash", time.Now()); err != nil {
			t.Fatal(err)
		}
		d, err := store.Device(ctx, "legacy")
		if err != nil || d.Name != "Hof" {
			t.Fatalf("name lost %+v %v", d, err)
		}
		store.Close()
	}
	if err := RestoreSnapshot(ctx, path, backup); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SetDeviceName(ctx, "legacy", "Wiederhergestellt"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("migration ledger %d: %v", count, err)
	}
}

func TestConcurrentOpenAndVisibilityAllocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	var wg sync.WaitGroup
	stores := make([]*Store, 4)
	errs := make([]error, 4)
	for i := range stores {
		wg.Add(1)
		go func(i int) { defer wg.Done(); stores[i], errs[i] = Open(ctx, path) }(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		defer stores[i].Close()
	}
	if err := stores[0].UpsertSlots(ctx, []config.Slot{{ID: "cam1", DefaultStream: "stream2"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if err := stores[0].UpsertDevice(ctx, Device{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	visible := true
	errs = make([]error, 2)
	for i, id := range []string{"one", "two"} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_, errs[i] = stores[i].ConfigureCamera(ctx, id, map[string]string{"camera.credentials." + id + ".username": "user"}, nil, &visible)
		}(i, id)
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrNoCameraSlot) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("allocation results %v", errs)
	}
	settings, err := stores[0].Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 2 {
		t.Fatalf("failed allocation partially persisted: %v", settings)
	}
}

func TestCameraCredentialWriteRollsBackBindingAndSettings(t *testing.T) {
	ctx := context.Background()
	store := openNameTestStore(t)
	if err := store.UpsertDevice(ctx, Device{ID: "device"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertBinding(ctx, Binding{SlotID: "cam1", DeviceID: "device", Username: "original", StreamName: "stream1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER reject_stream BEFORE UPDATE ON settings WHEN new.key='camera.credentials.device.stream' BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	err := store.UpsertBinding(ctx, Binding{SlotID: "cam1", DeviceID: "device", Username: "changed", StreamName: "stream2", Enabled: true})
	if err == nil {
		t.Fatal("expected failure")
	}
	bindings, err := store.Bindings(ctx)
	if err != nil || bindings[0].Username != "original" || bindings[0].StreamName != "stream1" {
		t.Fatalf("partial binding write %+v %v", bindings, err)
	}
	settings, err := store.Settings(ctx)
	if err != nil || settings["camera.credentials.device.username"] != "original" {
		t.Fatalf("partial settings write %v %v", settings, err)
	}
}

func TestUnversionedRuntimeSchemaPreservesNamesAndAuth(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertDevice(ctx, Device{ID: "device"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetDeviceName(ctx, "device", "Einfahrt"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuthSession(ctx, AuthSession{TokenHash: "hash", Role: "admin", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE schema_migrations`); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	d, err := store.Device(ctx, "device")
	if err != nil || d.Name != "Einfahrt" {
		t.Fatalf("name lost %+v %v", d, err)
	}
	if _, err := store.AuthSession(ctx, "hash", time.Now()); err != nil {
		t.Fatal(err)
	}
}
