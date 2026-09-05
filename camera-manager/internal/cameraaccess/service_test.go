package cameraaccess_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/go2rtc"
	"camera-appliance/camera-manager/internal/state"
)

func newService(t *testing.T) (*cameraaccess.Service, *state.Store) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{ConfigDir: dir, StateDir: dir, CaptureSSHHost: "capture-host"}
	store, err := state.Open(context.Background(), cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.UpsertDevice(context.Background(), state.Device{ID: "device", LastIP: "192.0.2.1", MACAddress: "aa:bb:cc:dd:ee:ff"}); err != nil {
		t.Fatal(err)
	}
	s := cameraaccess.New(store, cfg, func(context.Context, state.Device) (go2rtc.StreamEndpoint, error) {
		return go2rtc.StreamEndpoint{Host: "host.docker.internal", Port: "15541"}, nil
	})
	return s, store
}

func TestFrameTriesIdentityAndPersistsReferenceWithoutExposingSecrets(t *testing.T) {
	ctx := context.Background()
	s, store := newService(t)
	identity, err := s.SaveIdentity(ctx, cameraaccess.IdentityInput{ID: "office", Name: "Büro", Username: "right-user", Password: "right secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !identity.PasswordSet {
		t.Fatal("identity password missing")
	}
	calls := 0
	s.Capture = func(_ context.Context, rawURL, host string) ([]byte, error) {
		calls++
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		password, _ := parsed.User.Password()
		if host != "capture-host" || parsed.Host != "host.docker.internal:15541" {
			t.Fatalf("capture adapter inputs host=%s endpoint=%s", host, parsed.Host)
		}
		if parsed.User.Username() != "right-user" {
			return nil, &exec.ExitError{Stderr: []byte("failed rtsp://wrong:wrong-secret@camera/stream2")}
		}
		if password != "right secret" {
			t.Fatal("password whitespace changed")
		}
		return []byte("jpeg content"), nil
	}
	got, err := s.Frame(ctx, "device", cameraaccess.FrameInput{CredentialsInput: cameraaccess.CredentialsInput{Username: "wrong", Password: "wrong-secret"}, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || got.IdentityID != "office" || got.ImageBase64 == "" || got.SHA256 == "" {
		t.Fatalf("frame %+v calls=%d", got, calls)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "right secret") || strings.Contains(string(encoded), "wrong-secret") {
		t.Fatal("frame response leaked a password")
	}
	path, err := s.ReferenceImage(ctx, "device")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "jpeg content" {
		t.Fatalf("reference %q %v", data, err)
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["camera.credentials.device.identity_id"] != "office" {
		t.Fatal("successful identity was not remembered")
	}
	credentials, err := s.Credentials(ctx, "device")
	if err != nil || credentials.Username != "right-user" || !credentials.PasswordSet {
		t.Fatalf("credentials %+v %v", credentials, err)
	}
}

func TestIdentityCopyAndDeleteUseOneCredentialOwner(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.SaveIdentity(ctx, cameraaccess.IdentityInput{ID: "one", Name: "One", Username: "user", Password: "copy-secret"}); err != nil {
		t.Fatal(err)
	}
	copied, err := s.SaveIdentity(ctx, cameraaccess.IdentityInput{ID: "two", Name: "Two", Username: "user", CopyPasswordFromID: "one"})
	if err != nil || !copied.PasswordSet {
		t.Fatalf("copy %+v %v", copied, err)
	}
	if err := s.DeleteIdentity(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	list, err := s.Identities(ctx)
	if err != nil || len(list) != 1 || list[0].ID != "two" || !list[0].PasswordSet {
		t.Fatalf("identities %+v %v", list, err)
	}
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "copy-secret") {
		t.Fatal("identity list leaked its password")
	}
}

func TestFailuresHaveDomainKindsAndRedactedCaptureDetails(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	_, err := s.Frame(ctx, "missing", cameraaccess.FrameInput{})
	var failure *cameraaccess.Failure
	if !errors.As(err, &failure) || failure.Kind != cameraaccess.NotFound {
		t.Fatalf("missing camera error %v", err)
	}
	_, err = s.Frame(ctx, "device", cameraaccess.FrameInput{})
	if !errors.As(err, &failure) || failure.Kind != cameraaccess.InvalidInput {
		t.Fatalf("missing credentials error %v", err)
	}
	s.Capture = func(context.Context, string, string) ([]byte, error) {
		return nil, &exec.ExitError{Stderr: []byte("cannot open rtsp://user:sensitive-password@camera/stream2")}
	}
	_, err = s.Frame(ctx, "device", cameraaccess.FrameInput{CredentialsInput: cameraaccess.CredentialsInput{Username: "user", Password: "sensitive-password"}})
	if !errors.As(err, &failure) || failure.Kind != cameraaccess.CaptureFailed || strings.Contains(err.Error(), "sensitive-password") {
		t.Fatalf("capture failure %v", err)
	}
	if err := store.PutSettings(ctx, map[string]string{"camera.reference_image.device": filepath.Join(t.TempDir(), "outside.jpg")}); err != nil {
		t.Fatal(err)
	}
	_, err = s.ReferenceImage(ctx, "device")
	if !errors.As(err, &failure) || failure.Kind != cameraaccess.InvalidInput {
		t.Fatalf("reference escaped owned directory: %v", err)
	}
}
