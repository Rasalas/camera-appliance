package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"camera-appliance/camera-manager/internal/state"
)

func TestGo2RTCFrameURLPrefersViewerAliasWhileGo2RTCIsHealthy(t *testing.T) {
	ctx := context.Background()
	var down atomic.Bool
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer go2rtc.Close()
	a := newViewerTestApp(t, go2rtc.URL, "secret")
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1: [x]\n  cam1_stream1: [x]\n  cam1_stream2: [x]\n")
	device := state.Device{ID: "dev1"}

	for stream, want := range map[string]string{
		"stream2": go2rtc.URL + "/api/frame.jpeg?src=cam1",
		"stream1": go2rtc.URL + "/api/frame.jpeg?src=cam1_stream1",
	} {
		if got, ok := a.Go2RTCFrameURL(ctx, device, stream); !ok || got != want {
			t.Fatalf("%s: got %q %v, want %q", stream, got, ok, want)
		}
	}
	if _, ok := a.Go2RTCFrameURL(ctx, state.Device{ID: "unbound"}, "stream2"); ok {
		t.Fatal("unbound device must capture directly")
	}
	down.Store(true)
	if _, ok := a.Go2RTCFrameURL(ctx, device, "stream2"); ok {
		t.Fatal("unhealthy go2rtc must not be used for capture")
	}
}
