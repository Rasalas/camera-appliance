package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/config"
)

func TestRestartGo2RTCUsesConfiguredCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "restarted")
	script := filepath.Join(dir, "restart-go2rtc")
	contents := "#!/bin/sh\n: > \"" + marker + "\"\n"
	if err := os.WriteFile(script, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := RestartGo2RTC(context.Background(), config.Config{Go2RTCRestart: script}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected restart command to create marker: %v", err)
	}
}

func TestRestartGo2RTCReportsConfiguredCommandFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "restart-go2rtc")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho failed safely >&2\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	err := RestartGo2RTC(context.Background(), config.Config{Go2RTCRestart: script})
	if err == nil {
		t.Fatal("expected restart command failure")
	}
	if got := err.Error(); got == "" || !containsAll(got, "restart command failed", "failed safely") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
