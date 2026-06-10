package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

func TestApplyReleaseCreatesBackupAndCopiesFiles(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	installDir := newTestInstall(t, "old")
	archive := newReleaseArchive(t, map[string]string{
		"camera-appliance/manifest.json":                    `{"version":"1.2.3","commit":"abc123","build_time":"2026-06-02T00:00:00Z"}`,
		"camera-appliance/bin/camera-appliance":             "new",
		"camera-appliance/frontend/dist/index.html":         "<html>new</html>",
		"camera-appliance/compose.yaml":                     "services: {}\n",
		"camera-appliance/systemd/camera-appliance.service": "[Service]\n",
	})

	result, err := Apply(ctx, Options{
		Config:       cfg,
		Archive:      archive,
		InstallDir:   installDir,
		NoRestart:    true,
		AutoRollback: true,
		Now:          fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath == "" || !pathExists(result.BackupPath) {
		t.Fatalf("expected backup path to exist, got %+v", result)
	}
	if result.RollbackDir == "" || !pathExists(filepath.Join(result.RollbackDir, "bin", "camera-appliance")) {
		t.Fatalf("expected rollback snapshot, got %+v", result)
	}
	if got := readFile(t, filepath.Join(installDir, "bin", "camera-appliance")); got != "new" {
		t.Fatalf("expected new binary, got %q", got)
	}
	if pathExists(filepath.Join(installDir, "frontend", "dist", "old.txt")) {
		t.Fatalf("expected old frontend dist to be replaced")
	}
	if got := readFile(t, filepath.Join(installDir, "frontend", "dist", "index.html")); got != "<html>new</html>" {
		t.Fatalf("expected new frontend dist, got %q", got)
	}
	if !pathExists(filepath.Join(cfg.BackupDir(), lastUpdateFile)) {
		t.Fatalf("expected update metadata file")
	}
}

func TestApplyRollsBackWhenHealthcheckFails(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	installDir := newTestInstall(t, "old")
	archive := newReleaseArchive(t, map[string]string{
		"camera-appliance/manifest.json":        `{"version":"bad","commit":"bad"}`,
		"camera-appliance/bin/camera-appliance": "new",
	})

	result, err := Apply(ctx, Options{
		Config:       cfg,
		Archive:      archive,
		InstallDir:   installDir,
		NoRestart:    true,
		AutoRollback: true,
		Healthcheck: func(context.Context) error {
			return errors.New("viewer offline")
		},
		Now: fixedNow,
	})
	if err == nil {
		t.Fatal("expected healthcheck error")
	}
	if !result.RollbackApplied {
		t.Fatalf("expected automatic rollback, got %+v", result)
	}
	if got := readFile(t, filepath.Join(installDir, "bin", "camera-appliance")); got != "old" {
		t.Fatalf("expected old binary after rollback, got %q", got)
	}
}

func TestApplyRejectsReleaseArchiveWithSecrets(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	installDir := newTestInstall(t, "old")
	archive := newReleaseArchive(t, map[string]string{
		"camera-appliance/bin/camera-appliance": "new",
		"camera-appliance/secrets.env":          "TAPO_CAMERA_PASSWORD=leak",
	})

	_, err := Apply(ctx, Options{
		Config:       cfg,
		Archive:      archive,
		InstallDir:   installDir,
		NoRestart:    true,
		AutoRollback: true,
		Now:          fixedNow,
	})
	if err == nil || !strings.Contains(err.Error(), "forbidden path") {
		t.Fatalf("expected forbidden path error, got %v", err)
	}
}

func TestRollbackRestoresLastSnapshot(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	installDir := newTestInstall(t, "old")
	archive := newReleaseArchive(t, map[string]string{
		"camera-appliance/manifest.json":        `{"version":"1.2.3","commit":"abc123"}`,
		"camera-appliance/bin/camera-appliance": "new",
	})
	if _, err := Apply(ctx, Options{
		Config:       cfg,
		Archive:      archive,
		InstallDir:   installDir,
		NoRestart:    true,
		AutoRollback: true,
		Now:          fixedNow,
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(installDir, "bin", "camera-appliance"), "broken")

	result, err := Rollback(ctx, RollbackOptions{Config: cfg, NoRestart: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.RollbackApplied {
		t.Fatalf("expected manual rollback result, got %+v", result)
	}
	if got := readFile(t, filepath.Join(installDir, "bin", "camera-appliance")); got != "old" {
		t.Fatalf("expected old binary after manual rollback, got %q", got)
	}
}

func TestInstallCopiesReleaseAndInitializesRuntimeFiles(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	installDir := t.TempDir()
	archive := newReleaseArchive(t, map[string]string{
		"camera-appliance/manifest.json":        `{"version":"1.2.3","commit":"abc123"}`,
		"camera-appliance/.env.example":         "TAPO_CAMERA_PASSWORD=change-me\nADMIN_SESSION_SECRET=change-me\n",
		"camera-appliance/bin/camera-appliance": "new",
		"camera-appliance/compose.yaml":         "services: {}\n",
	})
	if err := os.Remove(cfg.Go2RTCConfigPath()); err != nil {
		t.Fatal(err)
	}

	result, err := Install(ctx, InstallOptions{
		Config:            cfg,
		Archive:           archive,
		InstallDir:        installDir,
		NoStart:           true,
		AllowNonRoot:      true,
		SkipCommandChecks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(installDir, "bin", "camera-appliance")); got != "new" {
		t.Fatalf("expected installed binary, got %q", got)
	}
	if got := readFile(t, filepath.Join(cfg.ConfigDir, "secrets.env")); !strings.Contains(got, "change-me") {
		t.Fatalf("expected copied secrets template, got %q", got)
	}
	if got := readFile(t, cfg.Go2RTCConfigPath()); got != "streams: {}\n" {
		t.Fatalf("expected initial go2rtc config, got %q", got)
	}
	if !result.SecretsCreated || !result.Go2RTCInitialized {
		t.Fatalf("expected created runtime files, got %+v", result)
	}
}

func newTestConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(filepath.Join(stateDir, "generated"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stateDir, "generated", "go2rtc.yaml"), "streams: {}\n")
	return config.Config{
		StateDir:      stateDir,
		ConfigDir:     configDir,
		BindAddr:      "127.0.0.1:8091",
		Go2RTCURL:     "http://127.0.0.1:1984",
		Go2RTCRTSPURL: "rtsp://127.0.0.1:8554",
	}
}

func newTestInstall(t *testing.T, binary string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin", "camera-appliance"), binary)
	if err := os.Chmod(filepath.Join(dir, "bin", "camera-appliance"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "frontend", "dist", "old.txt"), "old")
	writeFile(t, filepath.Join(dir, "compose.yaml"), "old compose")
	return dir
}

func newReleaseArchive(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(file)
	tw := tar.NewWriter(gw)
	for name, data := range files {
		header := &tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len(data)),
			ModTime: fixedNow(),
		}
		if strings.HasSuffix(name, "bin/camera-appliance") {
			header.Mode = 0o755
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
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
	return path
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
