package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/config"
)

// HTTPVersionHealthcheck rejects an old but still healthy manager. A successful
// restart command alone does not prove the requested release is serving traffic.
func HTTPVersionHealthcheck(cfg config.Config, expected Manifest) func(context.Context) error {
	return func(ctx context.Context) error {
		checkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		endpoint := strings.TrimRight(managerBaseURL(cfg.BindAddr), "/") + "/api/health"
		var lastErr error
		for {
			body, status, err := waitHTTPStatus(checkCtx, endpoint, 2*time.Second)
			var actual struct {
				Status  string `json:"status"`
				Version string `json:"version"`
				Commit  string `json:"commit"`
			}
			if err == nil {
				err = json.Unmarshal(body, &actual)
				if err == nil && (status != http.StatusOK || actual.Status != "ok" || actual.Version != expected.Version || (expected.Commit != "" && actual.Commit != expected.Commit)) {
					err = fmt.Errorf("manager reports %s (%s), expected %s (%s)", actual.Version, actual.Commit, expected.Version, expected.Commit)
				}
			}
			if err == nil {
				return HTTPHealthcheck(cfg)(ctx)
			}
			lastErr = err
			select {
			case <-checkCtx.Done():
				return fmt.Errorf("release healthcheck failed: %v: %w", lastErr, checkCtx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
}
