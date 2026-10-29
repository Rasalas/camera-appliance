package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/cameraaccess"
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
