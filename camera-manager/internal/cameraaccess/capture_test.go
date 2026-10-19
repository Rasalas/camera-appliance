package cameraaccess_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/state"
)

var testJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 'j', 'p', 'e', 'g'}

func TestFrameCapturesFromGo2RTCWithStoredCredentials(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	var changed []string
	s.CredentialsChanged = func(deviceID string) { changed = append(changed, deviceID) }
	if _, err := s.SaveCredentials(ctx, "device", cameraaccess.CredentialsInput{Username: "user", Password: "stored-secret", Stream: "stream2"}); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "device" {
		t.Fatalf("credential change hook calls = %v", changed)
	}
	go2rtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/frame.jpeg" || r.URL.Query().Get("src") != "cam1" {
			t.Errorf("unexpected go2rtc request %s", r.URL)
		}
		_, _ = w.Write(testJPEG)
	}))
	defer go2rtc.Close()
	var requestedStream string
	s.Restream = func(_ context.Context, device state.Device, stream string) (string, bool) {
		requestedStream = stream
		return go2rtc.URL + "/api/frame.jpeg?src=cam1", device.ID == "device"
	}
	s.Capture = func(context.Context, string, string) ([]byte, error) {
		t.Fatal("direct camera capture opened despite healthy go2rtc")
		return nil, nil
	}

	got, err := s.Frame(ctx, "device", cameraaccess.FrameInput{CredentialsInput: cameraaccess.CredentialsInput{Username: "user"}})
	if err != nil {
		t.Fatal(err)
	}
	image, _ := base64.StdEncoding.DecodeString(got.ImageBase64)
	if got.CredentialSource != "go2rtc" || string(image) != string(testJPEG) || requestedStream != "stream2" {
		t.Fatalf("frame %+v stream=%s", got, requestedStream)
	}
	if !strings.Contains(got.URLRedacted, "/api/frame.jpeg") {
		t.Fatalf("expected go2rtc frame URL, got %s", got.URLRedacted)
	}
}

func TestFrameFallsBackToCameraWhenGo2RTCCannotServeFrame(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	if _, err := s.SaveCredentials(ctx, "device", cameraaccess.CredentialsInput{Username: "user", Password: "stored-secret", Stream: "stream2"}); err != nil {
		t.Fatal(err)
	}
	// go2rtc answers 200 with an empty body when it cannot attach a consumer.
	go2rtc := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer go2rtc.Close()
	s.Restream = func(context.Context, state.Device, string) (string, bool) {
		return go2rtc.URL + "/api/frame.jpeg?src=cam1", true
	}
	direct := 0
	s.Capture = func(context.Context, string, string) ([]byte, error) {
		direct++
		return testJPEG, nil
	}
	got, err := s.Frame(ctx, "device", cameraaccess.FrameInput{})
	if err != nil {
		t.Fatal(err)
	}
	if direct != 1 || got.CredentialSource != "kamera" {
		t.Fatalf("expected direct fallback, got %+v direct=%d", got, direct)
	}
}

func TestFrameTestsNewCredentialsAgainstCamera(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	if _, err := s.SaveCredentials(ctx, "device", cameraaccess.CredentialsInput{Username: "user", Password: "stored-secret"}); err != nil {
		t.Fatal(err)
	}
	s.Restream = func(context.Context, state.Device, string) (string, bool) {
		t.Fatal("new credentials were routed through go2rtc")
		return "", false
	}
	s.Capture = func(context.Context, string, string) ([]byte, error) { return testJPEG, nil }
	for _, input := range []cameraaccess.CredentialsInput{{Username: "user", Password: "new-secret"}, {Username: "other-user"}} {
		if _, err := s.Frame(ctx, "device", cameraaccess.FrameInput{CredentialsInput: input}); err != nil {
			t.Fatal(err)
		}
	}
}
