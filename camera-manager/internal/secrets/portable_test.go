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
