package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/redaction"
	"camera-appliance/camera-manager/internal/releasearchive"
	"camera-appliance/camera-manager/internal/version"
)

const (
	DefaultInstallDir = "/opt/camera-appliance"
	DefaultReleaseURL = "https://github.com/Rasalas/camera-appliance/releases/latest/download/camera-appliance-latest.tar.gz"
	lastUpdateFile    = "update-last.json"
	// Recovery may follow an expired update deadline and, without a preserved
	// image, rebuild the old sources on a slow laptop.
	recoveryTimeout = 10 * time.Minute
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
	// Free space from old snapshots before measuring it; keep the one manual rollback uses.
	if last, err := readLastUpdate(opts.Config); err == nil || errors.Is(err, os.ErrNotExist) {
		_, _ = backup.Prune(opts.Config.BackupDir(), backup.RollbackSnapshots, backup.KeepRollbackSnapshots, last.RollbackDir)
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
	var buildRoot string
	if !opts.NoRestart && opts.Stack.DockerRoot != nil {
		buildRoot, _ = opts.Stack.DockerRoot(ctx)
	}
	if err := checkUpdateSpace(installDir, opts.Config.BackupDir(), releaseRoot, buildRoot); err != nil {
		return result, err
	}

	rollbackDir := filepath.Join(opts.Config.BackupDir(), "rollback-"+now().UTC().Format("20060102-150405.000000000"))
	if err := snapshotInstall(ctx, installDir, rollbackDir); err != nil {
		_ = os.RemoveAll(rollbackDir)
		return result, fmt.Errorf("rollback snapshot failed: %w", err)
	}
	result.RollbackDir = rollbackDir
	// Name order alone could drop the current snapshot after a clock reset.
	defer func() {
		_, _ = backup.Prune(opts.Config.BackupDir(), backup.RollbackSnapshots, backup.KeepRollbackSnapshots, rollbackDir)
	}()

	var previousImage string
	if !opts.NoRestart && opts.Stack.Preserve != nil {
		if ref, err := opts.Stack.Preserve(ctx); err != nil {
			result.Warning += " Vorheriges Image konnte nicht gesichert werden; ein Rollback baut die alte Version neu: " + redaction.Text(err.Error())
		} else {
			previousImage = ref
		}
	}

	if err := writeLastUpdate(opts.Config, lastUpdate{
		InstallDir:    installDir,
		BackupPath:    result.BackupPath,
		RollbackDir:   rollbackDir,
		AppliedAt:     now().UTC().Format(time.RFC3339),
		OldVersion:    result.OldVersion,
		NewVersion:    result.NewVersion,
		PreviousImage: previousImage,
	}); err != nil {
		return result, err
	}

	staged, err := stageTrees(ctx, releaseRoot, installDir)
	if err != nil {
		return result, fmt.Errorf("Update-Dateien konnten nicht vorbereitet werden, die Installation wurde nicht verändert: %w", err)
	}
	defer staged.discard()
	// The running backend serves frontend/dist from the install dir. Swap it
	// only after the new image is built.
	buildFirst := !opts.NoRestart && opts.Stack.Build != nil
	var keepStaged []string
	if buildFirst {
		keepStaged = []string{"frontend/dist"}
	}
	applied, err := overlayRelease(ctx, releaseRoot, installDir)
	if err == nil {
		err = staged.swap(keepStaged...)
	}
	if err != nil {
		staged.discard()
		// The install dir may now hold a mix of old and new files. Restore the
		// snapshot immediately instead of waiting for a human to run rollback.
		recoveryCtx, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
		defer cancel()
		restoreErr := restoreRollback(recoveryCtx, rollbackDir, installDir)
		if restoreErr != nil {
			return result, fmt.Errorf("update failed: %w; restoring rollback snapshot also failed: %v", err, restoreErr)
		}
		result.RollbackApplied = true
		if restartErr := restartRestored(recoveryCtx, opts.NoRestart, opts.Stack, previousImage, opts.Restart, oldHealthcheck); restartErr != nil {
			result.Warning += " Wiederhergestellte Installation startete nicht sauber: " + restartErr.Error()
		}
		return result, fmt.Errorf("update failed, previous installation was restored: %w", err)
	}
	result.AppliedFiles = append(staged.files, applied...)
	if err := ensureCommandLink(installDir); err != nil {
		result.Warning += " CLI-Link konnte nicht erstellt werden: " + err.Error()
	}

	if buildFirst {
		err = opts.Stack.Build(ctx)
		if err == nil {
			err = staged.swap()
		}
	}
	if err == nil {
		err = restartAndCheck(ctx, opts.NoRestart, opts.Restart, newHealthcheck)
	}
	if err != nil {
		staged.discard()
		if opts.AutoRollback {
			// Recovery needs its own deadline when the update timed out.
			recoveryCtx, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
			defer cancel()
			rollbackErr := restoreRollback(recoveryCtx, rollbackDir, installDir)
			if rollbackErr == nil {
				result.RollbackApplied = true
				rollbackErr = restartRestored(recoveryCtx, opts.NoRestart, opts.Stack, previousImage, opts.Restart, oldHealthcheck)
			}
			if rollbackErr != nil {
				return result, fmt.Errorf("update failed: %w; rollback also failed: %v", err, rollbackErr)
			}
			return result, fmt.Errorf("update healthcheck failed and rollback was applied: %w", err)
		}
		return result, err
	}
	result.Warning += joinNotes(refreshInstalledUnits(ctx, installDir))
	if !opts.NoRestart && opts.Stack.PruneImages != nil {
		if err := opts.Stack.PruneImages(ctx); err != nil {
			result.Warning += " Alte Images konnten nicht entfernt werden: " + redaction.Text(err.Error())
		}
	}
	return result, nil
}

// restartRestored prefers the preserved image so recovery does not depend on
// a rebuild, and falls back to Restart if that image fails or is missing.
func restartRestored(ctx context.Context, noRestart bool, stack StackOps, image string, restart, healthcheck func(context.Context) error) error {
	if noRestart || stack.RestorePrevious == nil || image == "" {
		return restartAndCheck(ctx, noRestart, restart, healthcheck)
	}
	err := stack.RestorePrevious(ctx, image)
	if err == nil && healthcheck != nil {
		err = healthcheck(ctx)
	}
	if err == nil {
		return nil
	}
	if rebuildErr := restartAndCheck(ctx, false, restart, healthcheck); rebuildErr != nil {
		return fmt.Errorf("previous image: %v; rebuild: %w", err, rebuildErr)
	}
	return nil
}

// checkUpdateSpace reserves staging, the rollback snapshot and, when buildRoot
// is known, the image build on Docker's filesystem (often the same disk).
func checkUpdateSpace(installDir, backupDir, releaseRoot, buildRoot string) error {
	var needs []spaceNeed
	release, releaseErr := treeSize(releaseRoot)
	snapshot, snapshotErr := treeSize(installDir)
	if releaseErr == nil && snapshotErr == nil {
		needs = append(needs, spaceNeed{path: installDir, bytes: release}, spaceNeed{path: backupDir, bytes: snapshot})
	}
	if buildRoot != "" {
		needs = append(needs, spaceNeed{path: buildRoot, bytes: dockerBuildSpace, build: true})
	}
	return checkFreeSpace(updateSpaceMargin, needs...)
}

func joinNotes(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	return " " + strings.Join(notes, " ")
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
	if size, err := treeSize(last.RollbackDir); err == nil {
		if err := checkFreeSpace(rollbackSpaceMargin, spaceNeed{path: installDir, bytes: size}); err != nil {
			return result, err
		}
	}
	if err := restoreRollback(ctx, last.RollbackDir, installDir); err != nil {
		return result, err
	}
	result.Warning += joinNotes(refreshInstalledUnits(ctx, installDir))
	if !opts.NoRestart && opts.Healthcheck == nil {
		opts.Healthcheck = HTTPVersionHealthcheck(opts.Config, result.NewVersion)
	}
	if err := restartRestored(ctx, opts.NoRestart, opts.Stack, last.PreviousImage, opts.Restart, opts.Healthcheck); err != nil {
		return result, err
	}
	return result, nil
}
