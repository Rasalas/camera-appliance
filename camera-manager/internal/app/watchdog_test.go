package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/state"
)

func TestWatchdogSwitchesFromDirectToRelay(t *testing.T) {
	ctx := context.Background()
	a := newWatchdogTestApp(t)
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "192.168.1.20" && port == "554" {
			return errors.New("direct unavailable")
		}
		return nil
	}
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 0 {
		t.Fatalf("expected first direct failure to be held, got %+v", result.PathChanges)
	}
	if restarts != 0 {
		t.Fatalf("expected no restart for first failure, got %d", restarts)
	}

	result, err = a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 1 || result.PathChanges[0].From != "direct" || result.PathChanges[0].To != "relay:nas" {
		t.Fatalf("expected direct to relay change, got %+v", result.PathChanges)
	}
	if restarts != 1 {
		t.Fatalf("expected one restart, got %d", restarts)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[activePathKeyPrefix+"dev1.id"] != "relay:nas" {
		t.Fatalf("expected active relay path, got %+v", settings)
	}
	assertWatchdogEvent(t, a, "watchdog.path_switched")
}

func TestWatchdogSwitchesFromRelayToDirectWhenRelayFails(t *testing.T) {
	ctx := context.Background()
	a := newWatchdogTestApp(t)
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "127.0.0.1" && port == "15541" {
			return errors.New("relay unavailable")
		}
		return nil
	}
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "relay:nas")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 0 {
		t.Fatalf("expected first relay failure to be held, got %+v", result.PathChanges)
	}
	if restarts != 0 {
		t.Fatalf("expected no restart for first failure, got %d", restarts)
	}

	result, err = a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 1 || result.PathChanges[0].From != "relay:nas" || result.PathChanges[0].To != "direct" {
		t.Fatalf("expected relay to direct change, got %+v", result.PathChanges)
	}
	if restarts != 1 {
		t.Fatalf("expected one restart, got %d", restarts)
	}
}

func TestWatchdogPreferDirectReturnsToDirect(t *testing.T) {
	ctx := context.Background()
	a := newWatchdogTestApp(t)
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "relay:nas")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.path_policy.dev1":             PathPolicyPreferDirect,
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 0 {
		t.Fatalf("expected first direct recovery to be held, got %+v", result.PathChanges)
	}
	if restarts != 0 {
		t.Fatalf("expected no restart before recovery threshold, got %d", restarts)
	}

	result, err = a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 1 || result.PathChanges[0].To != "direct" {
		t.Fatalf("expected prefer_direct to choose direct, got %+v", result.PathChanges)
	}
	if restarts != 1 {
		t.Fatalf("expected one restart, got %d", restarts)
	}
}

func TestWatchdogPathRestartCooldownDefersRepeatedRestarts(t *testing.T) {
	ctx := context.Background()
	a := newWatchdogTestApp(t)
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}
	a.RTSPProbe = func(_ context.Context, host, port string) error {
		if host == "192.168.1.20" && port == "554" {
			return errors.New("direct unavailable")
		}
		return nil
	}
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	if err := a.Store.PutSettings(ctx, map[string]string{
		pathFailThresholdKey:                  "1",
		pathRestartCooldownSecondsKey:         "120",
		watchdogPathRestartLastAtKey:          time.Now().UTC().Format(time.RFC3339),
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 1 {
		t.Fatalf("expected path change, got %+v", result.PathChanges)
	}
	if restarts != 0 {
		t.Fatalf("expected restart to be deferred by cooldown, got %d", restarts)
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[watchdogPathRestartPendingKey] != "true" {
		t.Fatalf("expected pending restart, got %+v", settings)
	}
	assertWatchdogEvent(t, a, "watchdog.path_restart_cooldown")

	if err := a.Store.PutSettings(ctx, map[string]string{
		watchdogPathRestartLastAtKey: time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.RunWatchdogOnce(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if restarts != 1 {
		t.Fatalf("expected pending restart after cooldown, got %d", restarts)
	}
	assertWatchdogEvent(t, a, "watchdog.path_restart_after_cooldown")
}

func TestWatchdogNoPathChangeDoesNotRestartOrRender(t *testing.T) {
	ctx := context.Background()
	a := newWatchdogTestApp(t)
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error {
		restarts++
		return nil
	}
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")

	result, err := a.RunWatchdogOnce(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PathChanges) != 0 {
		t.Fatalf("expected no path changes, got %+v", result.PathChanges)
	}
	if restarts != 0 {
		t.Fatalf("expected no restart, got %d", restarts)
	}
	events, err := a.Store.Events(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "go2rtc.rendered" || event.Type == "watchdog.path_switched" {
			t.Fatalf("expected no render/switch event, got %+v", event)
		}
	}
}

func newWatchdogTestApp(t *testing.T) *App {
	t.Helper()
	a := newViewerTestApp(t, "http://127.0.0.1:1", "secret")
	if err := a.Store.PutSettings(context.Background(), map[string]string{
		watchdogRestartGo2RTCOnFailureKey: "false",
	}); err != nil {
		t.Fatal(err)
	}
	return a
}

func seedWatchdogCamera(t *testing.T, a *App, deviceID, ip, activePath string) {
	t.Helper()
	ctx := context.Background()
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: deviceID, LastIP: ip}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: deviceID, Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PutSettings(ctx, map[string]string{
		activePathKeyPrefix + deviceID + ".id": activePath,
	}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:secret@192.168.1.20:554/stream2\n")
}

func assertWatchdogEvent(t *testing.T, a *App, eventType string) {
	t.Helper()
	events, err := a.Store.Events(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == eventType {
			return
		}
	}
	var types []string
	for _, event := range events {
		types = append(types, event.Type)
	}
	t.Fatalf("expected event %s, got %s", eventType, strings.Join(types, ", "))
}
