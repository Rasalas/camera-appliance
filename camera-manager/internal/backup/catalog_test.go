package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

func writeBackupFile(t *testing.T, cfg config.Config, name, content string) {
	t.Helper()
	if err := os.MkdirAll(cfg.BackupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.BackupDir(), name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListClassifiesBackupsAndSkipsOtherFiles(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir()}
	writeBackupFile(t, cfg, "camera-appliance-20260101-120000.000000001.tar.gz", "manual")
	writeBackupFile(t, cfg, "camera-appliance-auto-20260103-120000.000000001.tar.gz", "auto")
	writeBackupFile(t, cfg, "camera-appliance-update-20260102-120000.000000001.tar.gz", "update!")
	writeBackupFile(t, cfg, "support-bundle-20260104-120000.tar.gz", "bundle")
	writeBackupFile(t, cfg, "last-update.json", "{}")
	writeBackupFile(t, cfg, ".archive-123", "temp")
	if err := os.Mkdir(filepath.Join(cfg.BackupDir(), "camera-appliance-20260105-120000.tar.gz"), 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.tar.gz")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cfg.BackupDir(), "camera-appliance-20260106-120000.tar.gz")); err != nil {
		t.Fatal(err)
	}

	entries, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind Kind
		size int64
		day  int
	}{{KindAutomatic, 4, 3}, {KindUpdate, 7, 2}, {KindManual, 6, 1}}
	if len(entries) != len(want) {
		t.Fatalf("entries = %+v", entries)
	}
	for i, w := range want {
		got := entries[i]
		if got.Kind != w.kind || got.Size != w.size || !got.CreatedAt.Equal(time.Date(2026, 1, w.day, 12, 0, 0, 1, time.UTC)) {
			t.Fatalf("entry %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestListWithoutBackupDirectoryIsEmpty(t *testing.T) {
	entries, err := List(config.Config{StateDir: t.TempDir()})
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
}

func TestOpenAndDeleteRejectNamesOutsideThePattern(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir()}
	writeBackupFile(t, cfg, "camera-appliance-20260101-120000.tar.gz", "ok")
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "state.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"", "..", "../state.db", "camera-appliance-20260101-120000.tar.gz/..", "./camera-appliance-20260101-120000.tar.gz",
		"../backups/camera-appliance-20260101-120000.tar.gz", "/etc/passwd", "last-update.json",
		"camera-appliance-20260101-120000.tar", "camera-appliance-x-20260101-120000.tar.gz",
	} {
		if _, _, err := Open(cfg, name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Open(%q) err = %v", name, err)
		}
		if err := Delete(cfg, name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Delete(%q) err = %v", name, err)
		}
	}
	if _, _, err := Open(cfg, "camera-appliance-20260102-120000.tar.gz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing backup err = %v", err)
	}
}

func TestOpenRefusesSymlinkedBackup(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir()}
	writeBackupFile(t, cfg, "camera-appliance-20260101-120000.tar.gz", "ok")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "camera-appliance-20260102-120000.tar.gz"
	if err := os.Symlink(outside, filepath.Join(cfg.BackupDir(), name)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(cfg, name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("symlink err = %v", err)
	}
	if err := Delete(cfg, name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("symlink delete err = %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("symlink target was touched")
	}
}

func TestOpenReturnsBackupContent(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir()}
	name := "camera-appliance-update-20260101-120000.5.tar.gz"
	writeBackupFile(t, cfg, name, "archive")
	file, entry, err := Open(cfg, name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	if string(data) != "archive" || entry.Kind != KindUpdate || entry.Size != 7 {
		t.Fatalf("entry = %+v data = %q", entry, data)
	}
}

func TestPruneKeepsNewestOfKindAndNeverTouchesOthers(t *testing.T) {
	cfg := config.Config{StateDir: t.TempDir()}
	for day := 1; day <= 9; day++ {
		stamp := time.Date(2026, 1, day, 3, 0, 0, 0, time.UTC).Format(timestampLayout)
		writeBackupFile(t, cfg, "camera-appliance-auto-"+stamp+".tar.gz", "a")
		writeBackupFile(t, cfg, "camera-appliance-"+stamp+".tar.gz", "m")
		writeBackupFile(t, cfg, "camera-appliance-update-"+stamp+".tar.gz", "u")
	}
	removed, err := Prune(cfg, KindAutomatic, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || !strings.Contains(removed[0], "20260102") || !strings.Contains(removed[1], "20260101") {
		t.Fatalf("removed = %v", removed)
	}
	entries, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Kind]int{}
	for _, entry := range entries {
		counts[entry.Kind]++
	}
	if counts[KindAutomatic] != 7 || counts[KindManual] != 9 || counts[KindUpdate] != 9 {
		t.Fatalf("counts = %v", counts)
	}
}

func TestNewPathRoundTripsThroughList(t *testing.T) {
	cfg := newBackupTestConfig(t)
	for _, kind := range []Kind{KindManual, KindAutomatic, KindUpdate} {
		if _, err := Create(context.Background(), cfg, NewPath(cfg, kind), false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Create(context.Background(), cfg, "", false); err != nil {
		t.Fatal(err)
	}
	entries, err := List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Kind]int{}
	for _, entry := range entries {
		counts[entry.Kind]++
		if time.Since(entry.CreatedAt) > time.Minute || entry.Size == 0 {
			t.Fatalf("entry = %+v", entry)
		}
	}
	if counts[KindManual] != 2 || counts[KindAutomatic] != 1 || counts[KindUpdate] != 1 {
		t.Fatalf("counts = %v", counts)
	}
}
