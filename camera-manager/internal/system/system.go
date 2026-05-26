package system

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

type Status struct {
	AgentDVR          ServiceStatus `json:"agentdvr"`
	Go2RTC            ServiceStatus `json:"go2rtc"`
	CameraAppliance   ServiceStatus `json:"camera_appliance"`
	LastDiscoveryText string        `json:"last_discovery_text,omitempty"`
}

type ServiceStatus struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Message string `json:"message,omitempty"`
}

func Check(ctx context.Context, cfg config.Config) Status {
	return Status{
		AgentDVR:        httpStatus(ctx, "AgentDVR", cfg.AgentDVRURL),
		Go2RTC:          httpStatus(ctx, "go2rtc", cfg.Go2RTCURL),
		CameraAppliance: ServiceStatus{Name: "camera-appliance", Online: true, Message: "läuft"},
	}
}

func RestartGo2RTC(ctx context.Context, cfg config.Config) error {
	return dockerCompose(ctx, cfg, "restart", "go2rtc")
}

func RestartStack(ctx context.Context, cfg config.Config) error {
	return dockerCompose(ctx, cfg, "restart", "agentdvr", "go2rtc", "camera-manager")
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
	full := append([]string{"compose"}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	if cfg.ComposeFile != "" {
		cmd.Dir = "."
		full = append([]string{"compose", "-f", cfg.ComposeFile}, args...)
		cmd = exec.CommandContext(ctx, "docker", full...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %v failed: %w: %s", full, err, string(out))
	}
	return nil
}
