package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"camera-appliance/camera-manager/internal/config"
)

func TestGo2RTCRestartSurvivesClientDisconnect(t *testing.T) {
	a := newAuthTestApp(t)
	disconnect, serve := disconnectableRequest(http.MethodPost, "/api/go2rtc/restart")
	var restartCtxErr error
	restarts := 0
	a.Go2RTCRestart = func(ctx context.Context) error {
		restarts++
		disconnect()
		restartCtxErr = ctx.Err()
		if _, ok := ctx.Deadline(); !ok {
			t.Error("expected restart context to carry a deadline")
		}
		return nil
	}

	res := serve(New(a).Handler())
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if restarts != 1 || restartCtxErr != nil {
		t.Fatalf("expected restart to outlive the request, got %d restarts, ctx error %v", restarts, restartCtxErr)
	}
}

func TestStackRestartSurvivesClientDisconnect(t *testing.T) {
	a := newAuthTestApp(t)
	disconnect, serve := disconnectableRequest(http.MethodPost, "/api/system/restart-stack")
	var restartCtxErr error
	restarts := 0
	previous := restartStackFunc
	restartStackFunc = func(ctx context.Context, _ config.Config) error {
		restarts++
		disconnect()
		restartCtxErr = ctx.Err()
		return nil
	}
	t.Cleanup(func() { restartStackFunc = previous })

	res := serve(New(a).Handler())
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if restarts != 1 || restartCtxErr != nil {
		t.Fatalf("expected stack restart to outlive the request, got %d restarts, ctx error %v", restarts, restartCtxErr)
	}
}

// disconnectableRequest returns a cancel func that simulates the client leaving mid-request.
func disconnectableRequest(method, path string) (context.CancelFunc, func(http.Handler) *httptest.ResponseRecorder) {
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(method, path, nil).WithContext(ctx)
	return cancel, func(handler http.Handler) *httptest.ResponseRecorder {
		defer cancel()
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
}
