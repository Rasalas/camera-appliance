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

func TestUpdateWorkerSharesStateOutsideComposeProject(t *testing.T) {
	cfg := config.Config{InstallDir: "/opt/appliance", ConfigDir: "/etc/appliance", StateDir: "/var/lib/appliance", ComposeFile: "/opt/appliance/compose.yaml"}
	args := strings.Join(updateWorkerArgs(cfg, "sha256:old", "/var/lib/appliance/updates/worker-1", "/var/lib/appliance/updates/job.json", "1"), " ")
	for _, want := range []string{"run --rm -d", "--network host", "--entrypoint /var/lib/appliance/updates/worker-1", "/etc/appliance:/etc/appliance", "/var/lib/appliance:/var/lib/appliance", "sha256:old update-worker --job /var/lib/appliance/updates/job.json"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "--label") {
		t.Fatalf("worker must not be a compose orphan: %s", args)
	}
}

func TestSystemdWorkerAndBlockingRestart(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	t.Setenv("CAMERA_TEST_COMMAND_LOG", log)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"systemd-run", "systemctl"} {
		script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CAMERA_TEST_COMMAND_LOG\"\nif [ \"$*\" = '--user restart camera-appliance' ]; then exit 7; fi\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{RestartStrategy: "systemd"}
	if err := StartUpdateWorker(context.Background(), cfg, "/state/worker", "/state/job.json", "123"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyStackAndWait(context.Background(), cfg); err == nil {
		t.Fatal("manager restart failure ignored")
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	commands := string(data)
	for _, want := range []string{"--user --collect --unit=camera-appliance-update-123 /state/worker update-worker --job /state/job.json", "--user restart camera-appliance-go2rtc", "--user restart camera-appliance"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing %q: %s", want, commands)
		}
	}
	if strings.Contains(commands, "--no-block") {
		t.Fatalf("updater did not wait: %s", commands)
	}
}

func TestComposeUsesReleaseBuildMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "release.env"), []byte("VERSION=2.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(composeArgs(config.Config{ComposeFile: filepath.Join(dir, "compose.yaml")}, "up", "-d"), " ")
	if !strings.Contains(args, "--env-file "+filepath.Join(dir, "release.env")) {
		t.Fatalf("release version would be lost: %s", args)
	}
}
