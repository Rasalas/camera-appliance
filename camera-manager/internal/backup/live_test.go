package backup

import (
	"context"
	"path/filepath"
	"testing"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

func TestBackupIncludesCommittedWALChanges(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{StateDir: t.TempDir(), ConfigDir: t.TempDir()}
	live, err := state.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.PutSettings(ctx, map[string]string{"marker": "committed"}); err != nil {
		t.Fatal(err)
	}
	archive, err := Create(ctx, cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	restoredCfg := config.Config{StateDir: t.TempDir(), ConfigDir: t.TempDir()}
	if _, err := Restore(ctx, restoredCfg, archive.Path); err != nil {
		t.Fatal(err)
	}
	restored, err := state.Open(ctx, restoredCfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	settings, err := restored.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["marker"] != "committed" {
		t.Fatalf("backup lost committed WAL data: %v", settings)
	}
}

func TestRestoreUpdatesOpenStoreAndSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{StateDir: t.TempDir(), ConfigDir: t.TempDir()}
	live, err := state.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.PutSettings(ctx, map[string]string{"marker": "backup"}); err != nil {
		t.Fatal(err)
	}
	// Close the source so the original implementation gets a valid archive;
	// this test isolates restore from the independent WAL-backup defect.
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err := Create(ctx, cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	live, err = state.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.PutSettings(ctx, map[string]string{"marker": "newer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, cfg, archive.Path); err != nil {
		t.Fatal(err)
	}
	settings, err := live.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["marker"] != "backup" {
		t.Errorf("live store did not see restored state: %v", settings)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	settings, err = reopened.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["marker"] != "backup" {
		t.Errorf("old WAL overwrote restored state: %v", settings)
	}
}

func TestRestoreRejectsInvalidDatabaseBeforeChangingFiles(t *testing.T) {
	cfg := newBackupTestConfig(t)
	archive := filepath.Join(t.TempDir(), "invalid.tar.gz")
	buildTar(t, archive, map[string]tarEntry{"var/lib/camera-appliance/state.db": {content: "not a database", mode: 0600}})
	if _, err := Restore(context.Background(), cfg, archive); err == nil {
		t.Fatal("invalid database was accepted")
	}
}
