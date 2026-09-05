package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
	updater "camera-appliance/camera-manager/internal/update"
	"camera-appliance/camera-manager/internal/version"
)

func timeNow() time.Time { return time.Now() }

func timeNowAdd(d time.Duration) time.Time { return time.Now().Add(d) }

func timeSecond() time.Duration { return time.Second }

func sleepShort() { time.Sleep(25 * time.Millisecond) }

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
	flow.catalog.BaseURL = server.URL
	flow.catalog.Client = server.Client()
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

func TestUpdateFlowCheckUsesCompareCommitsWhenReleaseHasNoDetails(t *testing.T) {
	compareCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"tag_name":"v0.1.8",
			"name":"camera-appliance v0.1.8",
			"body":"**Full Changelog**: https://github.com/Rasalas/camera-appliance/compare/v0.1.7...v0.1.8",
			"published_at":"2026-08-23T00:00:00Z"
		}]`))
	})
	mux.HandleFunc("/compare/v0.1.7...v0.1.8", func(w http.ResponseWriter, _ *http.Request) {
		compareCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"commits":[
			{"sha":"1234567890abcdef","commit":{"message":"feat: add update progress\n\nMore details"}},
			{"sha":"abcdef1234567890","commit":{"message":"fix: keep the sheet draggable"}}
		]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	flow := newUpdateFlow(t.TempDir())
	flow.catalog.BaseURL = server.URL
	flow.catalog.Client = server.Client()
	flow.currentVersion = "0.1.7"

	if err := flow.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := flow.status()
	if !compareCalled {
		t.Fatal("expected compare endpoint to be called")
	}
	if len(st.Changes) != 1 {
		t.Fatalf("expected one release, got %+v", st.Changes)
	}
	if !strings.Contains(st.Changes[0].Notes, "feat: add update progress") || !strings.Contains(st.Changes[0].Notes, "abcdef1") {
		t.Fatalf("expected commit messages in fallback notes, got %q", st.Changes[0].Notes)
	}
}

func TestUpdateFlowCheckReportsUpToDateOnEqualVersion(t *testing.T) {
	_, flow := newFlowTestServer(t)
	// Newest release in the payload is v0.2.0; the installation matches it.
	flow.currentVersion = "0.2.0"

	if err := flow.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := flow.status()
	if st.Phase != updateUpToDate {
		t.Fatalf("expected phase up_to_date, got %q (%s)", st.Phase, st.Error)
	}
	if st.Latest != nil {
		t.Fatalf("up_to_date must not carry a latest release, got %+v", st.Latest)
	}
	if len(st.Changes) != 0 {
		t.Fatalf("up_to_date must not carry a changelog, got %+v", st.Changes)
	}
	if st.CurrentVersion != "0.2.0" {
		t.Fatalf("unexpected current version %q", st.CurrentVersion)
	}
}

func TestUpdateFlowDownloadRejectedWhenUpToDate(t *testing.T) {
	_, flow := newFlowTestServer(t)
	flow.currentVersion = "0.2.0"
	if err := flow.check(context.Background()); err != nil {
		t.Fatal(err)
	}

	a := newAuthTestApp(t)
	s := New(a)
	s.updates = flow

	if err := s.startUpdateDownload(context.Background()); err == nil {
		t.Fatal("expected download to be rejected without a newer release")
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

func TestInstallDispatchesDurableJobAndReportsLaunchFailure(t *testing.T) {
	for _, launchErr := range []error{nil, updater.ErrUpdateBusy, errors.New("worker unavailable")} {
		a := newAuthTestApp(t)
		s := New(a)
		s.updates.st.Phase = updateReady
		s.updates.st.ArchiveName = "release-test.tar.gz"
		s.updates.st.Digest = "sha256:expected"
		called := false
		s.startUpdateJob = func(_ context.Context, cfg config.Config, req updater.Request) (updater.Job, error) {
			called = true
			if req.Archive != filepath.Join(s.updates.archiveDir, "release-test.tar.gz") || req.Digest != "sha256:expected" || !req.AutoRollback {
				t.Fatalf("lost request: %+v", req)
			}
			return updater.Job{ID: "test", Phase: "queued"}, launchErr
		}
		rec := httptest.NewRecorder()
		s.startUpdateInstall(rec, httptest.NewRequest(http.MethodPost, "/api/system/update/install", nil))
		want := http.StatusAccepted
		if errors.Is(launchErr, updater.ErrUpdateBusy) {
			want = http.StatusConflict
		} else if launchErr != nil {
			want = http.StatusInternalServerError
		}
		if !called || rec.Code != want {
			t.Fatalf("dispatch called=%t code=%d want=%d body=%s", called, rec.Code, want, rec.Body.String())
		}
	}
}

func TestURLUpdateForwardsExplicitInsecureOptIn(t *testing.T) {
	t.Setenv("CAMERA_APPLIANCE_ALLOW_INSECURE_UPDATE", "1")
	s := New(newAuthTestApp(t))
	called := false
	s.startUpdateJob = func(_ context.Context, _ config.Config, req updater.Request) (updater.Job, error) {
		called = true
		if !req.AllowInsecureURL || req.URL != "http://localhost/release.tar.gz" {
			t.Fatalf("lost development opt-in: %+v", req)
		}
		return updater.Job{ID: "test", Phase: "queued"}, nil
	}
	rec := httptest.NewRecorder()
	s.startUpdate(rec, httptest.NewRequest(http.MethodPost, "/api/system/update", strings.NewReader(`{"url":"http://localhost/release.tar.gz"}`)))
	if !called || rec.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFreshServerReadsDurableJobWithoutExposingPrivateInputs(t *testing.T) {
	a := newAuthTestApp(t)
	dir := filepath.Join(a.Config.StateDir, "updates")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	persisted := map[string]any{"id": "old-process", "phase": "failed", "error": "healthcheck failed", "updated_at": time.Now().UTC(), "request": map[string]string{"url": "https://example.com/private-token"}, "config": map[string]string{"ConfigDir": "/private/config"}}
	data, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "job.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(a)
	rec := httptest.NewRecorder()
	s.getUpdateStatus(rec, httptest.NewRequest(http.MethodGet, "/api/system/update/status", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "healthcheck failed") || !strings.Contains(body, "old-process") {
		t.Fatalf("lost result: %s", body)
	}
	if strings.Contains(body, "private-token") || strings.Contains(body, "/private/config") {
		t.Fatalf("private job inputs exposed: %s", body)
	}
}

func TestPublicHealthIdentifiesRunningRelease(t *testing.T) {
	s := New(newAuthTestApp(t))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	current := version.Current()
	if rec.Code != http.StatusOK || body["status"] != "ok" || body["version"] != current.Version || body["commit"] != current.Commit {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}
}
