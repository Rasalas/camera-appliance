package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/state"
)

func TestDeviceNameCanBeSetValidatedAndCleared(t *testing.T) {
	ctx := context.Background()
	a := newAuthTestApp(t)
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20", Manufacturer: "Tapo", Model: "C200"}); err != nil {
		t.Fatal(err)
	}
	h := New(a).Handler()

	res := performJSON(h, http.MethodPut, "/api/devices/dev1/name", map[string]string{"name": "  Einfahrt "}, nil)
	var device state.Device
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &device) != nil || device.Name != "Einfahrt" {
		t.Fatalf("rename: %d %s", res.Code, res.Body.String())
	}
	listed := performJSON(h, http.MethodGet, "/api/devices", nil, nil)
	if !strings.Contains(listed.Body.String(), `"name":"Einfahrt"`) {
		t.Fatalf("device list misses name: %s", listed.Body.String())
	}

	res = performJSON(h, http.MethodPut, "/api/devices/dev1/name", map[string]string{"name": strings.Repeat("x", 41)}, nil)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "40 Zeichen") {
		t.Fatalf("too long: %d %s", res.Code, res.Body.String())
	}
	res = performJSON(h, http.MethodPut, "/api/devices/missing/name", map[string]string{"name": "Hof"}, nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing device: %d %s", res.Code, res.Body.String())
	}

	res = performJSON(h, http.MethodPut, "/api/devices/dev1/name", map[string]string{"name": ""}, nil)
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), `"name"`) {
		t.Fatalf("clear: %d %s", res.Code, res.Body.String())
	}
}
