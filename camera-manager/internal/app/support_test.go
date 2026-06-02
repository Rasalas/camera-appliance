package app

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/state"
)

func TestCreateSupportBundleIncludesDiagnosticsAndRedactsSecrets(t *testing.T) {
	ctx := context.Background()
	t.Setenv("PATH", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" {
			fmt.Fprint(w, `{"cam1":{"url":"rtsp://user:rtsp-pass-123@192.168.1.20:554/stream2"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	a := newViewerTestApp(t, srv.URL, "rtsp-pass-123")
	if err := a.Store.PutSettings(ctx, map[string]string{
		"camera.credentials.dev1.username": "user",
		"camera.credentials.dev1.stream":   "stream2",
		"camera.api_token":                 "top-token-123",
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertDevice(ctx, state.Device{ID: "dev1", LastIP: "192.168.1.20", Manufacturer: "Tapo", Model: "C310"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.UpsertBinding(ctx, state.Binding{SlotID: "cam1", DeviceID: "dev1", Label: "Hof", Username: "user", StreamName: "stream2", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	writeGeneratedGo2RTC(t, a.Config, "streams:\n  cam1:\n    - rtsp://user:rtsp-pass-123@192.168.1.20:554/stream2\n")

	result, err := a.CreateSupportBundle(ctx, filepath.Join(t.TempDir(), "support.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	entries := readSupportBundle(t, result.Path)
	for _, name := range []string{
		"status.json",
		"viewer.json",
		"network.txt",
		"events.json",
		"settings.redacted.json",
		"version.json",
		"docker.txt",
		"go2rtc-streams.redacted.json",
		"go2rtc.yaml.redacted",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("support bundle missing %s; got %v", name, result.Files)
		}
	}
	combined := strings.Join(mapValues(entries), "\n")
	if strings.Contains(combined, "rtsp-pass-123") {
		t.Fatalf("support bundle leaked RTSP password:\n%s", combined)
	}
	if strings.Contains(combined, "top-token-123") {
		t.Fatalf("support bundle leaked token:\n%s", combined)
	}
	if !strings.Contains(entries["go2rtc.yaml.redacted"], "rtsp://user:******@192.168.1.20:554/stream2") {
		t.Fatalf("go2rtc config was not redacted:\n%s", entries["go2rtc.yaml.redacted"])
	}
	if !strings.Contains(entries["settings.redacted.json"], `"camera.credentials.dev1.username": "user"`) {
		t.Fatalf("settings redaction hid useful credential metadata:\n%s", entries["settings.redacted.json"])
	}
	if !strings.Contains(entries["settings.redacted.json"], `"camera.api_token": "******"`) {
		t.Fatalf("settings redaction did not hide token:\n%s", entries["settings.redacted.json"])
	}
}

func readSupportBundle(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string]string{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = string(data)
	}
	return entries
}

func mapValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
