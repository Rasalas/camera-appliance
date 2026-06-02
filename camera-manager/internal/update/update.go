package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/backup"
	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/system"
	"camera-appliance/camera-manager/internal/version"
)

const (
	DefaultInstallDir = "/opt/camera-appliance"
	lastUpdateFile    = "update-last.json"
)

type Manifest struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

type Result struct {
	InstallDir      string       `json:"install_dir"`
	BackupPath      string       `json:"backup_path,omitempty"`
	RollbackDir     string       `json:"rollback_dir,omitempty"`
	OldVersion      version.Info `json:"old_version"`
	NewVersion      Manifest     `json:"new_version"`
	AppliedFiles    []string     `json:"applied_files,omitempty"`
	RollbackApplied bool         `json:"rollback_applied"`
	Warning         string       `json:"warning,omitempty"`
}

type Options struct {
	Config         config.Config
	Archive        string
	URL            string
	InstallDir     string
	NoRestart      bool
	AutoRollback   bool
	Restart        func(context.Context) error
	Healthcheck    func(context.Context) error
	HTTPClient     *http.Client
	Now            func() time.Time
	BackupOverride string
}

type RollbackOptions struct {
	Config      config.Config
	InstallDir  string
	NoRestart   bool
	Restart     func(context.Context) error
	Healthcheck func(context.Context) error
}

type lastUpdate struct {
	InstallDir  string       `json:"install_dir"`
	BackupPath  string       `json:"backup_path"`
	RollbackDir string       `json:"rollback_dir"`
	AppliedAt   string       `json:"applied_at"`
	OldVersion  version.Info `json:"old_version"`
	NewVersion  Manifest     `json:"new_version"`
}

func Apply(ctx context.Context, opts Options) (Result, error) {
	if err := validateSource(opts.Archive, opts.URL); err != nil {
		return Result{}, err
	}
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
	backupResult, err := backup.Create(ctx, opts.Config, opts.BackupOverride, false)
	if err != nil {
		return result, fmt.Errorf("pre-update backup failed: %w", err)
	}
	result.BackupPath = backupResult.Path

	archivePath := opts.Archive
	cleanupArchive := func() {}
	if opts.URL != "" {
		archivePath, cleanupArchive, err = downloadArchive(ctx, opts)
		if err != nil {
			return result, err
		}
		defer cleanupArchive()
	}

	stageDir, err := os.MkdirTemp(opts.Config.BackupDir(), "update-release-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stageDir)
	if err := extractReleaseArchive(ctx, archivePath, stageDir); err != nil {
		return result, err
	}
	releaseRoot, manifest, err := findReleaseRoot(stageDir)
	if err != nil {
		return result, err
	}
	result.NewVersion = manifest

	rollbackDir := filepath.Join(opts.Config.BackupDir(), "rollback-"+now().UTC().Format("20060102-150405"))
	if err := snapshotInstall(ctx, installDir, rollbackDir); err != nil {
		return result, fmt.Errorf("rollback snapshot failed: %w", err)
	}
	result.RollbackDir = rollbackDir

	applied, err := applyRelease(ctx, releaseRoot, installDir)
	if err != nil {
		return result, err
	}
	result.AppliedFiles = applied
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

	if err := restartAndCheck(ctx, opts.NoRestart, opts.Restart, opts.Healthcheck); err != nil {
		if opts.AutoRollback {
			rollbackErr := restoreRollback(ctx, rollbackDir, installDir)
			if rollbackErr == nil {
				result.RollbackApplied = true
				rollbackErr = restartAndCheck(ctx, opts.NoRestart, opts.Restart, opts.Healthcheck)
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
		OldVersion:      last.NewVersion.asVersionInfo(),
		NewVersion:      manifestFromVersion(last.OldVersion),
		RollbackApplied: true,
		Warning:         "Rollback aus letztem Update-Snapshot wiederhergestellt.",
	}
	if err := restoreRollback(ctx, last.RollbackDir, installDir); err != nil {
		return result, err
	}
	if err := restartAndCheck(ctx, opts.NoRestart, opts.Restart, opts.Healthcheck); err != nil {
		return result, err
	}
	return result, nil
}

func HTTPHealthcheck(cfg config.Config) func(context.Context) error {
	return func(ctx context.Context) error {
		managerBase := managerBaseURL(cfg.BindAddr)
		checks := []struct {
			name string
			url  string
		}{
			{name: "manager", url: strings.TrimRight(managerBase, "/") + "/api/status"},
			{name: "go2rtc", url: cfg.Go2RTCURL},
			{name: "viewer", url: strings.TrimRight(managerBase, "/") + "/api/viewer"},
		}
		var viewerBody []byte
		for _, check := range checks {
			body, err := waitHTTP(ctx, check.url, 30*time.Second)
			if err != nil {
				return fmt.Errorf("%s healthcheck failed: %w", check.name, err)
			}
			if check.name == "viewer" {
				viewerBody = body
			}
		}
		var viewer struct {
			Slots []json.RawMessage `json:"slots"`
		}
		if err := json.Unmarshal(viewerBody, &viewer); err != nil {
			return fmt.Errorf("viewer healthcheck returned invalid JSON: %w", err)
		}
		if len(viewer.Slots) < len(config.DefaultSlots()) {
			return fmt.Errorf("viewer healthcheck saw %d slots, expected at least %d", len(viewer.Slots), len(config.DefaultSlots()))
		}
		return nil
	}
}

func validateSource(archivePath, rawURL string) error {
	if (archivePath == "") == (rawURL == "") {
		return errors.New("exactly one of --archive or --url is required")
	}
	if rawURL != "" {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid update URL %q", rawURL)
		}
	}
	return nil
}

func cleanInstallDir(value string) (string, error) {
	if value == "" {
		value = DefaultInstallDir
	}
	clean, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", err
	}
	if clean == string(filepath.Separator) {
		return "", errors.New("refusing to use filesystem root as install dir")
	}
	return clean, nil
}

func downloadArchive(ctx context.Context, opts Options) (string, func(), error) {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("update download failed: %s", resp.Status)
	}
	file, err := os.CreateTemp("", "camera-appliance-update-*.tar.gz")
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 512*1024*1024)); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func extractReleaseArchive(ctx context.Context, archivePath, dst string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := cleanArchiveName(header.Name)
		if err != nil {
			return err
		}
		if forbiddenArchivePath(name) {
			return fmt.Errorf("release archive contains forbidden path %q", name)
		}
		target := filepath.Join(dst, name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(filepath.Separator)) {
			return fmt.Errorf("release archive path escapes destination: %q", header.Name)
		}
		mode := header.FileInfo().Mode()
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode.Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func cleanArchiveName(name string) (string, error) {
	clean := filepath.Clean(strings.TrimLeft(name, string(filepath.Separator)))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid release archive path %q", name)
	}
	return clean, nil
}

func forbiddenArchivePath(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		lower := strings.ToLower(part)
		if lower == ".ds_store" || strings.HasPrefix(lower, "._") {
			return true
		}
		switch lower {
		case ".git", ".private", "data", "node_modules", "secrets.env", "local.env", ".env":
			return true
		}
	}
	return false
}

func findReleaseRoot(stageDir string) (string, Manifest, error) {
	if validReleaseRoot(stageDir) {
		manifest, err := readManifest(stageDir)
		return stageDir, manifest, err
	}
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		return "", Manifest{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(stageDir, entry.Name())
		if validReleaseRoot(candidate) {
			manifest, err := readManifest(candidate)
			return candidate, manifest, err
		}
	}
	return "", Manifest{}, errors.New("release archive must contain bin/camera-appliance")
}

func validReleaseRoot(root string) bool {
	info, err := os.Stat(filepath.Join(root, "bin", "camera-appliance"))
	return err == nil && !info.IsDir()
}

func readManifest(root string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Manifest{Version: "unknown", Commit: "unknown"}, nil
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Version == "" {
		manifest.Version = "unknown"
	}
	if manifest.Commit == "" {
		manifest.Commit = "unknown"
	}
	return manifest, nil
}

func snapshotInstall(ctx context.Context, installDir, rollbackDir string) error {
	info, err := os.Stat(installDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("install dir is not a directory: %s", installDir)
	}
	return copyTree(ctx, installDir, rollbackDir, copyOptions{ExcludeGenerated: true})
}

func applyRelease(ctx context.Context, releaseRoot, installDir string) ([]string, error) {
	if pathExists(filepath.Join(releaseRoot, "frontend", "dist")) {
		if err := os.RemoveAll(filepath.Join(installDir, "frontend", "dist")); err != nil {
			return nil, err
		}
	}
	var files []string
	err := copyTree(ctx, releaseRoot, installDir, copyOptions{
		ExcludeGenerated: true,
		OnFile: func(path string) {
			files = append(files, path)
		},
	})
	if err != nil {
		return nil, err
	}
	binary := filepath.Join(installDir, "bin", "camera-appliance")
	if pathExists(binary) {
		if err := os.Chmod(binary, 0o755); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func restoreRollback(ctx context.Context, rollbackDir, installDir string) error {
	if rollbackDir == "" {
		return errors.New("rollback dir is empty")
	}
	if !pathExists(rollbackDir) {
		return fmt.Errorf("rollback dir not found: %s", rollbackDir)
	}
	if pathExists(filepath.Join(rollbackDir, "frontend", "dist")) {
		if err := os.RemoveAll(filepath.Join(installDir, "frontend", "dist")); err != nil {
			return err
		}
	}
	return copyTree(ctx, rollbackDir, installDir, copyOptions{ExcludeGenerated: true})
}

type copyOptions struct {
	ExcludeGenerated bool
	OnFile           func(string)
}

func copyTree(ctx context.Context, src, dst string, opts copyOptions) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		if opts.ExcludeGenerated && shouldSkipCopyPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		if opts.OnFile != nil {
			opts.OnFile(filepath.ToSlash(rel))
		}
		return nil
	})
}

func shouldSkipCopyPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		lower := strings.ToLower(part)
		if lower == ".ds_store" || strings.HasPrefix(lower, "._") {
			return true
		}
		switch lower {
		case ".git", ".private", "data", "node_modules", ".release":
			return true
		}
	}
	return false
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func restartAndCheck(ctx context.Context, noRestart bool, restart func(context.Context) error, healthcheck func(context.Context) error) error {
	if !noRestart {
		if restart != nil {
			if err := restart(ctx); err != nil {
				return err
			}
		}
	}
	if healthcheck != nil {
		return healthcheck(ctx)
	}
	return nil
}

func writeLastUpdate(cfg config.Config, last lastUpdate) error {
	if err := os.MkdirAll(cfg.BackupDir(), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(last, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfg.BackupDir(), lastUpdateFile), append(data, '\n'), 0o600)
}

func readLastUpdate(cfg config.Config) (lastUpdate, error) {
	data, err := os.ReadFile(filepath.Join(cfg.BackupDir(), lastUpdateFile))
	if err != nil {
		return lastUpdate{}, err
	}
	var last lastUpdate
	if err := json.Unmarshal(data, &last); err != nil {
		return lastUpdate{}, err
	}
	return last, nil
}

func managerBaseURL(bindAddr string) string {
	if strings.HasPrefix(bindAddr, "http://") || strings.HasPrefix(bindAddr, "https://") {
		return bindAddr
	}
	return "http://" + bindAddr
}

func waitHTTP(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
				_ = resp.Body.Close()
				if readErr == nil && resp.StatusCode < 500 {
					cancel()
					return body, nil
				}
				if readErr != nil {
					lastErr = readErr
				} else {
					lastErr = fmt.Errorf("%s", resp.Status)
				}
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
		cancel()
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("timeout")
			}
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func manifestFromVersion(info version.Info) Manifest {
	return Manifest{Version: info.Version, Commit: info.Commit, BuildTime: info.BuildTime}
}

func (m Manifest) asVersionInfo() version.Info {
	return version.Info{Version: m.Version, Commit: m.Commit, BuildTime: m.BuildTime}
}

func StackRestart(cfg config.Config) func(context.Context) error {
	return func(ctx context.Context) error {
		return system.ApplyStack(ctx, cfg)
	}
}
