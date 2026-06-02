package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

func TestViewerReportsOnlineAliasWithoutLeakingSecrets(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "secret")
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20", Manufacturer: "Tapo", Model: "C310"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Label: "Hof", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:secret@192.168.1.20:554/stream2\n")

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slot := viewer.Slots[0]
	if slot.State != ViewerStateOnline {
		t.Fatalf("expected online slot, got %s: %+v", slot.State, slot.Diagnostics)
	}
	if slot.Playback == nil || !strings.Contains(slot.Playback.PageURL, "src=cam1") {
		t.Fatalf("expected go2rtc playback URL for cam1, got %+v", slot.Playback)
	}
	data, _ := json.Marshal(viewer)
	if strings.Contains(string(data), "secret") {
		t.Fatalf("viewer response leaked secret: %s", data)
	}
	if strings.Contains(string(data), "rtsp://user") {
		t.Fatalf("viewer response leaked direct camera RTSP URL: %s", data)
	}
}

func TestViewerReportsCredentialsFailureBeforeStreamAvailability(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "")
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:redacted@192.168.1.20:554/stream2\n")

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Slots[0].State != ViewerStateCredentialsFailed {
		t.Fatalf("expected credentials failure, got %s", viewer.Slots[0].State)
	}
}

func TestViewerReportsOfflineWhenRTSPPortIsUnavailable(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "secret")
	a.RTSPProbe = func(context.Context, string, string) error { return errors.New("connection refused") }
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:redacted@192.168.1.20:554/stream2\n")

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Slots[0].State != ViewerStateOffline {
		t.Fatalf("expected offline slot, got %s", viewer.Slots[0].State)
	}
	if viewer.Slots[0].Playback != nil {
		t.Fatalf("offline slot should not expose playback metadata: %+v", viewer.Slots[0].Playback)
	}
}

func TestViewerUsesRTSPEndpointOverrideForProbe(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "secret")
	var probedHost, probedPort string
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "192.168.1.20" {
			return errors.New("direct path unavailable")
		}
		probedHost = host
		probedPort = port
		return nil
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.rtsp_endpoint.dev1.host": "host.docker.internal",
		"camera.rtsp_endpoint.dev1.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:redacted@host.docker.internal:15541/stream2\n")

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Slots[0].State != ViewerStateOnline {
		t.Fatalf("expected online slot, got %s", viewer.Slots[0].State)
	}
	if probedHost != "127.0.0.1" || probedPort != "15541" {
		t.Fatalf("expected probe through local relay, got %s:%s", probedHost, probedPort)
	}
	if viewer.Slots[0].Path == nil || viewer.Slots[0].Path.ID != "relay:manual" {
		t.Fatalf("expected selected manual relay path, got %+v", viewer.Slots[0].Path)
	}
}

func TestRenderGo2RTCSelectsRelayFallbackWhenDirectIsUnavailable(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "secret")
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "192.168.1.20" && port == "554" {
			return errors.New("direct path unavailable")
		}
		return nil
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	result, err := a.RenderGo2RTC(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "user:secret@host.docker.internal:15541") {
		t.Fatalf("expected rendered relay endpoint, got %s", data)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["camera.active_path.dev1.id"] != "relay:nas" {
		t.Fatalf("expected active relay path, got %+v", settings)
	}
}

func TestViewerReportsUnassignedSlotsAsUsableEmptyState(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "")

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(viewer.Slots) != len(config.DefaultSlots()) {
		t.Fatalf("expected default slots, got %d", len(viewer.Slots))
	}
	for _, slot := range viewer.Slots {
		if slot.State != ViewerStateUnassigned {
			t.Fatalf("expected %s to be unassigned, got %s", slot.Alias, slot.State)
		}
	}
}

func newViewerTestApp(t *testing.T, go2rtcURL, password string) *App {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := state.Open(ctx, filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	slots := config.DefaultSlots()
	if err := store.UpsertSlots(ctx, slots); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ConfigDir:      dir,
		StateDir:       dir,
		Go2RTCURL:      go2rtcURL,
		Go2RTCRTSPURL:  "rtsp://localhost:8554",
		TapoPassword:   password,
		RequestTimeout: 100 * time.Millisecond,
	}
	return &App{
		Config: cfg,
		Store:  store,
		Slots:  slots,
		RTSPProbe: func(context.Context, string, string) error {
			return nil
		},
	}
}

func writeGeneratedGo2RTC(t *testing.T, cfg config.Config, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(cfg.Go2RTCConfigPath()), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Go2RTCConfigPath(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
