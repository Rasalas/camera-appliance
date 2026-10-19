package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/redaction"
)

// StartUpdateWorker starts outside the manager's container or service cgroup.
// executable and jobPath must be on persistent storage shared with the worker.
func StartUpdateWorker(ctx context.Context, cfg config.Config, executable, jobPath, id string) error {
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		args := []string{"--user", "--collect", "--unit=camera-appliance-update-" + id, executable, "update-worker", "--job", jobPath, "--job-id", id}
		out, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("start independent systemd updater: %w: %s", err, out)
		}
		return nil
	}
	if runningInContainer() {
		image, err := currentContainerImage(ctx)
		if err != nil {
			return fmt.Errorf("cannot start updater: current container image is unknown: %w", err)
		}
		out, err := exec.CommandContext(ctx, "docker", updateWorkerArgs(cfg, image, executable, jobPath, id)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("start independent Docker updater: %w: %s", err, out)
		}
		return nil
	}
	// A host CLI may exit while the detached child continues. Do not attach its
	// lifetime to the HTTP request or terminal, and reap it if we stay alive.
	log, err := os.OpenFile(filepath.Join(cfg.StateDir, "updates", "worker.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(executable, "update-worker", "--job", jobPath, "--job-id", id)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func updateWorkerArgs(cfg config.Config, image, executable, jobPath, id string) []string {
	args := []string{"run", "--rm", "-d", "--name", "camera-appliance-update-" + id,
		"--network", "host", "--entrypoint", executable,
		"-v", "/var/run/docker.sock:/var/run/docker.sock", "-w", cfg.InstallDir}
	seen := map[string]bool{}
	for _, dir := range []string{cfg.InstallDir, filepath.Dir(cfg.ComposeFile), cfg.ConfigDir, cfg.StateDir} {
		if !seen[dir] {
			args = append(args, "-v", dir+":"+dir)
			seen[dir] = true
		}
	}
	return append(args, image, "update-worker", "--job", jobPath, "--job-id", id)
}

// ApplyStackAndWait is for the independent updater, which survives both unit
// restarts and compose recreation. Returning means the restart command finished.
func ApplyStackAndWait(ctx context.Context, cfg config.Config) error {
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		if cfg.Go2RTCRestart != "" {
			if err := RestartGo2RTC(ctx, cfg); err != nil {
				return err
			}
		} else if err := systemctlTry(ctx, true, "restart", "camera-appliance-go2rtc"); err != nil {
			return err
		}
		return systemctlTry(ctx, true, "restart", "camera-appliance")
	}
	return dockerCompose(ctx, cfg, "up", "-d", "--build", "--force-recreate", "--remove-orphans")
}

// ErrPreviousImageMissing means rollback has to rebuild the restored sources.
var ErrPreviousImageMissing = errors.New("previous camera-manager image is missing")

// BuildStack builds new images while the running containers keep serving.
// ApplyStackAndWait afterwards reuses the build cache.
func BuildStack(ctx context.Context, cfg config.Config) error {
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		return nil
	}
	return dockerCompose(ctx, cfg, "build")
}

// PreserveManagerImage tags the running manager image as <repo>:previous and
// returns the reference Compose uses for it.
func PreserveManagerImage(ctx context.Context, cfg config.Config) (string, error) {
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		return "", nil
	}
	manager, err := inspectManagerContainer(ctx)
	if err != nil {
		return "", err
	}
	if manager.ref == "" || strings.HasPrefix(manager.ref, "sha256:") {
		return "", fmt.Errorf("manager container has no image name: %q", manager.ref)
	}
	if out, err := commandOutput(ctx, 30*time.Second, "docker", "tag", manager.id, previousImageRef(manager.ref)); err != nil {
		return "", fmt.Errorf("docker tag: %w: %s", err, redaction.Text(strings.TrimSpace(out)))
	}
	return manager.ref, nil
}

// RestorePreviousStackAndWait points ref back at the preserved image and
// recreates the stack without building.
func RestorePreviousStackAndWait(ctx context.Context, cfg config.Config, ref string) error {
	if strings.EqualFold(cfg.RestartStrategy, "systemd") || ref == "" {
		return ErrPreviousImageMissing
	}
	previous := previousImageRef(ref)
	if _, err := commandOutput(ctx, 30*time.Second, "docker", "image", "inspect", "--format", "{{.Id}}", previous); err != nil {
		return fmt.Errorf("%w: %s", ErrPreviousImageMissing, previous)
	}
	if out, err := commandOutput(ctx, 30*time.Second, "docker", "tag", previous, ref); err != nil {
		return fmt.Errorf("docker tag: %w: %s", err, redaction.Text(strings.TrimSpace(out)))
	}
	return dockerCompose(ctx, cfg, "up", "-d", "--no-build", "--force-recreate", "--remove-orphans")
}

// PruneManagerImages removes dangling images built for this project's manager
// service. Tagged images, including :previous, and other projects stay.
func PruneManagerImages(ctx context.Context, cfg config.Config) error {
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		return nil
	}
	manager, err := inspectManagerContainer(ctx)
	if err != nil {
		return err
	}
	if manager.project == "" {
		return errors.New("manager container has no compose project label")
	}
	out, err := commandOutput(ctx, 2*time.Minute, "docker", pruneManagerImagesArgs(manager.project)...)
	if err != nil {
		return fmt.Errorf("docker image prune: %w: %s", err, redaction.Text(strings.TrimSpace(out)))
	}
	return nil
}

func pruneManagerImagesArgs(project string) []string {
	return []string{"image", "prune", "--force",
		"--filter", "label=com.docker.compose.project=" + project,
		"--filter", "label=com.docker.compose.service=camera-manager"}
}

type managerContainer struct{ id, ref, project string }

func inspectManagerContainer(ctx context.Context) (managerContainer, error) {
	hostname, _ := os.Hostname()
	var failures []error
	for _, name := range []string{hostname, "camera-manager"} {
		if name == "" {
			continue
		}
		out, err := commandOutput(ctx, 5*time.Second, "docker", "inspect", "--type", "container", "--format",
			`{{.Image}}|{{.Config.Image}}|{{index .Config.Labels "com.docker.compose.service"}}|{{index .Config.Labels "com.docker.compose.project"}}`, name)
		if err != nil {
			failures = append(failures, fmt.Errorf("docker inspect %s: %w", name, err))
			continue
		}
		fields := strings.Split(strings.TrimSpace(out), "|")
		// The hostname may belong to an unrelated container.
		if len(fields) != 4 || fields[2] != "camera-manager" || fields[0] == "" {
			failures = append(failures, fmt.Errorf("docker inspect %s: not the camera-manager service", name))
			continue
		}
		return managerContainer{id: fields[0], ref: fields[1], project: fields[3]}, nil
	}
	return managerContainer{}, errors.Join(failures...)
}

func previousImageRef(ref string) string {
	repo := ref
	if at := strings.Index(repo, "@"); at >= 0 {
		repo = repo[:at]
	}
	if colon := strings.LastIndex(repo, ":"); colon > strings.LastIndex(repo, "/") {
		repo = repo[:colon]
	}
	return repo + ":previous"
}
