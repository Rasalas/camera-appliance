package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/relay"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

func cameraConfigTestApp(t *testing.T) *App {
	t.Helper()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "test-password")
	if err := a.Store.UpsertDevice(context.Background(), state.Device{ID: "dev1", LastIP: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(context.Background(), state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "old-user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPasswordOnlyRetryAfterDatabaseFailureAppliesSavedSecret(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	t.Setenv("PATH", t.TempDir())
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", a.Config.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_camera_save BEFORE UPDATE OF id ON devices BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`); err != nil {
		t.Fatal(err)
	}
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error { restarts++; return nil }
	req := CameraConfiguration{Access: &cameraaccess.CredentialsInput{Username: "old-user", Stream: "stream2", Password: "replacement-password"}}
	result, err := a.ConfigureCamera(ctx, "dev1", req)
	if err == nil || result.Persisted || !result.PasswordPersisted {
		t.Fatalf("expected partial persistence: %+v %v", result, err)
	}
	if secrets.LoadCamera(a.Config.ConfigDir, "dev1").Value != req.Access.Password {
		t.Fatal("secret was not saved before injected failure")
	}
	if _, err := db.Exec(`DROP TRIGGER fail_camera_save`); err != nil {
		t.Fatal(err)
	}
	result, err = a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Applied || !result.Restarted || restarts != 1 {
		t.Fatalf("retry skipped apply: %+v %v, restarts %d", result, err, restarts)
	}
	data, err := os.ReadFile(a.Config.Go2RTCConfigPath())
	if err != nil || !strings.Contains(string(data), "replacement-password@") {
		t.Fatal("retry retained the old rendered secret", err)
	}
}

func TestCameraConfigurationRetryDoesNotRestartOtherCamerasRelays(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev2", LastIP: "192.0.2.2"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam2", DeviceID: "dev2", Username: "old-user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids": "nas,spare", "camera.relay.nas.ssh_target": "nas", "camera.relay.nas.host": "127.0.0.1",
		"camera.relay.spare.ssh_target": "spare", "camera.relay.spare.host": "127.0.0.1",
	}); err != nil {
		t.Fatal(err)
	}
	a.Relays().Probe = func(context.Context, string, string) error { return errors.New("closed") }
	var starts []string
	a.Relays().StartProcess = func(_ context.Context, r relay.ManagedRelay) (int, error) {
		starts = append(starts, r.ID)
		return os.Getpid(), nil
	}
	a.Go2RTCRestart = func(context.Context) error { return errors.New("go2rtc failed") }
	req := CameraConfiguration{RelayEndpoints: map[string]CameraEndpoint{"nas": {TargetHost: "192.0.2.91"}}}
	result, err := a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || result.Applied || strings.Join(starts, ",") != "nas" {
		t.Fatalf("first apply: %+v %v starts %v", result, err, starts)
	}
	a.Go2RTCRestart = func(context.Context) error { return nil }
	result, err = a.ConfigureCamera(ctx, "dev2", CameraConfiguration{RelayEndpoints: map[string]CameraEndpoint{"spare": {TargetHost: "192.0.2.92"}}})
	if err != nil || !result.Applied || strings.Join(starts, ",") != "nas,spare" {
		t.Fatalf("second camera: %+v %v starts %v", result, err, starts)
	}
	// A fresh manager must recover the operation scope from persisted retry data.
	a.relays = relay.New(a.Config, a.Store, a.Slots, a.probeRTSP)
	a.Relays().Probe = func(context.Context, string, string) error { return errors.New("closed") }
	a.Relays().StartProcess = func(_ context.Context, r relay.ManagedRelay) (int, error) {
		starts = append(starts, r.ID)
		return os.Getpid(), nil
	}
	result, err = a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Applied || strings.Join(starts, ",") != "nas,spare,nas" {
		t.Fatalf("retry touched unrelated relay: %+v %v starts %v", result, err, starts)
	}
}

func TestSavedCameraCredentialsReplaceBoundCredentials(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	service := cameraaccess.New(a.Store, a.Config, a.StreamEndpointForDevice)
	if _, err := service.SaveCredentials(ctx, "dev1", cameraaccess.CredentialsInput{Username: "new-user", Stream: "custom/path"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.Config.Go2RTCConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "old-user") || !strings.Contains(string(data), "new-user:") || !strings.Contains(string(data), "/custom/path") {
		t.Fatalf("stale render: %s", data)
	}
	bindings, err := a.Store.Bindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bindings[0].Username != "new-user" || bindings[0].StreamName != "custom/path" {
		t.Fatalf("stale binding: %+v", bindings[0])
	}
	// Legacy binding API remains a compatible write path and updates access reads.
	bindings[0].Username = "legacy-user"
	if err := a.Store.UpsertBinding(ctx, bindings[0]); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Credentials(ctx, "dev1")
	if err != nil || credentials.Username != "legacy-user" {
		t.Fatalf("credentials: %+v, %v", credentials, err)
	}
}

func TestCameraConfigurationApplyAndRetry(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error { restarts++; return nil }
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		t.Fatal(err)
	}
	visible := true
	req := CameraConfiguration{Visible: &visible, Display: map[string]string{"rotation": "90"}}
	result, err := a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Persisted || !result.Applied || restarts != 0 {
		t.Fatalf("display save: %+v, %v, restarts %d", result, err, restarts)
	}
	req = CameraConfiguration{Access: &cameraaccess.CredentialsInput{Username: "new-user", Stream: "stream1"}}
	a.Go2RTCRestart = func(context.Context) error { restarts++; return errors.New("restart failed") }
	result, err = a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Persisted || result.Applied || result.Error == "" || restarts != 1 {
		t.Fatalf("failed apply: %+v, %v, restarts %d", result, err, restarts)
	}
	a.Go2RTCRestart = func(context.Context) error { restarts++; return nil }
	result, err = a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Applied || !result.Restarted || restarts != 2 {
		t.Fatalf("retry: %+v, %v, restarts %d", result, err, restarts)
	}
	result, err = a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Applied || result.Restarted || restarts != 2 {
		t.Fatalf("idempotent save: %+v, %v, restarts %d", result, err, restarts)
	}
}

func TestCameraConfigurationAppliesEndpointAndVisibility(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error { restarts++; return nil }
	policy := "relay_only"
	result, err := a.ConfigureCamera(ctx, "dev1", CameraConfiguration{PathPolicy: &policy, Endpoint: &CameraEndpoint{Host: "relay.example", Port: "15541"}})
	if err != nil || !result.Applied || !result.Restarted {
		t.Fatalf("endpoint save: %+v %v", result, err)
	}
	data, err := os.ReadFile(a.Config.Go2RTCConfigPath())
	if err != nil || !strings.Contains(string(data), "relay.example:15541") {
		t.Fatalf("endpoint not applied: %s %v", data, err)
	}
	visible := false
	result, err = a.ConfigureCamera(ctx, "dev1", CameraConfiguration{Visible: &visible})
	if err != nil || !result.Applied || !result.Restarted {
		t.Fatalf("hide: %+v %v", result, err)
	}
	bindings, err := a.Store.Bindings(ctx)
	if err != nil || len(bindings) != 0 {
		t.Fatalf("bindings: %+v %v", bindings, err)
	}
	visible = true
	result, err = a.ConfigureCamera(ctx, "dev1", CameraConfiguration{Visible: &visible})
	if err != nil || !result.Applied || !result.Restarted || restarts != 3 {
		t.Fatalf("show: %+v %v restarts %d", result, err, restarts)
	}
	bindings, err = a.Store.Bindings(ctx)
	if err != nil || len(bindings) != 1 || bindings[0].SlotID != "cam1" || bindings[0].Username != "old-user" {
		t.Fatalf("new binding: %+v %v", bindings, err)
	}
}

func TestCameraConfigurationReconcilesRelayTargetAndDurableRetry(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids": "nas,unused", "camera.relay.nas.ssh_target": "nas", "camera.relay.nas.type": "ssh_local_forward", "camera.relay.nas.host": "127.0.0.1",
		"camera.relay.unused.ssh_target": "unused", "camera.relay.unused.type": "ssh_local_forward", "camera.path_policy.dev1": "auto",
	}); err != nil {
		t.Fatal(err)
	}
	a.Relays().Probe = func(context.Context, string, string) error { return errors.New("closed") }
	a.Go2RTCRestart = func(context.Context) error { return nil }
	if _, err := a.RenderGo2RTC(ctx); err != nil {
		t.Fatal(err)
	}
	// Establish a rendered baseline without touching processes, then change only
	// the SSH target. Its host never appears in the go2rtc YAML.
	before, err := os.ReadFile(a.Config.Go2RTCConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	starts := 0
	a.Relays().StartProcess = func(_ context.Context, r relay.ManagedRelay) (int, error) {
		starts++
		if r.ID != "nas" || len(r.Endpoints) != 1 || r.Endpoints[0].TargetHost != "192.0.2.99" || r.Endpoints[0].TargetPort != "8554" {
			t.Fatalf("wrong desired forwards: %+v", r)
		}
		return 0, errors.New("ssh unavailable")
	}
	req := CameraConfiguration{RelayEndpoints: map[string]CameraEndpoint{"nas": {TargetHost: "192.0.2.99", TargetPort: "8554"}}}
	result, err := a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Persisted || result.Applied || !strings.Contains(result.Error, "ssh unavailable") || starts != 1 {
		t.Fatalf("failed apply: %+v %v starts %d", result, err, starts)
	}
	settings, _ := a.Store.Settings(ctx)
	if settings["camera.configuration.dev1.relay_before"] == "" || settings["camera.configuration.dev1.pending"] != "true" {
		t.Fatal("retry was not durable")
	}
	// Replace the manager, as after an application restart. The retry must use
	// saved pre-change definitions rather than comparing desired state to itself.
	a.relays = relay.New(a.Config, a.Store, a.Slots, a.probeRTSP)
	a.Relays().Probe = func(context.Context, string, string) error { return errors.New("closed") }
	a.Relays().StartProcess = func(_ context.Context, r relay.ManagedRelay) (int, error) {
		starts++
		if r.ID != "nas" || r.Endpoints[0].TargetHost != "192.0.2.99" {
			t.Fatalf("wrong retry: %+v", r)
		}
		return os.Getpid(), nil
	}
	result, err = a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || !result.Applied || starts != 2 {
		t.Fatalf("retry: %+v %v starts %d", result, err, starts)
	}
	after, _ := os.ReadFile(a.Config.Go2RTCConfigPath())
	if string(before) != string(after) {
		t.Fatal("target-only change altered go2rtc YAML")
	}
	a.Relays().Probe = func(context.Context, string, string) error { t.Fatal("display save probed relay"); return nil }
	a.Relays().StopProcess = func(context.Context, relay.Status) error { t.Fatal("display save stopped relay"); return nil }
	result, err = a.ConfigureCamera(ctx, "dev1", CameraConfiguration{Display: map[string]string{"rotation": "90"}})
	if err != nil || !result.Applied || starts != 2 {
		t.Fatalf("display touched relay: %+v %v starts %d", result, err, starts)
	}
}

func TestCameraConfigurationRevertAfterPartialApplyStillReconcilesRelay(t *testing.T) {
	a := cameraConfigTestApp(t)
	ctx := context.Background()
	if err := a.Store.PutSettings(ctx, map[string]string{"camera.relay.ids": "nas", "camera.relay.nas.ssh_target": "nas", "camera.relay.nas.host": "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	a.Relays().Probe = func(context.Context, string, string) error { return errors.New("closed") }
	var targets []string
	a.Relays().StartProcess = func(_ context.Context, r relay.ManagedRelay) (int, error) {
		targets = append(targets, r.Endpoints[0].TargetHost)
		return os.Getpid(), nil
	}
	a.Go2RTCRestart = func(context.Context) error { return errors.New("go2rtc failed") }
	req := CameraConfiguration{Access: &cameraaccess.CredentialsInput{Username: "changed", Stream: "stream1"}, RelayEndpoints: map[string]CameraEndpoint{"nas": {TargetHost: "192.0.2.99"}}}
	result, err := a.ConfigureCamera(ctx, "dev1", req)
	if err != nil || result.Applied || len(targets) != 1 {
		t.Fatalf("partial apply %+v %v targets %v", result, err, targets)
	}
	a.Go2RTCRestart = func(context.Context) error { return nil }
	result, err = a.ConfigureCamera(ctx, "dev1", CameraConfiguration{RelayEndpoints: map[string]CameraEndpoint{"nas": {}}})
	if err != nil || !result.Applied || len(targets) != 2 || targets[1] != "192.0.2.1" {
		t.Fatalf("revert %+v %v targets %v", result, err, targets)
	}
}
