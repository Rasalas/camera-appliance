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

func TestWriteLocalEnvRoundTripPreservesSpecialPasswords(t *testing.T) {
	dir := t.TempDir()
	cases := []string{
		"plain-password",
		"trailing'quote",
		"has \"double\" quotes",
		"spaces and #hash",
	}
	for _, want := range cases {
		if err := writeLocalEnvKey(dir, "test.key", want); err != nil {
			t.Fatal(err)
		}
		if got := readLocalEnvKey(dir, "test.key"); got != want {
			t.Fatalf("round trip failed: got %q want %q", got, want)
		}
	}
}

func TestWriteLocalEnvTightensPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.env")
	if err := os.WriteFile(path, []byte("OTHER=value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalEnvKey(dir, "test.key", "secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("local.env must be tightened to owner-only, got %o", perm)
	}
}

func TestRemoveLocalEnvKey(t *testing.T) {
	dir := t.TempDir()
	if err := writeLocalEnvKey(dir, "keep.key", "stay"); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalEnvKey(dir, "drop.key", "gone"); err != nil {
		t.Fatal(err)
	}
	removeLocalEnvKey(dir, "drop.key")
	if readLocalEnvKey(dir, "drop.key") != "" {
		t.Fatal("dropped key should be removed")
	}
	if readLocalEnvKey(dir, "keep.key") != "stay" {
		t.Fatal("other keys must survive")
	}
}
