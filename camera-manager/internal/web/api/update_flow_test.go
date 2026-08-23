package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	updater "camera-appliance/camera-manager/internal/update"
)

func timeNow() time.Time                   { return time.Now() }
func timeNowAdd(d time.Duration) time.Time { return time.Now().Add(d) }
func timeSecond() time.Duration            { return time.Second }
func sleepShort()                          { time.Sleep(25 * time.Millisecond) }

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.2", "0.1.2", 0},
		{"v0.1.2", "0.1.3", -1},
		{"0.10.0", "0.9.9", 1},
		{"1.0", "1.0.0", 0},
		{"dev", "0.0.1", -1},
		{"0.2.0", "dev", 1},
		{"nas", "0.0.1", -1},
		{"nas", "v0.1.2", -1},
	}
	for _, tc := range cases {
		if got := updater.CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

const releasesPayload = `[
		{"tag_name":"v0.2.0","name":"0.2.0","body":"feat: second","published_at":"2026-08-01T00:00:00Z",
		 "assets":[{"name":"camera-appliance-0.2.0-abc.tar.gz","browser_download_url":"$BASE/assets/camera-appliance-0.2.0-abc.tar.gz"}]},
		{"tag_name":"v0.1.5","name":"0.1.5","body":"fix: first","published_at":"2026-07-01T00:00:00Z",
		 "assets":[{"name":"camera-appliance-latest.tar.gz","browser_download_url":"$BASE/assets/latest.tar.gz"}]},
		{"tag_name":"v0.1.2","name":"0.1.2","body":"old"}
	]`

func newFlowTestServer(t *testing.T) (*httptest.Server, *updateFlow) {
	t.Helper()
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.ReplaceAll(releasesPayload, "$BASE", baseURL+"/assets")))
	})
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write([]byte("fake-archive-bytes"))
	})
	server := httptest.NewServer(mux)
	baseURL = server.URL

	flow := newUpdateFlow(t.TempDir())
	flow.apiBase = server.URL
	flow.client = server.Client()
	return server, flow
}

func TestUpdateFlowCheckCollectsChangesSinceCurrent(t *testing.T) {
	_, flow := newFlowTestServer(t)

	if err := flow.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := flow.status()
	if st.Phase != updateAvailable {
		t.Fatalf("expected phase available, got %q (%s)", st.Phase, st.Error)
	}
	if st.Latest == nil || st.Latest.Tag != "v0.2.0" {
		t.Fatalf("unexpected latest: %+v", st.Latest)
	}
	// The test binary reports version "dev", so every release is newer.
	if len(st.Changes) != 3 {
		t.Fatalf("expected 3 changes since dev, got %+v", st.Changes)
	}
	foundFirstFix := false
	for _, change := range st.Changes {
		if strings.Contains(change.Notes, "fix: first") {
			foundFirstFix = true
		}
	}
	if !foundFirstFix {
		t.Fatalf("changelog should include intermediate releases: %+v", st.Changes)
	}
	if !strings.HasPrefix(st.Latest.URL, "http") {
		t.Fatalf("latest release must carry an absolute asset url, got %q", st.Latest.URL)
	}
}

func TestCollectNewerReleasesStopsAtCurrent(t *testing.T) {
	var releases []updater.Release
	payload := `[{"tag_name":"v0.2.0"},{"tag_name":"v0.1.5"},{"tag_name":"v0.1.2"}]`
	if err := json.Unmarshal([]byte(payload), &releases); err != nil {
		t.Fatal(err)
	}
	latest, changes := collectNewerReleases(releases, "0.1.2")
	if latest == nil || latest.Tag != "v0.2.0" {
		t.Fatalf("unexpected latest %+v", latest)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes since 0.1.2, got %d", len(changes))
	}
	_, none := collectNewerReleases(releases, "0.2.0")
	if none != nil {
		t.Fatalf("expected no changes when current is latest, got %+v", none)
	}
}

func TestUpdateFlowDownloadReachesReady(t *testing.T) {
	server, flow := newFlowTestServer(t)
	defer server.Close()
	if err := flow.check(context.Background()); err != nil {
		t.Fatal(err)
	}

	a := newAuthTestApp(t)
	s := New(a)
	s.updates = flow

	if err := s.startUpdateDownload(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := timeNowAdd(5 * timeSecond())
	for {
		st := flow.status()
		if st.Phase == updateReady || st.Phase == updateFailed {
			if st.Phase != updateReady {
				t.Fatalf("download failed: %s", st.Error)
			}
			if len(st.Digest) != 64 {
				t.Fatalf("expected sha256 digest, got %q", st.Digest)
			}
			if st.ArchiveName == "" {
				t.Fatal("expected archive name to be set")
			}
			break
		}
		if timeNow().After(deadline) {
			t.Fatalf("download did not finish in time, last phase %q", st.Phase)
		}
		sleepShort()
	}
}

func TestUpdateFlowInstallRejectsWrongPhase(t *testing.T) {
	_, flow := newFlowTestServer(t)
	a := newAuthTestApp(t)
	s := New(a)
	s.updates = flow

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/system/update/install", nil)
	s.startUpdateInstall(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 outside ready phase, got %d", rec.Code)
	}
}
