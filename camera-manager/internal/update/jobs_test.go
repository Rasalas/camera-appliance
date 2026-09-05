package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/version"
)

func jobFixture(t *testing.T) (config.Config, Request, string) {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.InstallDir = newTestInstall(t, "old")
	cfg.ComposeFile = filepath.Join(cfg.InstallDir, "compose.yaml")
	archive := newReleaseArchive(t, map[string]string{
		"camera-appliance/manifest.json":        `{"version":"2.0.0","commit":"new"}`,
		"camera-appliance/bin/camera-appliance": "new",
	})
	executable := filepath.Join(t.TempDir(), "worker")
	writeFile(t, executable, "test worker")
	return cfg, Request{Archive: archive, NoRestart: true, AutoRollback: true}, executable
}

func TestWorkerProcess(t *testing.T) {
	path := os.Getenv("CAMERA_TEST_UPDATE_JOB")
	if path == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := RunJob(ctx, path, os.Getenv("CAMERA_TEST_UPDATE_ID")); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentJobSurvivesSubmittingContextAndPersistsResult(t *testing.T) {
	cfg, req, executable := jobFixture(t)
	var worker *exec.Cmd
	submitted, cancel := context.WithCancel(context.Background())
	job, err := startJob(submitted, cfg, req, executable, func(_ context.Context, _ config.Config, _ string, path, id string) error {
		worker = exec.Command(os.Args[0], "-test.run=^TestWorkerProcess$")
		worker.Env = append(os.Environ(), "CAMERA_TEST_UPDATE_JOB="+path, "CAMERA_TEST_UPDATE_ID="+id)
		worker.Stdout, worker.Stderr = os.Stdout, os.Stderr
		return worker.Start()
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Wait(); err != nil {
		t.Fatal(err)
	}
	// Reopen through the persisted path, without the submitting server's memory.
	observed, err := ReadJob(config.Config{StateDir: cfg.StateDir})
	if err != nil {
		t.Fatal(err)
	}
	if observed.ID != job.ID || observed.Phase != "complete" || observed.Result.NewVersion.Version != "2.0.0" {
		t.Fatalf("lost worker outcome: %+v", observed)
	}
	if got := readFile(t, filepath.Join(cfg.InstallDir, "bin", "camera-appliance")); got != "new" {
		t.Fatalf("binary = %q", got)
	}
	if _, err := os.Stat(req.Archive); err != nil {
		t.Fatalf("user archive removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.StateDir, "updates", "worker-"+job.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker not cleaned: %v", err)
	}
	info, err := os.Stat(jobPath(cfg))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("job permissions: %v %v", info, err)
	}
}

func TestQueuedJobAndOSLockExcludeOtherWriters(t *testing.T) {
	cfg, req, executable := jobFixture(t)
	launch := func(context.Context, config.Config, string, string, string) error { return nil }
	job, err := startJob(context.Background(), cfg, req, executable, launch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startJob(context.Background(), cfg, req, executable, launch); !errors.Is(err, ErrUpdateBusy) {
		t.Fatalf("queued job not reserved: %v", err)
	}
	if _, err := Apply(context.Background(), Options{Config: cfg, Archive: req.Archive, InstallDir: cfg.InstallDir, NoRestart: true}); !errors.Is(err, ErrUpdateBusy) {
		t.Fatalf("CLI bypassed reservation: %v", err)
	}
	release, err := lockUpdate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockUpdate(cfg); !errors.Is(err, ErrUpdateBusy) {
		t.Fatalf("second file descriptor acquired OS lock: %v", err)
	}
	observed, err := ReadJob(cfg)
	release()
	if err != nil || observed.ID != job.ID || observed.Phase != "queued" {
		t.Fatalf("active queued status: %+v %v", observed, err)
	}
	if err := RunJob(context.Background(), jobPath(cfg), "stale-worker"); err == nil {
		t.Fatal("stale worker accepted a different job")
	}
	if err := RunJob(context.Background(), jobPath(cfg), job.ID); err != nil {
		t.Fatal(err)
	}
	if err := RunJob(context.Background(), jobPath(cfg), job.ID); err == nil {
		t.Fatal("completed job ran again")
	}
}

func TestWorkerLaunchFailureAndInterruptedJobAreVisible(t *testing.T) {
	cfg, req, executable := jobFixture(t)
	_, err := startJob(context.Background(), cfg, req, executable, func(context.Context, config.Config, string, string, string) error {
		return errors.New("systemd unavailable")
	})
	if err == nil {
		t.Fatal("launch failure ignored")
	}
	observed, err := ReadJob(cfg)
	if err != nil || observed.Phase != "failed" || !strings.Contains(observed.Error, "systemd unavailable") {
		t.Fatalf("launch status: %+v %v", observed, err)
	}
	f, err := readJobFile(jobPath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"queued", "installing"} {
		f.Phase, f.UpdatedAt = phase, time.Now().Add(-2*time.Minute)
		if err := writeJSONAtomic(jobPath(cfg), f); err != nil {
			t.Fatal(err)
		}
		observed, err = ReadJob(cfg)
		if err != nil || observed.Phase != "failed" || !strings.Contains(observed.Error, "unterbrochen") {
			t.Fatalf("interruption: %+v %v", observed, err)
		}
	}
}

func TestVersionHealthcheckRejectsHealthyOldRelease(t *testing.T) {
	for _, wrong := range []string{`{"status":"ok","version":"1.0.0","commit":"old"}`, `{"status":"ok","version":"2.0.0","commit":"old"}`, `{"status":"ok"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(wrong)) }))
		cfg := config.Config{BindAddr: server.URL, Go2RTCURL: server.URL}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err := HTTPVersionHealthcheck(cfg, Manifest{Version: "2.0.0", Commit: "new"})(ctx)
		cancel()
		server.Close()
		if err == nil {
			t.Fatalf("accepted old/unknown release %s", wrong)
		}
	}
}

func TestVersionHealthcheckWaitsForNewReleaseWithProtectedViewer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			if requests.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"status":"ok","version":"old"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok","version":"2.0.0","commit":"new"}`))
		case "/api/viewer":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer server.Close()
	cfg := config.Config{BindAddr: server.URL, Go2RTCURL: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := HTTPVersionHealthcheck(cfg, Manifest{Version: "2.0.0", Commit: "new"})(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackStillRestartsAfterUpdateDeadlineExpires(t *testing.T) {
	cfg, req, _ := jobFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	restarts := 0
	result, err := Apply(ctx, Options{Config: cfg, Archive: req.Archive, InstallDir: cfg.InstallDir, AutoRollback: true,
		Restart: func(ctx context.Context) error {
			restarts++
			if restarts == 1 {
				cancel()
				return ctx.Err()
			}
			return ctx.Err()
		},
		Healthcheck: func(ctx context.Context) error { return ctx.Err() },
	})
	defer cancel()
	if err == nil || !result.RollbackApplied || restarts != 2 {
		t.Fatalf("recovery failed: %+v restarts=%d err=%v", result, restarts, err)
	}
	if got := readFile(t, filepath.Join(cfg.InstallDir, "bin", "camera-appliance")); got != "old" {
		t.Fatalf("recovery left %q", got)
	}
	if strings.Contains(err.Error(), "rollback also failed") {
		t.Fatalf("recovery inherited canceled context: %v", err)
	}
}

func TestAutomaticRollbackChecksPreviousVersion(t *testing.T) {
	cfg, req, _ := jobFixture(t)
	current := version.Current()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/viewer" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": current.Version, "commit": current.Commit})
	}))
	defer server.Close()
	cfg.BindAddr, cfg.Go2RTCURL = server.URL, server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restarts := 0
	result, err := Apply(ctx, Options{Config: cfg, Archive: req.Archive, InstallDir: cfg.InstallDir, AutoRollback: true,
		Restart: func(context.Context) error {
			restarts++
			if restarts == 1 {
				time.AfterFunc(300*time.Millisecond, cancel)
			}
			return nil
		},
	})
	if err == nil || !result.RollbackApplied || restarts != 2 {
		t.Fatalf("unexpected result %+v, restarts=%d, err=%v", result, restarts, err)
	}
	if strings.Contains(err.Error(), "rollback also failed") {
		t.Fatalf("rollback checked the new version: %v", err)
	}
}
