package app

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestSSHRelayArgsBuildsLocalForwards(t *testing.T) {
	relay := ManagedRelay{
		RelayDefinition: RelayDefinition{
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
	a := newWatchdogTestApp(t)
	a.RTSPProbe = func(context.Context, string, string) error {
		return errors.New("closed")
	}
	starts := 0
	a.RelayStart = func(_ context.Context, relay ManagedRelay) (int, error) {
		starts++
		if relay.ID != "nas" || len(relay.Endpoints) != 1 {
			t.Fatalf("unexpected relay passed to starter: %+v", relay)
		}
		return os.Getpid(), nil
	}
	seedRelayCamera(t, a)

	statuses, err := a.EnsureManagedRelays(ctx)
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
	a := newWatchdogTestApp(t)
	a.RTSPProbe = func(context.Context, string, string) error {
		return errors.New("closed")
	}
	starts := 0
	a.RelayStart = func(context.Context, ManagedRelay) (int, error) {
		starts++
		return 0, errors.New("ssh failed")
	}
	seedRelayCamera(t, a)

	if _, err := a.EnsureManagedRelays(ctx); err == nil {
		t.Fatal("expected start error")
	}
	if _, err := a.EnsureManagedRelays(ctx); err != nil {
		t.Fatalf("expected second run to stay in backoff without new error, got %v", err)
	}
	if starts != 1 {
		t.Fatalf("expected one start attempt during backoff, got %d", starts)
	}
	settings, err := a.Store.Settings(ctx)
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
	a := newWatchdogTestApp(t)
	a.RTSPProbe = nil
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.relay.ids":                    "nas",
		"camera.relay.nas.name":               "NAS Relay",
		"camera.relay.nas.type":               RelayTypeSSHLocalForward,
		"camera.relay.nas.host":               "host.docker.internal",
		"camera.relay_endpoint.dev1.nas.port": port,
	}); err != nil {
		t.Fatal(err)
	}

	statuses, err := a.RelayStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].ProcessState != "external" {
		t.Fatalf("expected external relay status, got %+v", statuses)
	}
}

func seedRelayCamera(t *testing.T, a *App) {
	t.Helper()
	seedWatchdogCamera(t, a, "dev1", "192.168.1.20", "direct")
	if err := a.Store.PutSettings(context.Background(), map[string]string{
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
		RelayDefinition: RelayDefinition{Type: RelayTypeSSHLocalForward, BindHost: "127.0.0.1", SSHTarget: "nas"},
		Endpoints: []RelayEndpoint{
			{DeviceID: "dev1", LocalPort: strconv.Itoa(15541), BindHost: "127.0.0.1", TargetHost: "192.168.1.20", TargetPort: "554"},
			{DeviceID: "dev2", LocalPort: strconv.Itoa(15541), BindHost: "127.0.0.1", TargetHost: "192.168.1.21", TargetPort: "554"},
		},
	}
	if _, err := sshRelayArgs(relay); err == nil {
		t.Fatal("expected duplicate local port error")
	}
}
