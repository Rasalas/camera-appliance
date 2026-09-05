package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/releasearchive"
	"camera-appliance/camera-manager/internal/state"
	"camera-appliance/camera-manager/internal/system"
)

var ErrUpdateBusy = errors.New("ein anderes Update läuft bereits, bitte warten")

// Request contains only serializable update inputs. Secrets are not part of Config.
type Request struct {
	Archive          string `json:"archive,omitempty"`
	URL              string `json:"url,omitempty"`
	Digest           string `json:"digest,omitempty"`
	NoRestart        bool   `json:"no_restart"`
	AutoRollback     bool   `json:"auto_rollback"`
	AllowInsecureURL bool   `json:"allow_insecure_url"`
	Rollback         bool   `json:"rollback"`
}

type Job struct {
	ID        string    `json:"id"`
	Phase     string    `json:"phase"`
	UpdatedAt time.Time `json:"updated_at"`
	Result    Result    `json:"result"`
	Error     string    `json:"error,omitempty"`
}

type jobFile struct {
	Job
	Config  config.Config `json:"config"`
	Request Request       `json:"request"`
}

type workerLauncher func(context.Context, config.Config, string, string, string) error

// StartJob durably reserves the update slot before handing it to a supervisor
// outside the manager. Cancellation of the submitting request cannot stop it.
func StartJob(ctx context.Context, cfg config.Config, req Request) (Job, error) {
	executable, err := os.Executable()
	if err != nil {
		return Job{}, err
	}
	return startJob(ctx, cfg, req, executable, system.StartUpdateWorker)
}

func startJob(ctx context.Context, cfg config.Config, req Request, executable string, launch workerLauncher) (Job, error) {
	if !req.Rollback {
		if err := (releasearchive.Source{Archive: req.Archive, URL: req.URL, AllowInsecureURL: req.AllowInsecureURL}).Validate(); err != nil {
			return Job{}, err
		}
	}
	var err error
	if req.Rollback && cfg.InstallDir == "" {
		last, readErr := readLastUpdate(cfg)
		if readErr != nil {
			return Job{}, readErr
		}
		cfg.InstallDir = last.InstallDir
	}
	cfg.InstallDir, err = cleanInstallDir(cfg.InstallDir)
	if err != nil {
		return Job{}, err
	}
	for _, path := range []*string{&cfg.StateDir, &cfg.ConfigDir, &cfg.ComposeFile} {
		if *path == "" {
			return Job{}, errors.New("update requires explicit state, config and compose paths")
		}
		*path, err = filepath.Abs(*path)
		if err != nil {
			return Job{}, err
		}
	}
	release, err := lockIdle(cfg)
	if err != nil {
		return Job{}, err
	}
	defer release()
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	worker := filepath.Join(cfg.StateDir, "updates", "worker-"+id)
	if err := copyFile(executable, worker, 0o700); err != nil {
		return Job{}, err
	}
	// Local CLI archives may not be mounted into a Docker worker. Stage them on
	// the shared state volume, keeping the user's original archive untouched.
	if req.Archive != "" {
		staged := filepath.Join(cfg.StateDir, "updates", "release-"+id+".tar.gz")
		if err := copyFile(req.Archive, staged, 0o600); err != nil {
			_ = os.Remove(worker)
			return Job{}, err
		}
		req.Archive = staged
	}
	f := jobFile{Job: Job{ID: id, Phase: "queued", UpdatedAt: time.Now().UTC()}, Config: cfg, Request: req}
	path := jobPath(cfg)
	if err := writeJSONAtomic(path, f); err != nil {
		cleanupJob(f)
		return Job{}, err
	}
	if err := launch(ctx, cfg, worker, path, id); err != nil {
		f.Phase, f.Error = "failed", redaction.Text(err.Error())
		f.UpdatedAt = time.Now().UTC()
		persistErr := writeJSONAtomic(path, f)
		cleanupJob(f)
		return f.Job, errors.Join(err, persistErr)
	}
	return f.Job, nil
}

// RunJob is invoked only by the hidden update-worker command. The OS lock is
// held until the final result is durable, including restart and rollback.
func RunJob(ctx context.Context, path, expectedID string) error {
	f, err := readJobFile(path)
	if err != nil {
		return err
	}
	if f.ID != expectedID {
		return errors.New("update job was replaced before worker launch")
	}
	var release func()
	for {
		release, err = lockUpdate(f.Config)
		if !errors.Is(err, ErrUpdateBusy) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err != nil {
		return err
	}
	defer release()
	current, err := readJobFile(path)
	if err != nil {
		return err
	}
	if current.ID != expectedID || current.Phase != "queued" || time.Since(current.UpdatedAt) > time.Minute {
		return errors.New("update job is no longer queued")
	}
	f.Phase, f.UpdatedAt = "installing", time.Now().UTC()
	if err := writeJSONAtomic(path, f); err != nil {
		return err
	}
	defer cleanupJob(f)
	restart := func(ctx context.Context) error { return system.ApplyStackAndWait(ctx, f.Config) }
	if f.Request.Rollback {
		f.Result, err = rollback(ctx, RollbackOptions{Config: f.Config, InstallDir: f.Config.InstallDir, NoRestart: f.Request.NoRestart, Restart: restart})
	} else {
		f.Result, err = apply(ctx, Options{Config: f.Config, InstallDir: f.Config.InstallDir,
			Archive: f.Request.Archive, URL: f.Request.URL, Digest: f.Request.Digest,
			NoRestart: f.Request.NoRestart, AutoRollback: f.Request.AutoRollback,
			AllowInsecureURL: f.Request.AllowInsecureURL, Restart: restart})
	}
	f.Phase, f.UpdatedAt = "complete", time.Now().UTC()
	if err != nil {
		f.Phase, f.Error = "failed", redaction.Text(err.Error())
	}
	persistErr := writeJSONAtomic(path, f)
	// A fresh connection also works after the manager has reopened/migrated DB.
	if store, openErr := state.Open(context.Background(), f.Config.DBPath()); openErr == nil {
		level, message := "info", "Update installiert"
		if err != nil {
			level, message = "error", f.Error
		}
		_ = store.AddEvent(context.Background(), level, "update."+f.Phase, message, f.Result)
		_ = store.Close()
	}
	return errors.Join(err, persistErr)
}

func cleanupJob(f jobFile) {
	_ = os.Remove(filepath.Join(f.Config.StateDir, "updates", "worker-"+f.ID))
	if f.Request.Archive != "" && filepath.Dir(f.Request.Archive) == filepath.Join(f.Config.StateDir, "updates") && strings.HasPrefix(filepath.Base(f.Request.Archive), "release-") {
		_ = os.Remove(f.Request.Archive)
	}
}

// ReadJob survives manager restarts. An abandoned OS lock reveals interrupted
// workers instead of leaving the UI stuck on "installing" indefinitely.
func ReadJob(cfg config.Config) (Job, error) {
	f, err := readJobFile(jobPath(cfg))
	if err != nil {
		return Job{}, err
	}
	if f.Phase == "queued" || f.Phase == "installing" {
		release, lockErr := lockUpdate(cfg)
		if lockErr == nil {
			defer release()
			// Re-read under the lock: the worker may have finished in the meantime.
			f, err = readJobFile(jobPath(cfg))
			if err != nil {
				return Job{}, err
			}
			if f.Phase == "installing" || (f.Phase == "queued" && time.Since(f.UpdatedAt) > time.Minute) {
				f.Phase, f.Error = "failed", "Update-Prozess wurde unterbrochen; Installation prüfen oder Rollback ausführen."
			}
		} else if !errors.Is(lockErr, ErrUpdateBusy) {
			return Job{}, lockErr
		}
	}
	return f.Job, nil
}

func WaitJob(ctx context.Context, cfg config.Config, id string) (Result, error) {
	for {
		job, err := ReadJob(cfg)
		if err != nil {
			return Result{}, err
		}
		if job.ID != id {
			return Result{}, errors.New("update job was replaced; inspect update status")
		}
		switch job.Phase {
		case "complete":
			return job.Result, nil
		case "failed":
			return job.Result, errors.New(job.Error)
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func jobPath(cfg config.Config) string { return filepath.Join(cfg.StateDir, "updates", "job.json") }

func readJobFile(path string) (jobFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return jobFile{}, err
	}
	var f jobFile
	err = json.Unmarshal(data, &f)
	return f, err
}

func lockIdle(cfg config.Config) (func(), error) {
	release, err := lockUpdate(cfg)
	if err != nil {
		return nil, err
	}
	f, err := readJobFile(jobPath(cfg))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		release()
		return nil, err
	}
	if err == nil && f.Phase == "queued" && time.Since(f.UpdatedAt) < time.Minute {
		release()
		return nil, ErrUpdateBusy
	}
	return release, nil
}

func lockUpdate(cfg config.Config) (func(), error) {
	dir := filepath.Join(cfg.StateDir, "updates")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "update.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrUpdateBusy
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
