package system

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

type Status struct {
	Go2RTC            ServiceStatus   `json:"go2rtc"`
	CameraAppliance   ServiceStatus   `json:"camera_appliance"`
	Systemd           []ServiceStatus `json:"systemd,omitempty"`
	Docker            []ServiceStatus `json:"docker,omitempty"`
	LastDiscoveryText string          `json:"last_discovery_text,omitempty"`
}

type ServiceStatus struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Message string `json:"message,omitempty"`
}

func Check(ctx context.Context, cfg config.Config) Status {
	status := Status{
		Go2RTC:          httpStatus(ctx, "go2rtc", cfg.Go2RTCURL),
		CameraAppliance: ServiceStatus{Name: "camera-appliance", Online: true, Message: "läuft"},
		Systemd:         systemdStatuses(ctx, "camera-appliance.service"),
		Docker:          dockerComposeStatuses(ctx, cfg, "go2rtc", "camera-manager"),
	}
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		status.Systemd = systemdUserStatuses(ctx, "camera-appliance.service", "camera-appliance-go2rtc.service")
		status.Docker = nil
	}
	return status
}

func RestartGo2RTC(ctx context.Context, cfg config.Config) error {
	if cfg.Go2RTCRestart != "" {
		cmd := exec.CommandContext(ctx, cfg.Go2RTCRestart)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go2rtc restart command failed: %w: %s", err, string(out))
		}
		return nil
	}
	if os.Getenv("CAMERA_APPLIANCE_DEV_GO2RTC_NATIVE") == "1" {
		return restartNativeDevGo2RTC(ctx, cfg)
	}
	return dockerCompose(ctx, cfg, "up", "-d", "--force-recreate", "go2rtc")
}

func RestartStack(ctx context.Context, cfg config.Config) error {
	return ApplyStack(ctx, cfg)
}

func ApplyStack(ctx context.Context, cfg config.Config) error {
	// Native (non-Docker) deployments restart via systemd instead of compose.
	if strings.EqualFold(cfg.RestartStrategy, "systemd") {
		return applyStackSystemd(ctx, cfg)
	}
	image, imageFound := currentContainerImage(ctx)
	detached, err := applyStackMode(image, imageFound, runningInContainer())
	if err != nil {
		return err
	}
	if detached {
		return launchDetachedCompose(ctx, cfg, image, "up", "-d", "--build", "--force-recreate", "--remove-orphans")
	}
	return dockerCompose(ctx, cfg, "up", "-d", "--build", "--force-recreate", "--remove-orphans")
}

// applyStackSystemd restarts go2rtc via the configured command (or its unit)
// and then schedules a manager restart. Updates use ApplyStackAndWait from
// their independent supervisor instead.
func applyStackSystemd(ctx context.Context, cfg config.Config) error {
	if cfg.Go2RTCRestart != "" {
		cmd := exec.CommandContext(ctx, cfg.Go2RTCRestart)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go2rtc restart command failed: %w: %s", err, string(out))
		}
	} else if err := systemctlTry(ctx, true, "restart", "camera-appliance-go2rtc"); err != nil {
		return err
	}
	return systemctlTry(ctx, true, "--no-block", "restart", "camera-appliance")
}

func systemctlTry(ctx context.Context, user bool, args ...string) error {
	args = systemctlArgs(user, args...)
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

func runningInContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

func applyStackMode(image string, imageFound, inContainer bool) (bool, error) {
	if imageFound && image != "" {
		return true, nil
	}
	if inContainer {
		return false, errors.New("cannot safely recreate camera-manager: current container image could not be determined")
	}
	return false, nil
}

func currentContainerImage(ctx context.Context) (string, bool) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "", false
	}
	output, err := commandOutput(ctx, 2*time.Second, "docker", "inspect", "--format", "{{.Image}}", hostname)
	if err != nil {
		return "", false
	}
	image := strings.TrimSpace(output)
	return image, image != ""
}

func launchDetachedCompose(ctx context.Context, cfg config.Config, image string, composeCommand ...string) error {
	name := fmt.Sprintf("camera-appliance-stack-updater-%d", time.Now().UnixNano())
	args := detachedComposeArgs(cfg, image, name, composeCommand...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %v failed: %w: %s", args, err, string(out))
	}
	return nil
}

func detachedComposeArgs(cfg config.Config, image, name string, composeCommand ...string) []string {
	composeDir := filepath.Dir(cfg.ComposeFile)
	dirs := []string{composeDir}
	releaseEnv := filepath.Join(composeDir, "release.env")
	args := []string{"run", "--rm", "-d", "--name", name, "--entrypoint", "docker-compose", "-v", "/var/run/docker.sock:/var/run/docker.sock", "-w", composeDir}
	for _, dir := range dirs {
		args = append(args, "-v", dir+":"+dir)
	}
	args = append(args, image)
	if pathExists(releaseEnv) {
		args = append(args, "--env-file", releaseEnv)
	}
	args = append(args, "-f", cfg.ComposeFile)
	return append(args, composeCommand...)
}

func httpStatus(ctx context.Context, name, rawURL string) ServiceStatus {
	reqCtx, cancel := context.WithTimeout(ctx, 700*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ServiceStatus{Name: name, Online: false, Message: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ServiceStatus{Name: name, Online: false, Message: "nicht erreichbar"}
	}
	_ = resp.Body.Close()
	online := resp.StatusCode >= 200 && resp.StatusCode < 300
	return ServiceStatus{Name: name, Online: online, Message: resp.Status}
}

func dockerCompose(ctx context.Context, cfg config.Config, args ...string) error {
	full := composeArgs(cfg, args...)
	command := "docker"
	commandArgs := full
	if err := exec.CommandContext(ctx, "docker", "compose", "version").Run(); err != nil {
		if _, lookupErr := exec.LookPath("docker-compose"); lookupErr == nil {
			command = "docker-compose"
			commandArgs = full[1:]
		}
	}
	cmd := exec.CommandContext(ctx, command, commandArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", command, commandArgs, err, string(out))
	}
	return nil
}

func restartNativeDevGo2RTC(ctx context.Context, cfg config.Config) error {
	cmd := exec.CommandContext(ctx, "make", "-C", cfg.ConfigDir, "dev-go2rtc")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make dev-go2rtc failed: %w: %s", err, string(out))
	}
	return nil
}

func systemdStatuses(ctx context.Context, services ...string) []ServiceStatus {
	return systemdStatusesWithArgs(ctx, false, services...)
}

func systemdUserStatuses(ctx context.Context, services ...string) []ServiceStatus {
	return systemdStatusesWithArgs(ctx, true, services...)
}

func systemdStatusesWithArgs(ctx context.Context, user bool, services ...string) []ServiceStatus {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return []ServiceStatus{{Name: "systemd", Online: false, Message: "systemctl nicht verfügbar"}}
	}
	out := make([]ServiceStatus, 0, len(services))
	for _, service := range services {
		stateArgs := systemctlArgs(user, "is-active", service)
		state, err := commandOutput(ctx, 900*time.Millisecond, "systemctl", stateArgs...)
		state = strings.TrimSpace(state)
		if state == "" && err != nil {
			state = shortError(err)
		}
		enabledArgs := systemctlArgs(user, "is-enabled", service)
		enabled, enabledErr := commandOutput(ctx, 900*time.Millisecond, "systemctl", enabledArgs...)
		enabled = strings.TrimSpace(enabled)
		message := state
		if enabled != "" {
			message += ", " + enabled
		} else if enabledErr != nil {
			message += ", " + shortError(enabledErr)
		}
		out = append(out, ServiceStatus{Name: service, Online: state == "active", Message: message})
	}
	return out
}

func systemctlArgs(user bool, args ...string) []string {
	if !user {
		return args
	}
	return append([]string{"--user"}, args...)
}

func dockerComposeStatuses(ctx context.Context, cfg config.Config, services ...string) []ServiceStatus {
	if _, err := exec.LookPath("docker"); err != nil {
		return []ServiceStatus{{Name: "docker compose", Online: false, Message: "docker nicht verfügbar"}}
	}
	source := "compose"
	args := composeArgs(cfg, "ps", "--services", "--filter", "status=running")
	output, err := commandOutput(ctx, 1800*time.Millisecond, "docker", args...)
	running := serviceSet(output)
	if err != nil {
		fallback, fallbackErr := commandOutput(ctx, 3500*time.Millisecond, "docker", "ps", "--format", "{{.Names}}")
		if fallbackErr != nil {
			return []ServiceStatus{{Name: "docker", Online: false, Message: shortError(fmt.Errorf("%w: %s", err, output))}}
		}
		running = serviceSet(fallback)
		source = "docker ps"
	} else if len(running) == 0 {
		fallback, fallbackErr := commandOutput(ctx, 3500*time.Millisecond, "docker", "ps", "--format", "{{.Names}}")
		if fallbackErr == nil && strings.TrimSpace(fallback) != "" {
			running = serviceSet(fallback)
			source = "docker ps"
		}
	}
	out := make([]ServiceStatus, 0, len(services))
	for _, service := range services {
		if running[service] {
			out = append(out, ServiceStatus{Name: service, Online: true, Message: "running (" + source + ")"})
			continue
		}
		out = append(out, ServiceStatus{Name: service, Online: false, Message: "nicht running (" + source + ")"})
	}
	return out
}

func serviceSet(output string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func composeArgs(cfg config.Config, args ...string) []string {
	full := []string{"compose"}
	if cfg.ComposeFile != "" {
		if releaseEnv := filepath.Join(filepath.Dir(cfg.ComposeFile), "release.env"); pathExists(releaseEnv) {
			full = append(full, "--env-file", releaseEnv)
		}
		full = append(full, "-f", cfg.ComposeFile)
	}
	return append(full, args...)
}

func commandOutput(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()
	if cmdCtx.Err() != nil {
		return string(out), cmdCtx.Err()
	}
	return string(out), err
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.Join(strings.Fields(err.Error()), " ")
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
}
