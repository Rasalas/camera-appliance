package app

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"camera-appliance/camera-manager/internal/discovery"
)

func TestStartupDiscoveryRenderAndRestartSettings(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, render := range []bool{false, true} {
			for _, restart := range []bool{false, true} {
				name := strconv.FormatBool(enabled) + "/" + strconv.FormatBool(render) + "/" + strconv.FormatBool(restart)
				t.Run(name, func(t *testing.T) {
					a := newViewerTestApp(t, "http://127.0.0.1:1", "")
					scans, restarts := 0, 0
					a.Scan = func(context.Context, discovery.Options) ([]discovery.Result, []discovery.Subnet, error) {
						scans++
						return nil, nil, nil
					}
					a.Go2RTCRestart = func(context.Context) error { restarts++; return nil }
					if err := a.Store.PutSettings(context.Background(), map[string]string{"auto_discover": strconv.FormatBool(enabled), "render_after_discovery": strconv.FormatBool(render), "restart_after_render": strconv.FormatBool(restart)}); err != nil {
						t.Fatal(err)
					}
					if err := a.RunStartup(context.Background()); err != nil {
						t.Fatal(err)
					}
					if (scans == 1) != enabled {
						t.Fatalf("scans=%d enabled=%t", scans, enabled)
					}
					_, err := os.Stat(a.Config.Go2RTCConfigPath())
					if (err == nil) != (enabled && render) {
						t.Fatalf("rendered=%t", err == nil)
					}
					if (restarts == 1) != (enabled && render && restart) {
						t.Fatalf("restarts=%d", restarts)
					}
				})
			}
		}
	}
}

func TestFailedDiscoveryDoesNotRenderOrRestart(t *testing.T) {
	a := newViewerTestApp(t, "http://127.0.0.1:1", "")
	a.Scan = func(context.Context, discovery.Options) ([]discovery.Result, []discovery.Subnet, error) {
		return nil, nil, errors.New("scan failed")
	}
	a.Go2RTCRestart = func(context.Context) error { t.Error("restarted after failure"); return nil }
	if err := a.Store.PutSettings(context.Background(), map[string]string{"render_after_discovery": "true", "restart_after_render": "true"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Discover(context.Background()); err == nil {
		t.Fatal("expected scan failure")
	}
	if _, err := os.Stat(a.Config.Go2RTCConfigPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rendered after scan failure")
	}
}

func TestExplicitRestartRunsOnceWithAutomaticRestartEnabled(t *testing.T) {
	a := newViewerTestApp(t, "http://127.0.0.1:1", "")
	restarts := 0
	a.Go2RTCRestart = func(context.Context) error { restarts++; return nil }
	if err := a.Store.PutSettings(context.Background(), map[string]string{"restart_after_render": "true"}); err != nil {
		t.Fatal(err)
	}
	if err := a.RestartStreams(context.Background()); err != nil {
		t.Fatal(err)
	}
	if restarts != 1 {
		t.Fatalf("restarted %d times", restarts)
	}
}
