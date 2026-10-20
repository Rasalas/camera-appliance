package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

func TestViewerUsesWatchdogPathStateWithoutDialing(t *testing.T) {
	ctx := context.Background()
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer go2rtc.Close()
	a := newWatchdogTestApp(t)
	a.RTSPProbe = func(_ context.Context, host, _ string) error {
		if host == "192.168.1.20" {
			return errors.New("connection refused")
		}
		return nil
	}
	a.Go2RTCRestart = func(context.Context) error { return nil }
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := a.RunWatchdogOnce(ctx, true); err != nil {
			t.Fatal(err)
		}
	}
	a.Config.Go2RTCURL = go2rtc.URL
	var dials atomic.Int32
	a.RTSPProbe = func(context.Context, string, string) error {
		dials.Add(1)
		return nil
	}
	before, _ := a.Store.Settings(ctx)

	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dials.Load() != 0 {
		t.Fatalf("viewer dialed %d times despite fresh watchdog state", dials.Load())
	}
	slot := viewer.Slots[0]
	if slot.Path == nil || slot.Path.ID != "relay:nas" || slot.State != ViewerStateOnline {
		t.Fatalf("expected watchdog-selected relay path, got %s %+v", slot.State, slot.Path)
	}
	for _, path := range slot.Paths {
		if path.ID == "direct" && path.State != "failed" {
			t.Fatalf("expected persisted direct failure, got %+v", path)
		}
	}
	after, _ := a.Store.Settings(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("viewer read changed persisted watchdog state")
	}
}

func TestViewerProbesConcurrentlyAndCachesWithoutWatchdogState(t *testing.T) {
	ctx := context.Background()
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer go2rtc.Close()
	a := newViewerTestApp(t, go2rtc.URL, "secret")
	const probeDelay = 150 * time.Millisecond
	var dials atomic.Int32
	a.RTSPProbe = func(context.Context, string, string) error {
		dials.Add(1)
		time.Sleep(probeDelay)
		return errors.New("i/o timeout")
	}
	seedViewerCameras(t, a, 3)

	start := time.Now()
	viewer, err := a.Viewer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= 2*probeDelay {
		t.Fatalf("probes ran sequentially: %s for %d dials", elapsed, dials.Load())
	}
	if dials.Load() != 3 {
		t.Fatalf("expected one dial per camera, got %d", dials.Load())
	}
	for _, slot := range viewer.Slots[:3] {
		if slot.State != ViewerStateOffline {
			t.Fatalf("expected offline slot %s, got %s", slot.Alias, slot.State)
		}
	}
	if _, err := a.Viewer(ctx); err != nil {
		t.Fatal(err)
	}
	if dials.Load() != 3 {
		t.Fatalf("expected cached probe results, got %d dials", dials.Load())
	}
	settings, _ := a.Store.Settings(ctx)
	for key := range settings {
		if strings.HasPrefix(key, "camera.path_state.") {
			t.Fatalf("viewer probe advanced watchdog counters: %s", key)
		}
	}
}

func TestViewerIgnoresWatchdogStateWhenWatchdogIsDisabled(t *testing.T) {
	ctx := context.Background()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "secret")
	seedViewerCameras(t, a, 1)
	if _, err := a.RunWatchdogOnce(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{watchdogEnabledKey: "false"}); err != nil {
		t.Fatal(err)
	}
	var dials atomic.Int32
	a.RTSPProbe = func(context.Context, string, string) error {
		dials.Add(1)
		return nil
	}
	if _, err := a.Viewer(ctx); err != nil {
		t.Fatal(err)
	}
	if dials.Load() != 1 {
		t.Fatalf("expected live probe while watchdog is disabled, got %d dials", dials.Load())
	}
}

func TestViewerKeepsPlaybackWhileGo2RTCIsUnavailable(t *testing.T) {
	ctx := context.Background()
	var down atomic.Bool
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer go2rtc.Close()
	a := newViewerTestApp(t, go2rtc.URL, "secret")
	seedViewerCameras(t, a, 1)
	slotState := func() ViewerSlot {
		t.Helper()
		viewer, err := a.Viewer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return viewer.Slots[0]
	}
	if slot := slotState(); slot.State != ViewerStateOnline || slot.Playback == nil {
		t.Fatalf("expected online slot with playback, got %s %+v", slot.State, slot.Playback)
	}
	down.Store(true)
	for i := 1; i < viewerGo2RTCFailureLimit; i++ {
		slot := slotState()
		if slot.State != ViewerStateOnline || slot.Playback == nil {
			t.Fatalf("failure %d changed the slot: %s %+v", i, slot.State, slot.Playback)
		}
		if !hasDiagnostic(slot, "go2rtc", "warn") {
			t.Fatalf("expected go2rtc warning, got %+v", slot.Diagnostics)
		}
	}
	for i := 0; i < 3; i++ {
		slot := slotState()
		if slot.State != ViewerStateStreamUnavailable || !hasDiagnostic(slot, "go2rtc", "failed") {
			t.Fatalf("expected unavailable status after %d failures, got %s %+v", viewerGo2RTCFailureLimit+i, slot.State, slot.Diagnostics)
		}
		if slot.Playback == nil || slot.Playback.PageURL == "" {
			t.Fatalf("go2rtc outage removed playback: %+v", slot.Playback)
		}
		if !strings.Contains(slot.Message, "weiter versucht") {
			t.Fatalf("expected retry message, got %q", slot.Message)
		}
	}
	down.Store(false)
	if slot := slotState(); slot.State != ViewerStateOnline || slot.Playback == nil {
		t.Fatalf("expected recovery, got %s %+v", slot.State, slot.Playback)
	}
}

func TestViewerKeepsPlaybackWhenGo2RTCWasNeverOnline(t *testing.T) {
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer go2rtc.Close()
	a := newViewerTestApp(t, go2rtc.URL, "secret")
	seedViewerCameras(t, a, 1)
	viewer, err := a.Viewer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	slot := viewer.Slots[0]
	if slot.State != ViewerStateStreamUnavailable || slot.Playback == nil {
		t.Fatalf("expected unavailable status with playback on cold start, got %s %+v", slot.State, slot.Playback)
	}
}

func TestGo2RTCAvailabilityReportsLongOutagesAndColdStarts(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var cold go2rtcAvailability
	if !cold.observe(false, now) {
		t.Fatal("go2rtc that was never online must be reported unavailable at once")
	}
	var g go2rtcAvailability
	g.observe(true, now)
	if g.observe(false, now) {
		t.Fatal("first failure must be tolerated")
	}
	if !g.observe(false, now.Add(viewerGo2RTCFailureGrace)) {
		t.Fatal("outage longer than the grace period must be reported")
	}
}

func TestViewerCachesCredentialPresenceUntilInvalidated(t *testing.T) {
	ctx := context.Background()
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer go2rtc.Close()
	a := newViewerTestApp(t, go2rtc.URL, "")
	var lookups atomic.Int32
	password := ""
	var mu sync.Mutex
	a.loadCameraSecret = func(_, _ string) secrets.Result {
		lookups.Add(1)
		mu.Lock()
		defer mu.Unlock()
		return secrets.Result{Value: password}
	}
	seedViewerCameras(t, a, 1)
	state := func() string {
		t.Helper()
		viewer, err := a.Viewer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return viewer.Slots[0].State
	}
	if got := state(); got != ViewerStateCredentialsFailed {
		t.Fatalf("expected missing credentials, got %s", got)
	}
	mu.Lock()
	password = "camera-secret"
	mu.Unlock()
	if got := state(); got != ViewerStateCredentialsFailed || lookups.Load() != 1 {
		t.Fatalf("expected cached absence, got %s after %d lookups", got, lookups.Load())
	}
	a.InvalidateCredentialPresence("dev1")
	if got := state(); got != ViewerStateOnline || lookups.Load() != 2 {
		t.Fatalf("expected fresh lookup after invalidation, got %s after %d lookups", got, lookups.Load())
	}
	a.credentialPresence.mu.Lock()
	entry := a.credentialPresence.entries["dev1"]
	entry.checkedAt = entry.checkedAt.Add(-credentialPresenceTTL)
	a.credentialPresence.entries["dev1"] = entry
	a.credentialPresence.mu.Unlock()
	state()
	if lookups.Load() != 3 {
		t.Fatalf("expected lookup after TTL, got %d", lookups.Load())
	}
}

func TestViewerAssemblySpawnsNoProcesses(t *testing.T) {
	ctx := context.Background()
	bin := t.TempDir()
	logPath := filepath.Join(bin, "calls.log")
	for _, name := range []string{"docker", "docker-compose", "systemctl", "secret-tool"} {
		script := fmt.Sprintf("#!/bin/sh\necho %s >> %q\n[ %s = secret-tool ] && printf camera-secret\nexit 0\n", name, logPath, name)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("TAPO_CAMERA_PASSWORD", "")
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer go2rtc.Close()
	a := newViewerTestApp(t, go2rtc.URL, "")
	seedViewerCameras(t, a, 2)
	calls := func() string {
		data, _ := os.ReadFile(logPath)
		return string(data)
	}
	// The first request may resolve credential presence once per camera.
	if _, err := a.Viewer(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calls(); strings.Contains(got, "docker") || strings.Contains(got, "systemctl") {
		t.Fatalf("viewer ran service commands: %q", got)
	}
	_ = os.Remove(logPath)
	for i := 0; i < 3; i++ {
		if _, err := a.Viewer(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls(); got != "" {
		t.Fatalf("repeated viewer requests spawned processes: %q", got)
	}
}

// BenchmarkViewerAssembly simulates five offline cameras whose RTSP dials each time out.
func BenchmarkViewerAssembly(b *testing.B) {
	const dialTimeout = 100 * time.Millisecond
	for _, tc := range []struct {
		name     string
		watchdog bool
		cold     bool
	}{
		{name: "watchdog_state"},
		{name: "no_state_cold_cache", cold: true},
		{name: "no_state_warm_cache"},
	} {
		tc.watchdog = tc.name == "watchdog_state"
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			defer go2rtc.Close()
			a := newViewerTestApp(b, go2rtc.URL, "secret")
			a.RTSPProbe = func(context.Context, string, string) error {
				time.Sleep(dialTimeout)
				return errors.New("i/o timeout")
			}
			seedViewerCameras(b, a, 5)
			if tc.watchdog {
				if _, err := a.RunWatchdogOnce(ctx, true); err != nil {
					b.Fatal(err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if tc.cold {
					a.viewerProbes.mu.Lock()
					a.viewerProbes.entries = nil
					a.viewerProbes.mu.Unlock()
				}
				if _, err := a.Viewer(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// seedViewerCameras binds n cameras, each with a single direct path.
func seedViewerCameras(t testing.TB, a *App, n int) {
	t.Helper()
	ctx := context.Background()
	generated := "streams:\n"
	for i := 1; i <= n; i++ {
		deviceID := fmt.Sprintf("dev%d", i)
		slotID := fmt.Sprintf("cam%d", i)
		if err := a.Store.UpsertDevice(ctx, state.Device{ID: deviceID, LastIP: fmt.Sprintf("192.168.1.%d", 20+i)}); err != nil {
			t.Fatal(err)
		}
		if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: slotID, DeviceID: deviceID, Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		generated += fmt.Sprintf("  %s:\n    - rtsp://user:redacted@192.168.1.%d:554/stream2\n", slotID, 20+i)
	}
	writeGeneratedGo2RTC(t, a.Config, generated)
}

func hasDiagnostic(slot ViewerSlot, key, status string) bool {
	for _, diagnostic := range slot.Diagnostics {
		if diagnostic.Key == key && diagnostic.Status == status {
			return true
		}
	}
	return false
}
