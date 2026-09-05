package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/system"
)

func HTTPHealthcheck(cfg config.Config) func(context.Context) error {
	return func(ctx context.Context) error {
		managerBase := managerBaseURL(cfg.BindAddr)
		checks := []struct {
			name string
			url  string
		}{
			{name: "manager", url: strings.TrimRight(managerBase, "/") + "/api/health"},
			{name: "go2rtc", url: cfg.Go2RTCURL},
			{name: "viewer", url: strings.TrimRight(managerBase, "/") + "/api/viewer"},
		}
		var viewerBody []byte
		viewerProtected := false
		for _, check := range checks {
			body, status, err := waitHTTPStatus(ctx, check.url, 30*time.Second)
			if err != nil {
				return fmt.Errorf("%s healthcheck failed: %w", check.name, err)
			}
			if check.name == "viewer" {
				if status == http.StatusUnauthorized || status == http.StatusForbidden {
					// Viewer is auth-protected; reaching it means the service
					// is up. Slot validation is only possible when public.
					viewerProtected = true
					continue
				}
				viewerBody = body
			}
		}
		if viewerProtected {
			return nil
		}
		var viewer struct {
			Slots []json.RawMessage `json:"slots"`
		}
		if err := json.Unmarshal(viewerBody, &viewer); err != nil {
			return fmt.Errorf("viewer healthcheck returned invalid JSON: %w", err)
		}
		if len(viewer.Slots) < len(config.DefaultSlots()) {
			return fmt.Errorf("viewer healthcheck saw %d slots, expected at least %d", len(viewer.Slots), len(config.DefaultSlots()))
		}
		return nil
	}
}

func restartAndCheck(ctx context.Context, noRestart bool, restart func(context.Context) error, healthcheck func(context.Context) error) error {
	if !noRestart {
		if restart != nil {
			if err := restart(ctx); err != nil {
				return err
			}
		}
	}
	if healthcheck != nil {
		return healthcheck(ctx)
	}
	return nil
}

func managerBaseURL(bindAddr string) string {
	if strings.HasPrefix(bindAddr, "http://") || strings.HasPrefix(bindAddr, "https://") {
		return bindAddr
	}
	if host, port, err := net.SplitHostPort(bindAddr); err == nil && (host == "" || host == "0.0.0.0" || host == "::") {
		bindAddr = net.JoinHostPort("127.0.0.1", port)
	}
	return "http://" + bindAddr
}

func waitHTTP(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, error) {
	body, _, err := waitHTTPStatus(ctx, rawURL, timeout)
	return body, err
}

func waitHTTPStatus(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, int, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
				_ = resp.Body.Close()
				if readErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
					cancel()
					return body, resp.StatusCode, nil
				}
				if readErr != nil {
					lastErr = readErr
				} else if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
					// Auth-gated endpoints prove liveness even though the body
					// cannot be inspected.
					cancel()
					return nil, resp.StatusCode, nil
				} else {
					lastErr = fmt.Errorf("%s", resp.Status)
				}
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
		cancel()
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("timeout")
			}
			return nil, 0, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
}

func StackRestart(cfg config.Config) func(context.Context) error {
	return func(ctx context.Context) error {
		return system.ApplyStackAndWait(ctx, cfg)
	}
}
