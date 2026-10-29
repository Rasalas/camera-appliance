package relay

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestConfigurationApplyChangesActualSSHArgumentsAndStopsLastForward(t *testing.T) {
	a := newTestManager(t)
	seedRelayCamera(t, a)
	ctx := context.Background()
	a.startGrace = 30 * time.Millisecond
	a.Probe = func(context.Context, string, string) error { return errors.New("closed") }
	var commands [][]string
	a.newCommand = func(args []string) (*exec.Cmd, error) {
		commands = append(commands, append([]string(nil), args...))
		return exec.Command("bash", "-c", "exec -a ssh sleep 60"), nil
	}
	t.Cleanup(func() {
		a.childMu.Lock()
		defer a.childMu.Unlock()
		for _, child := range a.children {
			_ = syscall.Kill(child.pid, syscall.SIGKILL)
		}
	})
	if _, err := a.Start(ctx, "nas"); err != nil {
		t.Fatal(err)
	}
	apply := func(values map[string]string) {
		t.Helper()
		if err := a.Configure(ctx, func(u *ConfigurationUpdate) error {
			if err := a.store.PutSettings(ctx, values); err != nil {
				return err
			}
			_, err := u.Apply(ctx, u.Before)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply(map[string]string{"camera.relay_endpoint.dev1.nas.target_host": "192.0.2.99", "camera.relay_endpoint.dev1.nas.target_port": "8554"})
	if len(commands) != 2 || !strings.Contains(strings.Join(commands[1], " "), "-L 127.0.0.1:15541:192.0.2.99:8554") {
		t.Fatalf("stale SSH arguments: %v", commands)
	}
	apply(map[string]string{"camera.path_policy.dev1": "direct_only"})
	statuses, err := a.Statuses(ctx)
	if err != nil || len(statuses) != 1 || statuses[0].PID != 0 || len(commands) != 2 {
		t.Fatalf("last endpoint: %+v %v commands %v", statuses, err, commands)
	}
	if _, err := a.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatal("watchdog restarted empty relay")
	}
	apply(map[string]string{"camera.path_policy.dev1": "auto"})
	if len(commands) != 3 {
		t.Fatal("restoring policy did not restart forward")
	}
	if err := a.Configure(ctx, func(u *ConfigurationUpdate) error {
		visible := false
		if _, err := a.store.ConfigureCamera(ctx, "dev1", nil, nil, &visible); err != nil {
			return err
		}
		_, err := u.Apply(ctx, u.Before)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	statuses, err = a.Statuses(ctx)
	if err != nil || statuses[0].PID != 0 || len(commands) != 3 {
		t.Fatalf("hide retained forward: %+v %v", statuses, err)
	}
}

func TestConfigurationApplyPreservesManualDisabledAndExternalRelays(t *testing.T) {
	for _, kind := range []string{"manual", "disabled", "external"} {
		t.Run(kind, func(t *testing.T) {
			a := newTestManager(t)
			seedRelayCamera(t, a)
			ctx := context.Background()
			a.Probe = func(context.Context, string, string) error {
				if kind == "external" {
					return nil
				}
				return errors.New("closed")
			}
			if kind == "manual" {
				_ = a.store.PutSettings(ctx, map[string]string{"camera.relay.nas.auto_start": "false"})
			}
			if kind == "disabled" {
				_ = a.store.PutSettings(ctx, map[string]string{"camera.relay.nas.enabled": "false"})
			}
			a.StartProcess = func(context.Context, ManagedRelay) (int, error) { t.Fatal("unexpected start"); return 0, nil }
			a.StopProcess = func(context.Context, Status) error { t.Fatal("unexpected stop"); return nil }
			err := a.Configure(ctx, func(u *ConfigurationUpdate) error {
				if err := a.store.PutSettings(ctx, map[string]string{"camera.relay_endpoint.dev1.nas.target_host": "192.0.2.99"}); err != nil {
					return err
				}
				warnings, err := u.Apply(ctx, u.Before)
				if kind != "disabled" && len(warnings) == 0 {
					t.Fatal("missing manual apply warning")
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
