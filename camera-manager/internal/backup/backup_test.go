package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/secrets"
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
	if !strings.Contains(result.Warning, "Kamera-Passwörter") {
		t.Fatalf("warning must disclose credential content, got %q", result.Warning)
	}
}

func TestUploadPasswordSurvivesProtectedBackupRestore(t *testing.T) {
	cfg := newBackupTestConfig(t)
	if err := secrets.SaveUpload(cfg.ConfigDir, "test-target", "local-upload-test-password"); err != nil {
		t.Fatal(err)
	}
	result, err := Create(context.Background(), cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := secrets.SaveUpload(cfg.ConfigDir, "test-target", "changed-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(context.Background(), cfg, result.Path); err != nil {
		t.Fatal(err)
	}
	p, err := secrets.LoadUpload(cfg.ConfigDir, "test-target")
	if err != nil || p != "local-upload-test-password" {
		t.Fatalf("password was not restored: %v", err)
	}
	info, err := os.Stat(filepath.Join(cfg.ConfigDir, secrets.UploadPasswordFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("restored password permissions: %v", err)
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

func TestKeyringCredentialsSurviveReplacementMachine(t *testing.T) {
	ctx := context.Background()
	cfg := newBackupTestConfig(t)
	bin := t.TempDir()
	ring := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_KEYRING", ring)
	t.Setenv("TAPO_CAMERA_PASSWORD", "")
	tool := `#!/bin/sh
for account do :; done
case "$1" in
store) cat > "$TEST_KEYRING/$account" ;;
lookup) cat "$TEST_KEYRING/$account" 2>/dev/null ;;
clear) rm -f "$TEST_KEYRING/$account" ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "secret-tool"), []byte(tool), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.UpsertDevice(ctx, state.Device{ID: "stable-device"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutSettings(ctx, map[string]string{"camera.identity.ids": "unused-identity"}); err != nil {
		t.Fatal(err)
	}
	for _, save := range []func() (string, error){
		func() (string, error) { return secrets.Save(cfg.ConfigDir, "global-secret") },
		func() (string, error) { return secrets.SaveCamera(cfg.ConfigDir, "stable-device", "camera-secret") },
		func() (string, error) {
			return secrets.SaveIdentity(cfg.ConfigDir, "unused-identity", "identity-secret")
		},
	} {
		source, err := save()
		if err != nil || source != "keyring" {
			t.Fatalf("save %s %v", source, err)
		}
	}
	archive, err := Create(ctx, cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(ring); err != nil {
		t.Fatal(err)
	}
	// An open store survives restore, and the old machine's keyring is gone.
	if _, err := Restore(ctx, cfg, archive.Path); err != nil {
		t.Fatal(err)
	}
	if got := secrets.Load(cfg.ConfigDir).Value; got != "global-secret" {
		t.Fatalf("global credential lost")
	}
	if got := secrets.LoadCamera(cfg.ConfigDir, "stable-device").Value; got != "camera-secret" {
		t.Fatalf("camera credential lost")
	}
	if got := secrets.LoadIdentity(cfg.ConfigDir, "unused-identity").Value; got != "identity-secret" {
		t.Fatalf("identity credential lost")
	}
	info, err := os.Stat(filepath.Join(cfg.ConfigDir, secrets.PortableFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credential permissions %o", info.Mode().Perm())
	}
	if _, err := secrets.SaveCamera(cfg.ConfigDir, "stable-device", "changed-secret"); err != nil {
		t.Fatal(err)
	}
	if secrets.LoadCamera(cfg.ConfigDir, "stable-device").Value != "changed-secret" {
		t.Fatal("restored credential shadows edit")
	}
}

func TestLegacyArchiveReplacesPortableCredentials(t *testing.T) {
	ctx := context.Background()
	cfg := newBackupTestConfig(t)
	t.Setenv("TAPO_CAMERA_PASSWORD", "")
	archive, err := Create(ctx, cfg, "", false)
	if err != nil {
		t.Fatal(err)
	}
	// Construct the installed legacy format without a portable credentials entry.
	legacy := filepath.Join(t.TempDir(), "legacy.tar.gz")
	source, err := os.Open(archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	gz, err := gzip.NewReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	dest, err := os.Create(legacy)
	if err != nil {
		t.Fatal(err)
	}
	outgz := gzip.NewWriter(dest)
	outtar := tar.NewWriter(outgz)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "etc/camera-appliance/"+secrets.PortableFile {
			continue
		}
		if err := outtar.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(outtar, tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(outtar.Close(), outgz.Close(), dest.Close()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ConfigDir, secrets.PortableFile), []byte(`{"TAPO_CAMERA_PASSWORD":"stale"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, cfg, legacy); err != nil {
		t.Fatal(err)
	}
	if secrets.Load(cfg.ConfigDir).Value != "x" {
		t.Fatal("legacy local.env shadowed by stale portable credentials")
	}
}

func TestLegacyRestoreDoesNotInheritDestinationSecrets(t *testing.T) {
	for _, global := range []string{"source-global", ""} {
		t.Run("global="+global, func(t *testing.T) {
			ctx := context.Background()
			cfg := newBackupTestConfig(t)
			t.Setenv("TAPO_CAMERA_PASSWORD", "")
			store, err := state.Open(ctx, cfg.DBPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := store.UpsertDevice(ctx, state.Device{ID: "inherited"}); err != nil {
				t.Fatal(err)
			}
			if err := store.UpsertDevice(ctx, state.Device{ID: "explicit"}); err != nil {
				t.Fatal(err)
			}
			if err := store.PutSettings(ctx, map[string]string{"camera.identity.ids": "identity"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := os.ReadFile(cfg.DBPath())
			if err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(t.TempDir(), "legacy.tar.gz")
			buildTar(t, archive, map[string]tarEntry{
				"var/lib/camera-appliance/state.db": {content: string(db), mode: 0600},
				"etc/camera-appliance/local.env":    {content: "TAPO_CAMERA_PASSWORD='" + global + "'\nCAMERA_PASSWORD_EXPLICIT='source-camera'\n", mode: 0600},
			})
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			if err := os.WriteFile(filepath.Join(bin, "secret-tool"), []byte("#!/bin/sh\nprintf '%s\\n' destination-secret\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := Restore(ctx, cfg, archive); err != nil {
				t.Fatal(err)
			}
			if got := secrets.LoadCamera(cfg.ConfigDir, "inherited"); got.Value != global {
				t.Fatal("legacy camera inherited destination credential")
			}
			if got := secrets.LoadCamera(cfg.ConfigDir, "explicit"); got.Value != "source-camera" {
				t.Fatal("legacy explicit override was lost")
			}
			if got := secrets.LoadIdentity(cfg.ConfigDir, "identity"); got.Value != "" {
				t.Fatal("legacy identity inherited destination credential")
			}
		})
	}
}
