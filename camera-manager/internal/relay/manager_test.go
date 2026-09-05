package relay

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/streamrouting"
)

func TestSSHRelayArgsBuildsLocalForwards(t *testing.T) {
	relay := ManagedRelay{
		RelayDefinition: streamrouting.RelayDefinition{
			ID:        "nas",
			Name:      "NAS Relay",
			Type:      RelayTypeSSHLocalForward,
			BindHost:  "127.0.0.1",
			SSHTarget: "nas",
			Enabled:   true,
		},
		Endpoints: []RelayEndpoint{
			{DeviceID: "dev1", LocalPort: "15541", BindHost: "127.0.0.1", TargetHost: "192.168.1.20", TargetPort: "554"},
			{DeviceID: "dev2", LocalPort: "15542", BindHost: "127.0.0.1", TargetHost: "192.168.1.21", TargetPort: "8554"},
		},
	}

	args, err := sshRelayArgs(relay)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-N",
		"ExitOnForwardFailure=yes",
		"BatchMode=yes",
		"-L 127.0.0.1:15541:192.168.1.20:554",
		"-L 127.0.0.1:15542:192.168.1.21:8554",
		"nas",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected args to contain %q, got %s", want, joined)
		}
	}
}

func TestEnsureManagedRelaysStartsStoppedAutoRelay(t *testing.T) {
	ctx := context.Background()
	a := newTestManager(t)
	a.Probe = func(context.Context, string, string) error {
		return errors.New("closed")
	}
	starts := 0
	a.StartProcess = func(_ context.Context, relay ManagedRelay) (int, error) {
		starts++
		if relay.ID != "nas" || len(relay.Endpoints) != 1 {
			t.Fatalf("unexpected relay passed to starter: %+v", relay)
		}
		return os.Getpid(), nil
	}
	seedRelayCamera(t, a)

	statuses, err := a.Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("expected one relay start, got %d", starts)
	}
	if len(statuses) != 1 || statuses[0].ProcessState != "running" {
		t.Fatalf("expected running status, got %+v", statuses)
	}
}

func TestEnsureManagedRelaysBacksOffAfterStartError(t *testing.T) {
	ctx := context.Background()
	a := newTestManager(t)
	a.Probe = func(context.Context, string, string) error {
		return errors.New("closed")
	}
	starts := 0
	a.StartProcess = func(context.Context, ManagedRelay) (int, error) {
		starts++
		return 0, errors.New("ssh failed")
	}
	seedRelayCamera(t, a)

	if _, err := a.Ensure(ctx); err == nil {
		t.Fatal("expected start error")
	}
	if _, err := a.Ensure(ctx); err != nil {
		t.Fatalf("expected second run to stay in backoff without new error, got %v", err)
	}
	if starts != 1 {
		t.Fatalf("expected one start attempt during backoff, got %d", starts)
	}
	settings, err := a.store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings[relayRuntimeKey("nas", "last_error")] != "ssh failed" {
		t.Fatalf("expected runtime error setting, got %+v", settings)
	}
	if settings[relayRuntimeKey("nas", "backoff_until")] == "" {
		t.Fatalf("expected backoff setting, got %+v", settings)
	}
}

func TestRelayStatusDetectsExternalForward(t *testing.T) {
	ctx := context.Background()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestManager(t)
	a.Probe = nil
	seedCamera(t, a)
	if err := a.store.PutSettings(ctx, map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.type":               RelayTypeSSHLocalForward,
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": port,
	}); err != nil {
		t.Fatal(err)
	}

	statuses, err := a.Statuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].ProcessState != "external" {
		t.Fatalf("expected external relay status, got %+v", statuses)
	}
}

func seedRelayCamera(t *testing.T, a *Manager) {
	t.Helper()
	seedCamera(t, a)
	if err := a.store.PutSettings(context.Background(), map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.type":               RelayTypeSSHLocalForward,
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay.nas.ssh_target":         "nas",
		"camera.relay.nas.auto_start":         "true",
		"camera.relay_endpoint.dev1.nas.port": "15541",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSSHRelayArgsRejectsDuplicateLocalPorts(t *testing.T) {
	relay := ManagedRelay{
		RelayDefinition: streamrouting.RelayDefinition{Type: RelayTypeSSHLocalForward, BindHost: "127.0.0.1", SSHTarget: "nas"},
		Endpoints: []RelayEndpoint{
			{DeviceID: "dev1", LocalPort: strconv.Itoa(15541), BindHost: "127.0.0.1", TargetHost: "192.168.1.20", TargetPort: "554"},
			{DeviceID: "dev2", LocalPort: strconv.Itoa(15541), BindHost: "127.0.0.1", TargetHost: "192.168.1.21", TargetPort: "554"},
		},
	}
	if _, err := sshRelayArgs(relay); err == nil {
		t.Fatal("expected duplicate local port error")
	}
}

func TestStreamPathCandidatesAutoAssignRelayPortFromSlot(t *testing.T) {
	settings := map[string]string{
		"camera.relay.ids":      "nas",
		"camera.relay.nas.host": "host.docker.internal",
	}
	binding := state.Binding{
		DeviceID: "dev1",
		SlotID:   "cam2",
		Device:   &state.Device{ID: "dev1", LastIP: "192.168.1.20"},
	}

	paths := streamrouting.Candidates(binding, settings)
	if len(paths) != 2 {
		t.Fatalf("expected direct + auto relay path, got %+v", paths)
	}
	relayPath := paths[1]
	if relayPath.Kind != streamrouting.PathKindRelay || relayPath.Host != "host.docker.internal" {
		t.Fatalf("unexpected relay path: %+v", relayPath)
	}
	if want := strconv.Itoa(streamrouting.RelayPortBaseDefault + 1); relayPath.Port != want {
		t.Fatalf("expected auto port %s for cam2, got %s", want, relayPath.Port)
	}

	settings["camera.relay_endpoint.dev1.nas.port"] = "15541"
	paths = streamrouting.Candidates(binding, settings)
	if len(paths) != 2 || paths[1].Port != "15541" {
		t.Fatalf("expected explicit port to win over auto port, got %+v", paths)
	}

	noSlot := state.Binding{DeviceID: "dev1", Device: binding.Device}
	delete(settings, "camera.relay_endpoint.dev1.nas.port")
	if paths = streamrouting.Candidates(noSlot, settings); len(paths) != 1 {
		t.Fatalf("expected no auto relay path without slot, got %+v", paths)
	}
}

func TestRelayEndpointsSkipDirectOnlyCameras(t *testing.T) {
	settings := map[string]string{
		"camera.relay.ids":        "nas",
		"camera.relay.nas.host":   "host.docker.internal",
		"camera.path_policy.dev1": streamrouting.PathPolicyDirectOnly,
	}
	bindings := []state.Binding{
		{DeviceID: "dev1", SlotID: "cam1", Device: &state.Device{ID: "dev1", LastIP: "192.168.1.20"}},
		{DeviceID: "dev2", SlotID: "cam2", Device: &state.Device{ID: "dev2", LastIP: "192.168.1.21"}},
	}

	relays := managedRelaysFromSettings(settings, bindings)
	if len(relays) != 1 {
		t.Fatalf("expected one relay, got %+v", relays)
	}
	endpoints := relays[0].Endpoints
	if len(endpoints) != 1 || endpoints[0].DeviceID != "dev2" {
		t.Fatalf("expected only dev2 endpoint, got %+v", endpoints)
	}
	if want := strconv.Itoa(streamrouting.RelayPortBaseDefault + 1); endpoints[0].LocalPort != want {
		t.Fatalf("expected auto port %s, got %+v", want, endpoints[0])
	}
}

func TestSSHRelayArgsSkipsEndpointsWithoutTargetHost(t *testing.T) {
	relay := ManagedRelay{
		RelayDefinition: streamrouting.RelayDefinition{Type: RelayTypeSSHLocalForward, BindHost: "127.0.0.1", SSHTarget: "nas"},
		Endpoints: []RelayEndpoint{
			{DeviceID: "dev1", LocalPort: "15541", BindHost: "127.0.0.1", TargetHost: "192.168.1.20", TargetPort: "554"},
			{DeviceID: "dev2", LocalPort: "15542", BindHost: "127.0.0.1", TargetPort: "554"},
		},
	}

	args, err := sshRelayArgs(relay)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-L 127.0.0.1:15541:192.168.1.20:554") {
		t.Fatalf("expected configured forward, got %s", joined)
	}
	if strings.Contains(joined, "15542") {
		t.Fatalf("unexpected forward for endpoint without target host: %s", joined)
	}
}

func TestSSHRelayArgsSkipsEndpointsWithoutLocalPort(t *testing.T) {
	relay := ManagedRelay{
		RelayDefinition: streamrouting.RelayDefinition{Type: RelayTypeSSHLocalForward, BindHost: "127.0.0.1", SSHTarget: "nas"},
		Endpoints: []RelayEndpoint{
			{DeviceID: "dev1", LocalPort: "15541", BindHost: "127.0.0.1", TargetHost: "192.168.1.20", TargetPort: "554"},
			{DeviceID: "dev2", BindHost: "127.0.0.1", TargetHost: "192.168.1.21", TargetPort: "554"},
		},
	}

	args, err := sshRelayArgs(relay)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-L 127.0.0.1:15541:192.168.1.20:554") {
		t.Fatalf("expected configured forward, got %s", joined)
	}
	if strings.Contains(joined, "192.168.1.21") {
		t.Fatalf("unexpected forward for endpoint without local port: %s", joined)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	ctx := context.Background()
	cfg := config.Config{StateDir: t.TempDir(), RequestTimeout: 50 * time.Millisecond}
	store, err := state.Open(ctx, cfg.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	slots := config.DefaultSlots()
	if err := store.UpsertSlots(ctx, slots); err != nil {
		t.Fatal(err)
	}
	return New(cfg, store, slots, nil)
}
func seedCamera(t *testing.T, a *Manager) {
	t.Helper()
	ctx := context.Background()
	if err := a.store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
}
