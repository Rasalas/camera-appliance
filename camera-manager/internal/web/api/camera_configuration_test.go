package api

import (
	"camera-appliance/camera-manager/internal/app"
	"camera-appliance/camera-manager/internal/state"
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCameraConfigurationRouteValidationAndPersistence(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.Store.UpsertDevice(context.Background(), state.Device{ID: "device"}); err != nil {
		t.Fatal(err)
	}
	h := New(a).Handler()
	response := performJSON(h, http.MethodPut, "/api/devices/device/configuration", map[string]any{"name": "Hof", "display": map[string]string{"rotation": "90"}}, nil)
	var result app.CameraConfigurationResult
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.Persisted || !result.Applied || result.Restarted {
		t.Fatalf("save: %d %s", response.Code, response.Body.String())
	}
	device, err := a.Store.Device(context.Background(), "device")
	if err != nil || device.Name != "Hof" {
		t.Fatalf("name: %+v %v", device, err)
	}
	for _, tc := range []struct {
		path   string
		body   any
		status int
	}{
		{"device", map[string]any{"path_policy": "wrong"}, 400},
		{"device", map[string]any{"endpoint": map[string]string{"host": "user:password@host", "port": "554"}}, 400},
		{"device", map[string]any{"display": map[string]string{"rotation": "45"}}, 400},
		{"device", map[string]any{"unknown": true}, 400},
		{"missing", map[string]any{}, 404},
	} {
		response := performJSON(h, http.MethodPut, "/api/devices/"+tc.path+"/configuration", tc.body, nil)
		if response.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, response.Code, response.Body.String())
		}
	}
}
