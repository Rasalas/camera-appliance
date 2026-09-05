package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"camera-appliance/camera-manager/internal/config"
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
		image, found := currentContainerImage(ctx)
		if !found {
			return fmt.Errorf("cannot start updater: current container image is unknown")
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
