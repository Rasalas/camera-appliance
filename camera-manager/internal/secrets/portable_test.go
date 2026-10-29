package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableEmptyPasswordDoesNotResurrectDestinationCredential(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envKey, "")
	if err := os.WriteFile(filepath.Join(dir, "local.env"), []byte("TAPO_CAMERA_PASSWORD='old-global'\nCAMERA_PASSWORD_DEVICE='old-camera'\nCAMERA_IDENTITY_PASSWORD_IDENTITY='old-identity'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, PortableFile), []byte(`{"TAPO_CAMERA_PASSWORD":"","CAMERA_PASSWORD_DEVICE":"","CAMERA_IDENTITY_PASSWORD_IDENTITY":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets.env"), []byte("TAPO_CAMERA_PASSWORD='old-file'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := ExportPortable(dir, []string{"device"}, []string{"identity"})
	if err != nil {
		t.Fatal(err)
	}
	var values portable
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values[envKey] != "" {
		t.Fatal("re-export resurrected empty global password")
	}
	for _, result := range []Result{Load(dir), LoadCamera(dir, "device"), LoadIdentity(dir, "identity")} {
		if result.Value != "" {
			t.Fatal("restored absence resurrected destination password")
		}
	}
}

func TestExportReportsKeyringFailureInsteadOfLosingCredentials(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv(envKey, "")
	if err := os.WriteFile(filepath.Join(bin, "secret-tool"), []byte("#!/bin/sh\nexit 2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ExportPortable(t.TempDir(), []string{"device"}, nil); err == nil {
		t.Fatal("backup silently omitted failed keyring lookup")
	}
}

func TestPortableRoundTripPreservesGlobalInheritanceAndExplicitOverrides(t *testing.T) {
	t.Setenv(envKey, "")
	t.Setenv("PATH", t.TempDir())
	source, destination := t.TempDir(), t.TempDir()
	if _, err := Save(source, "source-global"); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveCamera(source, "explicit", "camera-override"); err != nil {
		t.Fatal(err)
	}
	data, err := ExportPortable(source, []string{"inherited", "explicit"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveCamera(destination, "inherited", "stale-destination"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, PortableFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	if got := LoadCamera(destination, "inherited"); got.Value != "source-global" {
		t.Fatalf("restored inheritance: %+v", got)
	}
	if _, err := Save(destination, "changed-global"); err != nil {
		t.Fatal(err)
	}
	if got := LoadCamera(destination, "inherited"); got.Value != "changed-global" {
		t.Fatalf("global change did not reach inheriting camera: %+v", got)
	}
	if got := LoadCamera(destination, "explicit"); got.Value != "camera-override" {
		t.Fatalf("explicit override changed: %+v", got)
	}
	data, err = ExportPortable(destination, []string{"inherited", "explicit"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var values portable
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if value, found := values[localEnvCameraKey("inherited")]; !found || value != "" {
		t.Fatal("re-export lost global inheritance")
	}
}
