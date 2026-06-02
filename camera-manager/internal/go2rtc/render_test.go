package go2rtc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

func TestRenderWritesRedactedConfig(t *testing.T) {
	dir := t.TempDir()
	result, err := Render(context.Background(), RenderInput{
		Slots: []config.Slot{{ID: "cam1", Label: "Kamera 1", DefaultStream: "stream2"}},
		Bindings: []state.Binding{{
			SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true,
			Device: &state.Device{LastIP: "192.168.1.20"},
		}},
		Password: "secret",
		Output:   filepath.Join(dir, "go2rtc.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "user:secret@192.168.1.20") {
		t.Fatalf("unrendered config: %s", data)
	}
	if strings.Contains(result.RedactedYAML, "secret") {
		t.Fatalf("redacted yaml leaked secret: %s", result.RedactedYAML)
	}
}

func TestRenderUsesEndpointOverride(t *testing.T) {
	dir := t.TempDir()
	result, err := Render(context.Background(), RenderInput{
		Slots: []config.Slot{{ID: "cam1", Label: "Kamera 1", DefaultStream: "stream2"}},
		Bindings: []state.Binding{{
			SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true,
			Device: &state.Device{LastIP: "192.168.1.20"},
		}},
		Password: "secret",
		Endpoints: map[string]StreamEndpoint{
			"dev1": {Host: "host.docker.internal", Port: "15541"},
		},
		Output: filepath.Join(dir, "go2rtc.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "user:secret@host.docker.internal:15541") {
		t.Fatalf("endpoint override was not rendered: %s", data)
	}
}
