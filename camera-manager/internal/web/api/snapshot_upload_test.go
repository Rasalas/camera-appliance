package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/snapshotupload"
	"camera-appliance/camera-manager/internal/state"
)

func TestSnapshotUploadAPISettingsCropAndFreshCapture(t *testing.T) {
	a := newAuthTestApp(t)
	ctx := context.Background()
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "device", LastIP: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	s := New(a)
	var frame bytes.Buffer
	if err := jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 100, 80)), nil); err != nil {
		t.Fatal(err)
	}
	captures, sends := 0, 0
	s.cameras.Capture = func(context.Context, string, string) ([]byte, error) { captures++; return frame.Bytes(), nil }
	s.uploads.Send = func(_ context.Context, _ snapshotupload.Config, password, _ string, data []byte) error {
		sends++
		if password != "server-test-secret" {
			t.Fatal("camera credentials used as upload credentials")
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 50 || cfg.Height != 40 {
			t.Fatalf("unexpected JPEG: %+v %v", cfg, err)
		}
		return nil
	}
	h := s.Handler()
	config := map[string]any{"protocol": "ftp", "host": "localhost", "port": 21, "username": "upload-user", "directory": "/images", "host_key": "", "password": "server-test-secret"}
	response := performJSON(h, http.MethodPut, "/api/snapshot-upload", config, nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "server-test-secret") {
		t.Fatalf("settings response %d %s", response.Code, response.Body)
	}
	for _, endpoint := range []string{"/api/settings", "/api/snapshot-upload"} {
		response = performJSON(h, http.MethodGet, endpoint, nil, nil)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "server-test-secret") {
			t.Fatalf("password in %s", endpoint)
		}
	}
	crop := snapshotupload.Crop{Enabled: true, X: 50, Y: 25, Width: 50, Height: 50}
	response = performJSON(h, http.MethodPut, "/api/devices/device/upload-crop", crop, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("save crop %d: %s", response.Code, response.Body)
	}
	response = performJSON(h, http.MethodGet, "/api/devices/device/upload-crop", nil, nil)
	var saved snapshotupload.Crop
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil || saved != crop {
		t.Fatalf("crop not persisted: %s", response.Body)
	}
	response = performJSON(h, http.MethodPost, "/api/devices/device/upload-snapshot", map[string]any{"username": "camera-user", "password": "camera-test-secret", "stream": "stream2", "crop": crop}, nil)
	if response.Code != http.StatusOK || captures != 1 || sends != 1 || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("upload %d: %s", response.Code, response.Body)
	}
	response = performJSON(h, http.MethodPost, "/api/devices/device/upload-snapshot", map[string]any{"crop": map[string]any{"enabled": true, "width": 200, "height": 50}}, nil)
	if response.Code != http.StatusBadRequest || captures != 1 || sends != 1 {
		t.Fatal("invalid crop reached camera or server")
	}
	response = performJSON(h, http.MethodPost, "/api/devices/device/upload-snapshot", map[string]any{"password": "do-not-echo", "unknown-secret": "do-not-echo"}, nil)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "do-not-echo") {
		t.Fatal("malformed request was accepted or leaked")
	}
	response = performJSON(h, http.MethodGet, "/api/devices/missing/upload-crop", nil, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing camera: %d", response.Code)
	}
}

func TestSnapshotEndpointsRequireAdmin(t *testing.T) {
	a := newAuthTestApp(t)
	ctx := context.Background()
	if err := a.SetAuthPassword(ctx, "admin", "admin-pass"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAuthPassword(ctx, "viewer", "viewer-pass"); err != nil {
		t.Fatal(err)
	}
	h := New(a).Handler()
	cookie := loginCookie(t, h, "viewer", "viewer-pass")
	for _, route := range []struct{ method, path string }{{"GET", "/api/snapshot-upload"}, {"PUT", "/api/snapshot-upload"}, {"GET", "/api/devices/device/upload-crop"}, {"PUT", "/api/devices/device/upload-crop"}, {"POST", "/api/devices/device/upload-snapshot"}} {
		if res := performJSON(h, route.method, route.path, nil, cookie); res.Code != http.StatusForbidden {
			t.Fatalf("viewer allowed %s %s: %d", route.method, route.path, res.Code)
		}
		if res := performJSON(h, route.method, route.path, nil, nil); res.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous allowed %s %s: %d", route.method, route.path, res.Code)
		}
	}
}
