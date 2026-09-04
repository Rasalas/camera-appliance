package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

func newBackupTestConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{ConfigDir: filepath.Join(dir, "etc"), StateDir: filepath.Join(dir, "var")}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath()), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.ConfigDir, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := state.Open(context.Background(), cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ConfigDir, "local.env"), []byte("TAPO_CAMERA_PASSWORD=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCreateArchiveHasRestrictedPermissions(t *testing.T) {
	cfg := newBackupTestConfig(t)
	out := filepath.Join(cfg.BackupDir(), "backup.tar.gz")
	result, err := Create(context.Background(), cfg, out, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("backup archive must not be group/world accessible, got %o", perm)
	}
	includedLocalEnv := false
	for _, name := range result.Files {
		if name == "etc/camera-appliance/local.env" {
			includedLocalEnv = true
		}
	}
	if !includedLocalEnv {
		t.Fatalf("expected local.env to be listed as included, got %v", result.Files)
	}
	if !strings.Contains(result.Warning, "Zugangsdaten") {
		t.Fatalf("warning must disclose credential content, got %q", result.Warning)
	}
}

func TestRestoreClampsArchivePermissions(t *testing.T) {
	cfg := newBackupTestConfig(t)
	archive := filepath.Join(t.TempDir(), "crafted.tar.gz")
	db, err := os.ReadFile(cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	buildTar(t, archive, map[string]tarEntry{
		"var/lib/camera-appliance/state.db": {content: string(db), mode: 0o777},
		"etc/camera-appliance/local.env":    {content: "PASSWORD=secret\n", mode: 0o644},
	})
	if _, err := Restore(context.Background(), cfg, archive); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{cfg.DBPath(), filepath.Join(cfg.ConfigDir, "local.env")} {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("%s permissions must be clamped, got %o", target, perm)
		}
	}
}

type tarEntry struct {
	content string
	mode    int64
}

func buildTar(t *testing.T, path string, files map[string]tarEntry) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(file)
	tw := tar.NewWriter(gw)
	for name, item := range files {
		header := &tar.Header{Name: name, Mode: item.mode, Size: int64(len(item.content))}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(item.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
