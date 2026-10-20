package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/streamrouting"
)

func TestWatchdogRestartsGo2RTCOnlyAfterConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	a := newGo2RTCSupervisionTestApp(t, newGo2RTCHealthServer(t, false))
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}

	for i := 1; i < go2rtcHealthFailureThreshold; i++ {
		if _, err := a.RunWatchdogOnce(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if restarts != 0 {
		t.Fatalf("expected no restart before %d failures, got %d", go2rtcHealthFailureThreshold, restarts)
	}
	result, err := a.RunWatchdogOnce(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if restarts != 1 || result.Action != "go2rtc neu gestartet" {
		t.Fatalf("expected restart at threshold, got %d restarts, action %q", restarts, result.Action)
	}
	assertWatchdogEvent(t, a, "watchdog.go2rtc_restarted")

	// Still unhealthy: the backoff holds the next restart.
	if _, err := a.RunWatchdogOnce(ctx, false); err != nil {
		t.Fatal(err)
	}
	if restarts != 1 {
		t.Fatalf("expected backoff to hold restart, got %d restarts", restarts)
	}

	expireSetting(t, a, watchdogGo2RTCNextRestartAtKey)
	before := time.Now().UTC()
	if _, err := a.RunWatchdogOnce(ctx, false); err != nil {
		t.Fatal(err)
	}
	if restarts != 2 {
		t.Fatalf("expected restart after backoff, got %d restarts", restarts)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nextAt, ok := parseSettingTime(settings[watchdogGo2RTCNextRestartAtKey])
	if !ok || nextAt.Before(before.Add(2*go2rtcRestartBackoffBase-time.Second)) {
		t.Fatalf("expected doubled backoff after second restart, got %q", settings[watchdogGo2RTCNextRestartAtKey])
	}
}

func TestWatchdogGo2RTCRecoveryResetsFailureCount(t *testing.T) {
	ctx := context.Background()
	var healthy atomic.Bool
	a := newGo2RTCSupervisionTestApp(t, newToggleHealthServer(t, &healthy))
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}

	for _, online := range []bool{false, false, true, false, false} {
		healthy.Store(online)
		if _, err := a.RunWatchdogOnce(ctx, false); err != nil {
			t.Fatal(err)
		}
	}
	if restarts != 0 {
		t.Fatalf("expected non-consecutive failures not to restart, got %d", restarts)
	}
}

func TestWatchdogContinuesPathChecksWhenGo2RTCRestartFails(t *testing.T) {
	ctx := context.Background()
	a := newGo2RTCSupervisionTestApp(t, newGo2RTCHealthServer(t, false))
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return errors.New("docker unavailable")
	}
	a.RTSPProbe = failDirectProbe
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	seedRelay(t, a, map[string]string{
		streamrouting.PathFailThresholdKey: "1",
		watchdogGo2RTCHealthFailuresKey:    "2",
	})

	result, err := a.RunWatchdogOnce(ctx, true)
	if err == nil {
		t.Fatal("expected restart error")
	}
	if len(result.PathChanges) != 1 || result.PathChanges[0].To != "relay:nas" {
		t.Fatalf("expected path check despite failed go2rtc restart, got %+v", result.PathChanges)
	}
	if restarts != 2 {
		t.Fatalf("expected health and path restart attempts, got %d", restarts)
	}
	assertWatchdogEvent(t, a, "watchdog.go2rtc_restart_failed")
}

func TestWatchdogRetriesFailedPathRestart(t *testing.T) {
	ctx := context.Background()
	a := newWatchdogTestApp(t)
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		if restarts == 1 {
			return errors.New("docker unavailable")
		}
		return nil
	}
	a.RTSPProbe = failDirectProbe
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	seedRelay(t, a, map[string]string{streamrouting.PathFailThresholdKey: "1"})

	result, err := a.RunWatchdogOnce(ctx, true)
	if err == nil || len(result.PathChanges) != 1 {
		t.Fatalf("expected failed path restart, got %+v, %v", result, err)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[watchdogPathRestartPendingKey] != "true" {
		t.Fatalf("expected pending restart after failure, got %+v", settings)
	}
	if settings[streamrouting.ActivePathKeyPrefix+"dev1.id"] != "relay:nas" {
		t.Fatalf("expected active relay path, got %q", settings[streamrouting.ActivePathKeyPrefix+"dev1.id"])
	}

	if _, err := a.RunWatchdogOnce(ctx, false); err != nil {
		t.Fatal(err)
	}
	if restarts != 1 {
		t.Fatalf("expected retry to wait for backoff, got %d restarts", restarts)
	}

	expireSetting(t, a, watchdogPathRestartRetryAtKey)
	result, err = a.RunWatchdogOnce(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if restarts != 2 || result.Action != "go2rtc nach Cooldown neu gestartet" {
		t.Fatalf("expected pending restart to be retried, got %d restarts, action %q", restarts, result.Action)
	}
	settings, err = a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[watchdogPathRestartPendingKey] != "false" || settings[watchdogPathRestartFailuresKey] != "0" {
		t.Fatalf("expected pending restart to be cleared, got %+v", settings)
	}
	assertWatchdogEvent(t, a, "watchdog.path_restart_after_cooldown")
}

func TestGo2RTCRestartsAreSerialized(t *testing.T) {
	ctx := context.Background()
	a := newGo2RTCSupervisionTestApp(t, newGo2RTCHealthServer(t, false))
	if err := a.Store.PutSettings(ctx, map[string]string{
		"restart_after_render":          "true",
		watchdogGo2RTCHealthFailuresKey: "2",
	}); err != nil {
		t.Fatal(err)
	}
	var active, maxActive, calls atomic.Int32
	a.Go2RTCRestart = func(context.Context) error {
		current := active.Add(1)
		defer active.Add(-1)
		calls.Add(1)
		for {
			seen := maxActive.Load()
			if current <= seen || maxActive.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				t.Error(err)
			}
		}()
	}
	for range 3 {
		run(func() error { return a.RestartStreams(ctx) })
		run(func() error { _, err := a.RenderConfiguredGo2RTC(ctx); return err })
	}
	run(func() error { _, err := a.RunWatchdogOnce(ctx, false); return err })
	wg.Wait()

	if calls.Load() < 7 {
		t.Fatalf("expected all restarts to run, got %d", calls.Load())
	}
	if maxActive.Load() != 1 {
		t.Fatalf("expected serialized go2rtc restarts, saw %d concurrent", maxActive.Load())
	}
}

func TestRestartBackoffDoublesUpToCap(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, expected := range want {
		if got := restartBackoff(i + 1); got != expected {
			t.Fatalf("attempt %d: got %s, want %s", i+1, got, expected)
		}
	}
}

func newGo2RTCSupervisionTestApp(t *testing.T, go2rtcURL string) *App {
	t.Helper()
	a := newViewerTestApp(t, go2rtcURL, "secret")
	if err := a.Store.PutSettings(context.Background(), map[string]string{watchdogRestartGo2RTCOnFailureKey: "true"}); err != nil {
		t.Fatal(err)
	}
	return a
}

func newGo2RTCHealthServer(t *testing.T, healthy bool) string {
	t.Helper()
	var state atomic.Bool
	state.Store(healthy)
	return newToggleHealthServer(t, &state)
}

func newToggleHealthServer(t *testing.T, healthy *atomic.Bool) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func failDirectProbe(_ context.Context, host, port string) error {
	if host == "192.168.1.20" && port == "554" {
		return errors.New("direct unavailable")
	}
	return nil
}

func seedRelay(t *testing.T, a *App, extra map[string]string) {
	t.Helper()
	values := map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}
	for key, value := range extra {
		values[key] = value
	}
	if err := a.Store.PutSettings(context.Background(), values); err != nil {
		t.Fatal(err)
	}
}

func expireSetting(t *testing.T, a *App, key string) {
	t.Helper()
	if err := a.Store.PutSettings(context.Background(), map[string]string{key: time.Now().UTC().Add(-time.Second).Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
}
