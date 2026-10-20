package backup

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestPruneKeepsNewestAndProtectedEntries(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"camera-appliance-20260101-120000.tar.gz",
		"camera-appliance-20260102-120000.000000001.tar.gz",
		"camera-appliance-20260103-120000.000000001.tar.gz",
		"camera-appliance-20260104-120000.000000001.tar.gz",
		"camera-appliance-20260105-120000.000000001.tar.gz",
		// Release downloads and manual names share the prefix but must survive.
		"camera-appliance-0.5.4-abc123.tar.gz",
		"camera-appliance-latest.tar.gz",
		"notes.txt",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	protected := filepath.Join(dir, "camera-appliance-20260101-120000.tar.gz")
	removed, err := Prune(dir, BackupArchives, 2, protected)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	want := []string{
		filepath.Join(dir, "camera-appliance-20260102-120000.000000001.tar.gz"),
		filepath.Join(dir, "camera-appliance-20260103-120000.000000001.tar.gz"),
	}
	if len(removed) != len(want) || removed[0] != want[0] || removed[1] != want[1] {
		t.Fatalf("removed %v, want %v", removed, want)
	}
	for _, name := range []string{
		"camera-appliance-20260101-120000.tar.gz",
		"camera-appliance-20260104-120000.000000001.tar.gz",
		"camera-appliance-20260105-120000.000000001.tar.gz",
		"camera-appliance-0.5.4-abc123.tar.gz",
		"camera-appliance-latest.tar.gz",
		"notes.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should survive pruning: %v", name, err)
		}
	}
}

func TestPruneRemovesRollbackDirectoriesOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"rollback-20260101-120000.000000000", "rollback-20260102-120000.000000000", "rollback-20260103-120000.000000000"} {
		if err := os.MkdirAll(filepath.Join(dir, name, "bin"), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// A file with a matching name is not a snapshot and stays.
	if err := os.WriteFile(filepath.Join(dir, "rollback-20250101-120000.000000000"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := Prune(dir, RollbackSnapshots, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed %v", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "rollback-20260103-120000.000000000", "bin")); err != nil {
		t.Fatal("newest snapshot must survive")
	}
	if _, err := os.Stat(filepath.Join(dir, "rollback-20250101-120000.000000000")); err != nil {
		t.Fatal("non-directory must survive")
	}
}

func TestCreateKeepsNewestDefaultBackups(t *testing.T) {
	cfg := newBackupTestConfig(t)
	if err := os.MkdirAll(cfg.BackupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"camera-appliance-20200101-000000.000000001.tar.gz",
		"camera-appliance-20200102-000000.000000001.tar.gz",
		"camera-appliance-20200103-000000.000000001.tar.gz",
		"camera-appliance-20200104-000000.000000001.tar.gz",
		"camera-appliance-20200105-000000.000000001.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(cfg.BackupDir(), name), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Create(context.Background(), cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(cfg.BackupDir(), "camera-appliance-*.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != KeepBackups {
		t.Fatalf("expected %d backups, got %v", KeepBackups, matches)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatal("new backup must survive pruning")
	}
	if _, err := os.Stat(filepath.Join(cfg.BackupDir(), "camera-appliance-20200101-000000.000000001.tar.gz")); !os.IsNotExist(err) {
		t.Fatal("oldest backup should be pruned")
	}
}
