package update

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/releasearchive"
	"camera-appliance/camera-manager/internal/version"
)

const (
	DefaultInstallDir = "/opt/camera-appliance"
	DefaultReleaseURL = "https://github.com/Rasalas/camera-appliance/releases/latest/download/camera-appliance-latest.tar.gz"
	lastUpdateFile    = "update-last.json"
)

func Apply(ctx context.Context, opts Options) (Result, error) {
	if err := (releasearchive.Source{Archive: opts.Archive, URL: opts.URL, AllowInsecureURL: opts.AllowInsecureURL}).Validate(); err != nil {
		return Result{}, err
	}
	release, err := EnsureSingleFlight()
	if err != nil {
		return Result{}, err
	}
	defer release()
	unlock, err := lockIdle(opts.Config)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	return apply(ctx, opts)
}

func apply(ctx context.Context, opts Options) (Result, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	installDir, err := cleanInstallDir(opts.InstallDir)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		InstallDir: installDir,
		OldVersion: version.Current(),
		Warning:    "Backup und Rollback-Snapshot wurden erstellt. Release-Archive dürfen keine Secrets enthalten.",
	}
	if previous, err := releasearchive.ReadManifest(installDir); err == nil && previous.Version != "unknown" {
		result.OldVersion = manifestAsVersionInfo(previous)
	}
	backupResult, err := backup.Create(ctx, opts.Config, opts.BackupOverride, false)
	if err != nil {
		return result, fmt.Errorf("pre-update backup failed: %w", err)
	}
	result.BackupPath = backupResult.Path

	prepared, err := releasearchive.Prepare(ctx, releasearchive.Source{Archive: opts.Archive, URL: opts.URL, Digest: opts.Digest, AllowInsecureURL: opts.AllowInsecureURL}, opts.Config.BackupDir(), opts.HTTPClient)
	if err != nil {
		return result, err
	}
	defer prepared.Close()
	releaseRoot, manifest := prepared.Root, prepared.Manifest
	if !opts.NoRestart && (manifest.Version == "unknown" || manifest.Commit == "unknown") {
		return result, errors.New("release manifest must identify version and commit before restarting")
	}
	result.NewVersion = manifest
	newHealthcheck := opts.Healthcheck
	oldHealthcheck := opts.Healthcheck
	if !opts.NoRestart && opts.Healthcheck == nil {
		newHealthcheck = HTTPVersionHealthcheck(opts.Config, manifest)
		oldHealthcheck = HTTPVersionHealthcheck(opts.Config, manifestFromVersion(result.OldVersion))
	}

	rollbackDir := filepath.Join(opts.Config.BackupDir(), "rollback-"+now().UTC().Format("20060102-150405.000000000"))
	if err := snapshotInstall(ctx, installDir, rollbackDir); err != nil {
		return result, fmt.Errorf("rollback snapshot failed: %w", err)
	}
	result.RollbackDir = rollbackDir

	if err := writeLastUpdate(opts.Config, lastUpdate{
		InstallDir:  installDir,
		BackupPath:  result.BackupPath,
		RollbackDir: rollbackDir,
		AppliedAt:   now().UTC().Format(time.RFC3339),
		OldVersion:  result.OldVersion,
		NewVersion:  result.NewVersion,
	}); err != nil {
		return result, err
	}

	applied, err := applyRelease(ctx, releaseRoot, installDir)
	if err != nil {
		// The install dir may now hold a mix of old and new files. Restore the
		// snapshot immediately instead of waiting for a human to run rollback.
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		restoreErr := restoreRollback(recoveryCtx, rollbackDir, installDir)
		if restoreErr != nil {
			return result, fmt.Errorf("update failed: %w; restoring rollback snapshot also failed: %v", err, restoreErr)
		}
		result.RollbackApplied = true
		if restartErr := restartAndCheck(recoveryCtx, opts.NoRestart, opts.Restart, oldHealthcheck); restartErr != nil {
			result.Warning += " Wiederhergestellte Installation startete nicht sauber: " + restartErr.Error()
		}
		return result, fmt.Errorf("update failed, previous installation was restored: %w", err)
	}
	result.AppliedFiles = applied
	if err := ensureCommandLink(installDir); err != nil {
		result.Warning += " CLI-Link konnte nicht erstellt werden: " + err.Error()
	}

	if err := restartAndCheck(ctx, opts.NoRestart, opts.Restart, newHealthcheck); err != nil {
		if opts.AutoRollback {
			// Recovery needs its own deadline when the update timed out.
			recoveryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rollbackErr := restoreRollback(recoveryCtx, rollbackDir, installDir)
			if rollbackErr == nil {
				result.RollbackApplied = true
				rollbackErr = restartAndCheck(recoveryCtx, opts.NoRestart, opts.Restart, oldHealthcheck)
			}
			if rollbackErr != nil {
				return result, fmt.Errorf("update failed: %w; rollback also failed: %v", err, rollbackErr)
			}
			return result, fmt.Errorf("update healthcheck failed and rollback was applied: %w", err)
		}
		return result, err
	}
	return result, nil
}

func Rollback(ctx context.Context, opts RollbackOptions) (Result, error) {
	release, err := lockIdle(opts.Config)
	if err != nil {
		return Result{}, err
	}
	defer release()
	return rollback(ctx, opts)
}

func rollback(ctx context.Context, opts RollbackOptions) (Result, error) {
	last, err := readLastUpdate(opts.Config)
	if err != nil {
		return Result{}, err
	}
	installDir := opts.InstallDir
	if installDir == "" {
		installDir = last.InstallDir
	}
	installDir, err = cleanInstallDir(installDir)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		InstallDir:      installDir,
		BackupPath:      last.BackupPath,
		RollbackDir:     last.RollbackDir,
		OldVersion:      manifestAsVersionInfo(last.NewVersion),
		NewVersion:      manifestFromVersion(last.OldVersion),
		RollbackApplied: true,
		Warning:         "Rollback aus letztem Update-Snapshot wiederhergestellt.",
	}
	if err := restoreRollback(ctx, last.RollbackDir, installDir); err != nil {
		return result, err
	}
	if !opts.NoRestart && opts.Healthcheck == nil {
		opts.Healthcheck = HTTPVersionHealthcheck(opts.Config, result.NewVersion)
	}
	if err := restartAndCheck(ctx, opts.NoRestart, opts.Restart, opts.Healthcheck); err != nil {
		return result, err
	}
	return result, nil
}
