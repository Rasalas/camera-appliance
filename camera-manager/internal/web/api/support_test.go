package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"camera-appliance/camera-manager/internal/state"
)

func TestSupportReportMasksCredentialsAndOmitsDetails(t *testing.T) {
	a := newAuthTestApp(t)
	ctx := context.Background()
	if err := a.Store.AddEvent(ctx, "error", "camera.failed", "rtsp://viewer:fixture-secret@192.0.2.1/live", map[string]string{"password": "private-detail"}); err != nil {
		t.Fatal(err)
	}
	response := performJSON(New(a).Handler(), http.MethodGet, "/api/support", nil, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var report struct {
		Events []state.Event `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 1 || !strings.Contains(report.Events[0].Message, "192.0.2.1") {
		t.Fatal("missing diagnostic event")
	}
	if strings.Contains(response.Body.String(), "fixture-secret") || strings.Contains(response.Body.String(), "private-detail") || strings.Contains(response.Body.String(), "details_json") {
		t.Fatal("unredacted diagnostics")
	}
}

func TestSupportDownloadReturnsArchiveWithoutArbitraryFileAccess(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a := newAuthTestApp(t)
	if err := a.Store.PutSettings(context.Background(), map[string]string{"camera.password": "fixture-secret"}); err != nil {
		t.Fatal(err)
	}
	response := performJSON(New(a).Handler(), http.MethodPost, "/api/support-bundle/download?path=/etc/passwd", map[string]string{"out": "/not-allowed"}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/gzip" || !strings.Contains(response.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("not a downloadable archive")
	}
	gz, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	names := map[string]bool{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "fixture-secret") {
			t.Fatal("secret in bundle")
		}
		names[header.Name] = true
	}
	for _, name := range []string{"version.json", "events.json", "settings.redacted.json"} {
		if !names[name] {
			t.Fatalf("missing %s", name)
		}
	}
	entries, err := os.ReadDir(a.Config.BackupDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("temporary archive was not removed")
	}
}

func TestSupportEndpointsRequireAdmin(t *testing.T) {
	a := newAuthTestApp(t)
	for _, role := range []string{"admin", "viewer"} {
		if err := a.SetAuthPassword(context.Background(), role, "fixture-pass"); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(a).Handler()
	viewer := loginCookie(t, handler, "viewer", "fixture-pass")
	for _, endpoint := range []struct{ method, path string }{{http.MethodGet, "/api/support"}, {http.MethodPost, "/api/support-bundle/download"}} {
		for _, cookie := range []*http.Cookie{nil, viewer} {
			response := performJSON(handler, endpoint.method, endpoint.path, map[string]string{}, cookie)
			if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
				t.Fatalf("%s exposed with status %d", endpoint.path, response.Code)
			}
		}
	}
}

func TestSupportReportIncludesUpTo100EventsForDownload(t *testing.T) {
	a := newAuthTestApp(t)
	for i := 0; i < 101; i++ {
		if err := a.Store.AddEvent(context.Background(), "info", "test", "fixture event", nil); err != nil {
			t.Fatal(err)
		}
	}
	response := performJSON(New(a).Handler(), http.MethodGet, "/api/support", nil, nil)
	var report struct {
		Events []state.Event `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 100 {
		t.Fatalf("download should include 100 recent events, got %d", len(report.Events))
	}
}
