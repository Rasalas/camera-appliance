package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:secret@192.168.1.20:554/stream2\n  cam1_stream1:\n    - rtsp://user:secret@192.168.1.20:554/stream1\n")

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
	if strings.Contains(slot.Playback.PageURL, "webrtc") || !strings.Contains(slot.Playback.PageURL, "mode=mse%2Cmp4%2Cmjpeg") {
		t.Fatalf("expected MSE-first kiosk playback URL, got %+v", slot.Playback)
	}
	if !strings.Contains(slot.Playback.HDPageURL, "src=cam1_stream1") {
		t.Fatalf("expected HD playback URL for cam1_stream1, got %+v", slot.Playback)
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

func TestViewerIncludesDisplayTransformAndLayout(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "secret")
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam5", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.display.dev1.rotation":     "90",
		"camera.display.dev1.mirror":       "true",
		"camera.display.dev1.flip":         "true",
		"camera.display.dev1.fit_mode":     "contain",
		"camera.display.dev1.crop_x":       "20",
		"camera.display.dev1.crop_y":       "10",
		"camera.display.dev1.crop_width":   "60",
		"camera.display.dev1.crop_height":  "80",
		"viewer.layout.mode":               ViewerLayoutFocusRight,
		"viewer.layout.focus_slot_id":      "cam5",
		"viewer.layout.slot_order":         "cam3,cam1,missing,cam3",
		"viewer.layout.split_percent":      "63",
		"viewer.layout.gap_px":             "6",
		"camera.display.dev1.ignored_test": "kept only in settings",
	}); err != nil {
		t.Fatal(err)
	}

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Layout.Mode != ViewerLayoutFocusRight || viewer.Layout.FocusSlotID != "cam5" || viewer.Layout.SplitPercent != 63 || viewer.Layout.GapPX != 6 {
		t.Fatalf("unexpected viewer layout: %+v", viewer.Layout)
	}
	if viewer.Layout.ID != ViewerLayoutFourPlusLarge || viewer.Layout.Name == "" || len(viewer.Layout.Options) < 2 {
		t.Fatalf("expected named layout options, got %+v", viewer.Layout)
	}
	expectedOrder := []string{"cam3", "cam1", "cam2", "cam4", "cam5"}
	if !slices.Equal(viewer.Layout.SlotOrder, expectedOrder) {
		t.Fatalf("unexpected slot order: %+v", viewer.Layout.SlotOrder)
	}
	if len(viewer.Layout.Cells) != 5 || viewer.Layout.Cells[0].SlotID != "cam5" || viewer.Layout.Cells[1].SlotID != "cam3" {
		t.Fatalf("expected focus layout cells, got %+v", viewer.Layout.Cells)
	}
	slot := viewer.Slots[len(viewer.Slots)-1]
	if slot.Display.Rotation != 90 || !slot.Display.Mirror || !slot.Display.Flip || slot.Display.FitMode != "contain" {
		t.Fatalf("unexpected display transform: %+v", slot.Display)
	}
	if slot.Display.Crop != (DisplayCrop{X: 20, Y: 10, Width: 60, Height: 80}) {
		t.Fatalf("unexpected crop: %+v", slot.Display.Crop)
	}
}

func TestViewerLayoutPresetIDControlsLayoutCells(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "secret")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"viewer.layout.id":            ViewerLayoutVerticalPlusGrid,
		"viewer.layout.mode":          ViewerLayoutFocusMiddle,
		"viewer.layout.focus_slot_id": "cam2",
	}); err != nil {
		t.Fatal(err)
	}

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Layout.ID != ViewerLayoutVerticalPlusGrid || viewer.Layout.Mode != ViewerLayoutFocusMiddle {
		t.Fatalf("unexpected vertical layout: %+v", viewer.Layout)
	}
	if len(viewer.Layout.Cells) != 5 {
		t.Fatalf("expected focus plus four grid cells, got %+v", viewer.Layout.Cells)
	}
	if viewer.Layout.Cells[0].SlotID != "cam2" || viewer.Layout.Cells[0].Size != "portrait" {
		t.Fatalf("expected portrait focus cell, got %+v", viewer.Layout.Cells[0])
	}
}

func TestViewerReturnsSanitizedCustomLayout(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "secret")
	custom, err := json.Marshal(ViewerCustomLayout{
		Columns: []int{40, 20, 20, 20},
		Rows:    []int{70, 30},
		Cells: []ViewerCustomLayoutCell{
			{SlotID: "cam3", Column: 2, Row: 1, ColumnSpan: 2, RowSpan: 2},
			{SlotID: "missing", Column: 1, Row: 1, ColumnSpan: 1, RowSpan: 1},
			{SlotID: "cam3", Column: 1, Row: 1, ColumnSpan: 1, RowSpan: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"viewer.layout.id":         ViewerLayoutCustom,
		"viewer.layout.mode":       ViewerLayoutCustom,
		"viewer.layout.custom":     string(custom),
		"viewer.layout.slot_order": "cam3,cam1",
	}); err != nil {
		t.Fatal(err)
	}

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Layout.ID != ViewerLayoutCustom || viewer.Layout.Mode != ViewerLayoutCustom {
		t.Fatalf("unexpected custom layout: %+v", viewer.Layout)
	}
	if !slices.Equal(viewer.Layout.Custom.Columns, []int{40, 20, 20, 20}) || !slices.Equal(viewer.Layout.Custom.Rows, []int{70, 30}) {
		t.Fatalf("unexpected custom weights: %+v", viewer.Layout.Custom)
	}
	if len(viewer.Layout.Custom.Cells) != len(config.DefaultSlots()) {
		t.Fatalf("expected one custom cell per slot, got %+v", viewer.Layout.Custom.Cells)
	}
	first := viewer.Layout.Custom.Cells[0]
	if first.SlotID != "cam3" || first.Column != 2 || first.Row != 1 || first.ColumnSpan != 2 || first.RowSpan != 2 {
		t.Fatalf("expected sanitized lead cell, got %+v", first)
	}
	for _, cell := range viewer.Layout.Custom.Cells {
		if cell.SlotID == "missing" {
			t.Fatalf("custom layout kept missing slot: %+v", viewer.Layout.Custom.Cells)
		}
	}
}

func TestViewerIncludesPerformanceModeAndStreamMetrics(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cam1":{"producers":[{"url":"rtsp://redacted"}],"consumers":[{"remote_addr":"127.0.0.1"}]}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "secret")
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		"viewer.performance.mode": ViewerPerformanceLow,
	}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:redacted@192.168.1.20:554/stream2\n")

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Performance.Mode != ViewerPerformanceLow || viewer.Performance.Name == "" || len(viewer.Performance.Options) != 4 {
		t.Fatalf("unexpected performance settings: %+v", viewer.Performance)
	}
	slot := viewer.Slots[0]
	if slot.Stream == nil || slot.Stream.Alias != "cam1" || !slot.Stream.Configured || slot.Stream.Producers != 1 || slot.Stream.Consumers != 1 {
		t.Fatalf("unexpected stream metrics: %+v", slot.Stream)
	}
	if !slices.ContainsFunc(slot.Diagnostics, func(diag ViewerDiagnostic) bool {
		return diag.Key == "stream" && diag.Status == "ok" && strings.Contains(diag.Message, "Consumer: 1")
	}) {
		t.Fatalf("expected stream diagnostic, got %+v", slot.Diagnostics)
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
		RequestTimeout: 100 * time.Millisecond,
	}
	a := &App{
		Config: cfg,
		Store:  store,
		Slots:  slots,
		RTSPProbe: func(context.Context, string, string) error {
			return nil
		},
	}
	a.SetCameraCredentials(password, "test")
	return a
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

func TestEffectiveBindAddrEnablesLANOnExistingPort(t *testing.T) {
	got, err := effectiveBindAddr("127.0.0.1:8091", "true")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.0.0.0:8091" {
		t.Fatalf("expected LAN bind address, got %q", got)
	}
}

func TestEffectiveBindAddrDisablesLANOnExistingPort(t *testing.T) {
	got, err := effectiveBindAddr("0.0.0.0:8091", "false")
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:8091" {
		t.Fatalf("expected loopback bind address, got %q", got)
	}
}

func TestEffectiveBindAddrPreservesConfiguredAddressWhenUnset(t *testing.T) {
	got, err := effectiveBindAddr("0.0.0.0:8091", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.0.0.0:8091" {
		t.Fatalf("expected configured bind address, got %q", got)
	}
}

func TestApplyNetworkAccessKeepsLANClosedWithoutAdminPassword(t *testing.T) {
	a := newViewerTestApp(t, "http://127.0.0.1:1984", "")
	a.Config.BindAddr = "0.0.0.0:8091"
	if err := a.Store.PutSettings(context.Background(), map[string]string{NetworkSettingLANAccess: "true"}); err != nil {
		t.Fatal(err)
	}
	if err := a.applyNetworkAccess(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Config.BindAddr != "127.0.0.1:8091" {
		t.Fatalf("expected safe loopback bind without admin password, got %q", a.Config.BindAddr)
	}
}

func TestEffectiveBindAddrRejectsNonCanonicalBooleans(t *testing.T) {
	for _, value := range []string{"1", "yes", "on", "TRUE", " true ", "false "} {
		if _, err := effectiveBindAddr("127.0.0.1:8091", value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}
