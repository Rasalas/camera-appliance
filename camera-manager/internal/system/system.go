package system

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
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
	return Status{
		Go2RTC:          httpStatus(ctx, "go2rtc", cfg.Go2RTCURL),
		CameraAppliance: ServiceStatus{Name: "camera-appliance", Online: true, Message: "läuft"},
		Systemd:         systemdStatuses(ctx, "camera-appliance.service"),
		Docker:          dockerComposeStatuses(ctx, cfg, "go2rtc", "camera-manager"),
	}
}

func RestartGo2RTC(ctx context.Context, cfg config.Config) error {
	return dockerCompose(ctx, cfg, "restart", "go2rtc")
}

func RestartStack(ctx context.Context, cfg config.Config) error {
	return dockerCompose(ctx, cfg, "restart", "go2rtc", "camera-manager")
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
	return ServiceStatus{Name: name, Online: resp.StatusCode < 500, Message: resp.Status}
}

func dockerCompose(ctx context.Context, cfg config.Config, args ...string) error {
	full := composeArgs(cfg, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %v failed: %w: %s", full, err, string(out))
	}
	return nil
}

func systemdStatuses(ctx context.Context, services ...string) []ServiceStatus {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return []ServiceStatus{{Name: "systemd", Online: false, Message: "systemctl nicht verfügbar"}}
	}
	out := make([]ServiceStatus, 0, len(services))
	for _, service := range services {
		state, err := commandOutput(ctx, 900*time.Millisecond, "systemctl", "is-active", service)
		state = strings.TrimSpace(state)
		if state == "" && err != nil {
			state = shortError(err)
		}
		enabled, enabledErr := commandOutput(ctx, 900*time.Millisecond, "systemctl", "is-enabled", service)
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

func dockerComposeStatuses(ctx context.Context, cfg config.Config, services ...string) []ServiceStatus {
	if _, err := exec.LookPath("docker"); err != nil {
		return []ServiceStatus{{Name: "docker compose", Online: false, Message: "docker nicht verfügbar"}}
	}
	source := "compose"
	args := composeArgs(cfg, "ps", "--services", "--filter", "status=running")
	output, err := commandOutput(ctx, 1800*time.Millisecond, "docker", args...)
	running := serviceSet(output)
	if err != nil {
		fallback, fallbackErr := commandOutput(ctx, 1800*time.Millisecond, "docker", "ps", "--format", "{{.Names}}")
		if fallbackErr != nil {
			return []ServiceStatus{{Name: "docker compose", Online: false, Message: shortError(fmt.Errorf("%w: %s", err, output))}}
		}
		running = serviceSet(fallback)
		source = "docker ps"
	} else if len(running) == 0 {
		fallback, fallbackErr := commandOutput(ctx, 1800*time.Millisecond, "docker", "ps", "--format", "{{.Names}}")
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
