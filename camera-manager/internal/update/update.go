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
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
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

type InstallResult struct {
	InstallDir        string   `json:"install_dir"`
	Version           Manifest `json:"version"`
	AppliedFiles      []string `json:"applied_files,omitempty"`
	SecretsCreated    bool     `json:"secrets_created"`
	Go2RTCInitialized bool     `json:"go2rtc_initialized"`
	SystemdEnabled    bool     `json:"systemd_enabled"`
	KioskEnabled      bool     `json:"kiosk_enabled"`
	DesktopInstalled  bool     `json:"desktop_installed"`
	Started           bool     `json:"started"`
	Warnings          []string `json:"warnings,omitempty"`
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

type InstallOptions struct {
	Config                  config.Config
	Archive                 string
	URL                     string
	SourceDir               string
	InstallDir              string
	UserName                string
	EnableSystemd           bool
	EnableKiosk             bool
	InstallDesktopLaunchers bool
	NoStart                 bool
	HTTPClient              *http.Client
	AllowNonRoot            bool
	SkipCommandChecks       bool
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

func Install(ctx context.Context, opts InstallOptions) (InstallResult, error) {
	if !opts.AllowNonRoot && os.Geteuid() != 0 {
		return InstallResult{}, errors.New("install must run as root; use sudo")
	}
	cfg := opts.Config
	if cfg.ConfigDir == "" {
		cfg.ConfigDir = config.DefaultConfigDir
	}
	if cfg.StateDir == "" {
		cfg.StateDir = config.DefaultStateDir
	}
	if cfg.BindAddr == "" {
		cfg.BindAddr = config.DefaultBindAddr
	}
	if cfg.Go2RTCURL == "" {
		cfg.Go2RTCURL = config.DefaultGo2RTCURL
	}
	if cfg.Go2RTCRTSPURL == "" {
		cfg.Go2RTCRTSPURL = config.DefaultGo2RTCRTSP
	}
	if cfg.ComposeFile == "" {
		cfg.ComposeFile = config.DefaultComposeFile
	}

	installDir, err := cleanInstallDir(opts.InstallDir)
	if err != nil {
		return InstallResult{}, err
	}
	releaseRoot, manifest, cleanup, err := installReleaseRoot(ctx, opts)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	if !opts.SkipCommandChecks {
		if err := requireInstallCommands(opts); err != nil {
			return InstallResult{}, err
		}
	}

	result := InstallResult{
		InstallDir: installDir,
		Version:    manifest,
		Warnings: []string{
			"Secrets werden nicht überschrieben. Prüfe /etc/camera-appliance/secrets.env vor Kundeneinsatz.",
			"Firewall-Regeln wurden nicht geändert; der normale Kiosk-Betrieb benötigt keine eingehenden Ports.",
		},
	}
	if err := createRuntimeDirs(cfg, opts.AllowNonRoot); err != nil {
		return result, err
	}
	secretsCreated, err := ensureSecretsFile(releaseRoot, cfg.ConfigDir)
	if err != nil {
		return result, err
	}
	result.SecretsCreated = secretsCreated
	go2rtcInitialized, err := ensureGo2RTCConfig(cfg)
	if err != nil {
		return result, err
	}
	result.Go2RTCInitialized = go2rtcInitialized

	applied, err := applyRelease(ctx, releaseRoot, installDir)
	if err != nil {
		return result, err
	}
	result.AppliedFiles = applied

	if opts.EnableSystemd {
		if err := installSystemd(ctx, installDir, opts.NoStart); err != nil {
			return result, err
		}
		result.SystemdEnabled = true
		result.Started = !opts.NoStart
	} else if !opts.NoStart {
		result.Warnings = append(result.Warnings, "Systemd wurde nicht aktiviert. Starte manuell mit: cd "+installDir+" && sudo docker compose up -d --build")
	}

	if opts.EnableKiosk {
		if opts.UserName == "" {
			return result, errors.New("--enable-kiosk requires --user USER")
		}
		if err := installKiosk(ctx, installDir, opts.UserName, opts.NoStart); err != nil {
			return result, err
		}
		result.KioskEnabled = true
	}
	if opts.InstallDesktopLaunchers {
		if opts.UserName == "" {
			return result, errors.New("--install-desktop-launchers requires --user USER")
		}
		if err := installDesktopLaunchers(installDir, opts.UserName); err != nil {
			return result, err
		}
		result.DesktopInstalled = true
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

func downloadInstallArchive(ctx context.Context, opts InstallOptions) (string, func(), error) {
	return downloadArchive(ctx, Options{URL: opts.URL, HTTPClient: opts.HTTPClient})
}

func installReleaseRoot(ctx context.Context, opts InstallOptions) (string, Manifest, func(), error) {
	if opts.SourceDir != "" {
		root, manifest, err := findReleaseRoot(opts.SourceDir)
		return root, manifest, func() {}, err
	}
	if err := validateSource(opts.Archive, opts.URL); err != nil {
		return "", Manifest{}, func() {}, err
	}
	archivePath := opts.Archive
	cleanupArchive := func() {}
	var err error
	if opts.URL != "" {
		archivePath, cleanupArchive, err = downloadInstallArchive(ctx, opts)
		if err != nil {
			return "", Manifest{}, cleanupArchive, err
		}
	}
	stageDir, err := os.MkdirTemp("", "camera-appliance-install-")
	if err != nil {
		cleanupArchive()
		return "", Manifest{}, cleanupArchive, err
	}
	cleanup := func() {
		_ = os.RemoveAll(stageDir)
		cleanupArchive()
	}
	if err := extractReleaseArchive(ctx, archivePath, stageDir); err != nil {
		cleanup()
		return "", Manifest{}, func() {}, err
	}
	root, manifest, err := findReleaseRoot(stageDir)
	if err != nil {
		cleanup()
		return "", Manifest{}, func() {}, err
	}
	return root, manifest, cleanup, nil
}

func requireInstallCommands(opts InstallOptions) error {
	for _, name := range []string{"docker"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("%s is required; install Docker before running camera-appliance install", name)
		}
	}
	if err := runCommand(context.Background(), "", "docker", "compose", "version"); err != nil {
		return errors.New("Docker Compose plugin is required; install docker-compose-plugin")
	}
	if opts.EnableSystemd || opts.EnableKiosk {
		if _, err := exec.LookPath("systemctl"); err != nil {
			return errors.New("systemctl is required for --enable-systemd or --enable-kiosk")
		}
	}
	if opts.EnableKiosk {
		if _, err := exec.LookPath("runuser"); err != nil {
			return errors.New("runuser is required for --enable-kiosk")
		}
	}
	return nil
}

func createRuntimeDirs(cfg config.Config, skipSystemLog bool) error {
	dirs := []string{
		cfg.ConfigDir,
		cfg.GeneratedDir(),
		cfg.BackupDir(),
		filepath.Join(cfg.StateDir, "logs"),
	}
	if !skipSystemLog {
		dirs = append(dirs, "/var/log/camera-appliance")
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func ensureSecretsFile(releaseRoot, configDir string) (bool, error) {
	target := filepath.Join(configDir, "secrets.env")
	if pathExists(target) {
		return false, nil
	}
	source := filepath.Join(releaseRoot, ".env.example")
	if !pathExists(source) {
		return false, fmt.Errorf("release is missing .env.example")
	}
	if err := copyFile(source, target, 0o600); err != nil {
		return false, err
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func ensureGo2RTCConfig(cfg config.Config) (bool, error) {
	target := cfg.Go2RTCConfigPath()
	if pathExists(target) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return false, err
	}
	if err := os.WriteFile(target, []byte("streams: {}\n"), 0o600); err != nil {
		return false, err
	}
	return true, nil
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
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func installSystemd(ctx context.Context, installDir string, noStart bool) error {
	unit := filepath.Join(installDir, "systemd", "camera-appliance.service")
	if !pathExists(unit) {
		return fmt.Errorf("systemd unit not found: %s", unit)
	}
	if err := copyFile(unit, "/etc/systemd/system/camera-appliance.service", 0o644); err != nil {
		return err
	}
	if err := runCommand(ctx, "", "systemctl", "daemon-reload"); err != nil {
		return err
	}
	args := []string{"enable", "camera-appliance.service"}
	if !noStart {
		args = []string{"enable", "--now", "camera-appliance.service"}
	}
	return runCommand(ctx, "", "systemctl", args...)
}

func installKiosk(ctx context.Context, installDir, userName string, noStart bool) error {
	account, err := lookupInstallUser(userName)
	if err != nil {
		return err
	}
	unit := filepath.Join(installDir, "systemd", "camera-kiosk.service")
	if !pathExists(unit) {
		return fmt.Errorf("kiosk unit not found: %s", unit)
	}
	userSystemdDir := filepath.Join(account.HomeDir, ".config", "systemd", "user")
	wantsDir := filepath.Join(userSystemdDir, "default.target.wants")
	if err := os.MkdirAll(wantsDir, 0o750); err != nil {
		return err
	}
	target := filepath.Join(userSystemdDir, "camera-kiosk.service")
	if err := copyFile(unit, target, 0o644); err != nil {
		return err
	}
	link := filepath.Join(wantsDir, "camera-kiosk.service")
	_ = os.Remove(link)
	if err := os.Symlink("../camera-kiosk.service", link); err != nil {
		return err
	}
	if err := chownTree(filepath.Join(account.HomeDir, ".config", "systemd"), account.UID, account.GID); err != nil {
		return err
	}
	_ = runCommand(ctx, "", "loginctl", "enable-linger", userName)
	if noStart {
		return nil
	}
	_ = runCommand(ctx, "", "runuser", "-u", userName, "--", "systemctl", "--user", "daemon-reload")
	_ = runCommand(ctx, "", "runuser", "-u", userName, "--", "systemctl", "--user", "restart", "camera-kiosk.service")
	return nil
}

func installDesktopLaunchers(installDir, userName string) error {
	account, err := lookupInstallUser(userName)
	if err != nil {
		return err
	}
	desktopDir := filepath.Join(account.HomeDir, "Desktop")
	if err := os.MkdirAll(desktopDir, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(installDir, "desktop"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
			continue
		}
		target := filepath.Join(desktopDir, entry.Name())
		if err := copyFile(filepath.Join(installDir, "desktop", entry.Name()), target, 0o755); err != nil {
			return err
		}
		if err := os.Chown(target, account.UID, account.GID); err != nil {
			return err
		}
	}
	return nil
}

type installUser struct {
	HomeDir string
	UID     int
	GID     int
}

func lookupInstallUser(userName string) (installUser, error) {
	account, err := user.Lookup(userName)
	if err != nil {
		return installUser{}, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return installUser{}, err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return installUser{}, err
	}
	if account.HomeDir == "" || !pathExists(account.HomeDir) {
		return installUser{}, fmt.Errorf("user home not found for %s", userName)
	}
	return installUser{HomeDir: account.HomeDir, UID: uid, GID: gid}, nil
}

func chownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, uid, gid)
	})
}

func runCommand(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
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
