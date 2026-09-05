package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"camera-appliance/camera-manager/internal/snapshotupload"
	"camera-appliance/camera-manager/internal/state"
)

func TestUploadImageSettingsAPI(t *testing.T) {
	a := newAuthTestApp(t)
	if err := a.Store.UpsertDevice(context.Background(), state.Device{ID: "device"}); err != nil {
		t.Fatal(err)
	}
	h := New(a).Handler()
	endpoint := "/api/devices/device/upload-image-settings"
	res := performJSON(h, "GET", endpoint, nil, nil)
	var got snapshotupload.ImageSettings
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil || res.Code != http.StatusOK || got.Masks == nil || len(got.Masks) != 0 || got.Timestamp {
		t.Fatalf("unsafe default: %s", res.Body)
	}
	want := snapshotupload.ImageSettings{Masks: []snapshotupload.Mask{{ID: "one", Mode: "black", X: 10, Y: 10, Width: 20, Height: 20}, {ID: "two", Mode: "pixelate", X: 50, Y: 50, Width: 20, Height: 20}}, Timestamp: true}
	res = performJSON(h, "PUT", endpoint, want, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("save settings: %d %s", res.Code, res.Body)
	}
	res = performJSON(New(a).Handler(), "GET", endpoint, nil, nil)
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("privacy lost after restart: %s", res.Body)
	}
	for _, body := range []any{nil, map[string]any{"timestamp": true}, map[string]any{"masks": nil}, map[string]any{"masks": []any{}, "bypass": true}, map[string]any{"masks": []any{map[string]any{"id": "x", "mode": "black", "width": 20, "height": 20, "enabled": false}}}} {
		res = performJSON(h, "PUT", endpoint, body, nil)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("invalid privacy accepted: %d %s", res.Code, res.Body)
		}
	}
	res = performJSON(h, "GET", "/api/devices/missing/upload-image-settings", nil, nil)
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing device: %d", res.Code)
	}
	res = performJSON(h, "POST", "/api/devices/device/upload-snapshot", map[string]any{"crop": map[string]any{"enabled": false}, "masks": []any{}}, nil)
	if res.Code != http.StatusBadRequest {
		t.Fatal("manual request could override saved privacy")
	}
}
