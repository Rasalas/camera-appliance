package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/config"
)

// fakeDocker logs every invocation. Inspecting camera-manager reports the
// given container fields; "image inspect" fails when previousMissing is set.
func fakeDocker(t *testing.T, inspect string, previousMissing bool) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	t.Setenv("CAMERA_TEST_COMMAND_LOG", log)
	t.Setenv("CAMERA_TEST_INSPECT", inspect)
	missing := "0"
	if previousMissing {
		missing = "1"
	}
	t.Setenv("CAMERA_TEST_PREVIOUS_MISSING", missing)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$CAMERA_TEST_COMMAND_LOG"
for arg do ref="$arg"; done
case "$1 $2" in
  "inspect --type")
    if [ "$ref" = camera-manager ]; then echo "$CAMERA_TEST_INSPECT"; exit 0; fi
    echo "Error: No such object: $ref" >&2; exit 1 ;;
  "image inspect")
    if [ "$CAMERA_TEST_PREVIOUS_MISSING" = 1 ]; then echo "Error: No such image" >&2; exit 1; fi
    echo sha256:old; exit 0 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return log
}

func readCommands(t *testing.T, log string) string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

const runningManager = "sha256:old|camera-appliance-camera-manager|camera-manager|camera-appliance"

func TestPreserveManagerImageTagsRunningImage(t *testing.T) {
	log := fakeDocker(t, runningManager, false)
	ref, err := PreserveManagerImage(context.Background(), config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if ref != "camera-appliance-camera-manager" {
		t.Fatalf("ref = %q", ref)
	}
	if commands := readCommands(t, log); !strings.Contains(commands, "tag sha256:old camera-appliance-camera-manager:previous") {
		t.Fatalf("running image was not preserved: %s", commands)
	}
}

func TestPreserveManagerImageRejectsOtherService(t *testing.T) {
	log := fakeDocker(t, "sha256:other|alexxit/go2rtc:1.9.14|go2rtc|camera-appliance", false)
	if _, err := PreserveManagerImage(context.Background(), config.Config{}); err == nil {
		t.Fatal("non-manager container accepted")
	}
	if commands := readCommands(t, log); strings.Contains(commands, "tag ") {
		t.Fatalf("unexpected tag: %s", commands)
	}
}

func TestRestorePreviousStackRetagsWithoutBuilding(t *testing.T) {
	log := fakeDocker(t, runningManager, false)
	cfg := config.Config{ComposeFile: filepath.Join(t.TempDir(), "compose.yaml")}
	if err := RestorePreviousStackAndWait(context.Background(), cfg, "camera-appliance-camera-manager"); err != nil {
		t.Fatal(err)
	}
	commands := readCommands(t, log)
	tag := strings.Index(commands, "tag camera-appliance-camera-manager:previous camera-appliance-camera-manager")
	up := strings.Index(commands, "up -d --no-build --force-recreate --remove-orphans")
	if tag < 0 || up < 0 || up < tag {
		t.Fatalf("expected retag before no-build recreate: %s", commands)
	}
	if strings.Contains(commands, "--build ") || strings.Contains(commands, " build") {
		t.Fatalf("rollback must not build: %s", commands)
	}
}

func TestRestorePreviousStackReportsMissingImage(t *testing.T) {
	log := fakeDocker(t, runningManager, true)
	err := RestorePreviousStackAndWait(context.Background(), config.Config{}, "camera-appliance-camera-manager")
	if !errors.Is(err, ErrPreviousImageMissing) {
		t.Fatalf("err = %v", err)
	}
	if commands := readCommands(t, log); strings.Contains(commands, "up ") || strings.Contains(commands, "tag ") {
		t.Fatalf("missing image must not change the stack: %s", commands)
	}
	if err := RestorePreviousStackAndWait(context.Background(), config.Config{RestartStrategy: "systemd"}, "x"); !errors.Is(err, ErrPreviousImageMissing) {
		t.Fatalf("systemd deployments have no image to restore: %v", err)
	}
}

func TestPruneManagerImagesOnlyTargetsProjectManager(t *testing.T) {
	log := fakeDocker(t, runningManager, false)
	if err := PruneManagerImages(context.Background(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	commands := readCommands(t, log)
	want := "image prune --force --filter label=com.docker.compose.project=camera-appliance --filter label=com.docker.compose.service=camera-manager"
	if !strings.Contains(commands, want) {
		t.Fatalf("missing %q: %s", want, commands)
	}
	// --all would also remove tagged images such as :previous.
	if strings.Contains(commands, "--all") || strings.Contains(commands, " -a") {
		t.Fatalf("prune must stay limited to dangling images: %s", commands)
	}
}

func TestPreviousImageRef(t *testing.T) {
	for ref, want := range map[string]string{
		"camera-appliance-camera-manager":           "camera-appliance-camera-manager:previous",
		"camera-appliance-camera-manager:latest":    "camera-appliance-camera-manager:previous",
		"registry.local:5000/team/manager":          "registry.local:5000/team/manager:previous",
		"registry.local:5000/team/manager:1.2":      "registry.local:5000/team/manager:previous",
		"team/manager@sha256:0123456789abcdef01234": "team/manager:previous",
	} {
		if got := previousImageRef(ref); got != want {
			t.Errorf("previousImageRef(%q) = %q, want %q", ref, got, want)
		}
	}
}
