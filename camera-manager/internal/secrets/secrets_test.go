package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveFallsBackToLocalEnv(t *testing.T) {
	dir := t.TempDir()
	if err := writeLocalEnvKey(dir, envKey, "test-password"); err != nil {
		t.Fatal(err)
	}
	if got := readLocalEnvKey(dir, envKey); got != "test-password" {
		t.Fatalf("unexpected password: %q", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "local.env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "TAPO_CAMERA_PASSWORD='test-password'\n" {
		t.Fatalf("unexpected local.env: %q", string(data))
	}
}

func TestCameraPasswordUsesDeviceScopedKey(t *testing.T) {
	dir := t.TempDir()
	if err := writeLocalEnvKey(dir, localEnvCameraKey("device_abc-123"), "camera-password"); err != nil {
		t.Fatal(err)
	}
	got := LoadCamera(dir, "device_abc-123")
	if got.Value != "camera-password" || got.Source != "local.env" {
		t.Fatalf("unexpected camera secret: %+v", got)
	}
}

func TestIdentityPasswordDoesNotFallBackToGlobal(t *testing.T) {
	dir := t.TempDir()
	if err := writeLocalEnvKey(dir, envKey, "global-password"); err != nil {
		t.Fatal(err)
	}
	got := LoadIdentity(dir, "tapo")
	if got.Value != "" || got.Source != "" {
		t.Fatalf("identity secret should not use global fallback: %+v", got)
	}
	if err := writeLocalEnvKey(dir, localEnvIdentityKey("tapo"), "identity-password"); err != nil {
		t.Fatal(err)
	}
	got = LoadIdentity(dir, "tapo")
	if got.Value != "identity-password" || got.Source != "local.env" {
		t.Fatalf("unexpected identity secret: %+v", got)
	}
}
