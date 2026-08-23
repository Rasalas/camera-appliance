package system

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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

func TestDetachedComposeArgsRunsFromExternalHelper(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "release.env"), []byte("VERSION=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ComposeFile: filepath.Join(dir, "compose.yaml")}
	got := detachedComposeArgs(cfg, "sha256:test", "updater-1", "up", "-d", "--force-recreate")
	joined := strings.Join(got, " ")
	for _, want := range []string{"run --rm -d", "--entrypoint docker-compose", "/var/run/docker.sock:/var/run/docker.sock", dir + ":" + dir, "sha256:test", "--env-file " + filepath.Join(dir, "release.env"), "-f " + filepath.Join(dir, "compose.yaml"), "up -d --force-recreate"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected helper args to contain %q, got %q", want, joined)
		}
	}
}

func TestDetachedComposeArgsOmitsMissingReleaseEnv(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{ComposeFile: filepath.Join(dir, "compose.yaml")}
	joined := strings.Join(detachedComposeArgs(cfg, "sha256:test", "updater-1", "up", "-d"), " ")
	if strings.Contains(joined, "--env-file") {
		t.Fatalf("expected missing release.env to be omitted, got %q", joined)
	}
}

func TestApplyStackModeFailsClosedInsideContainerWithoutImage(t *testing.T) {
	if _, err := applyStackMode("", false, true); err == nil {
		t.Fatal("expected containerized apply without image discovery to fail closed")
	}
	if detached, err := applyStackMode("sha256:test", true, true); err != nil || !detached {
		t.Fatalf("expected discovered image to use detached helper, detached=%t err=%v", detached, err)
	}
	if detached, err := applyStackMode("", false, false); err != nil || detached {
		t.Fatalf("expected native process to use synchronous compose, detached=%t err=%v", detached, err)
	}
}

func TestApplyStackModeIgnoresErrorOutputWhenDiscoveryFailed(t *testing.T) {
	if _, err := applyStackMode("docker inspect error text", false, true); err == nil {
		t.Fatal("expected failed image discovery with nonempty error output to fail closed")
	}
}

func TestSystemctlArgsForUserUnits(t *testing.T) {
	got := systemctlArgs(true, "--no-block", "restart", "camera-appliance")
	want := []string{"--user", "--no-block", "restart", "camera-appliance"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("systemctlArgs() = %v, want %v", got, want)
	}

	got = systemctlArgs(false, "is-active", "camera-appliance")
	want = []string{"is-active", "camera-appliance"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("systemctlArgs() = %v, want %v", got, want)
	}
}
