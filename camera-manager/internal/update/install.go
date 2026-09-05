package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/releasearchive"
)

func Install(ctx context.Context, opts InstallOptions) (InstallResult, error) {
	if !opts.AllowNonRoot && os.Geteuid() != 0 {
		return InstallResult{}, errors.New("install must run as root; use sudo")
	}
	release, err := EnsureSingleFlight()
	if err != nil {
		return InstallResult{}, err
	}
	defer release()

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

	unlock, err := lockIdle(cfg)
	if err != nil {
		return InstallResult{}, err
	}
	defer unlock()

	installDir, err := cleanInstallDir(opts.InstallDir)
	if err != nil {
		return InstallResult{}, err
	}
	var prepared *releasearchive.Release
	if opts.SourceDir != "" {
		prepared, err = releasearchive.OpenDirectory(opts.SourceDir)
	} else {
		prepared, err = releasearchive.Prepare(ctx, releasearchive.Source{Archive: opts.Archive, URL: opts.URL, Digest: opts.Digest, AllowInsecureURL: opts.AllowInsecureURL}, "", opts.HTTPClient)
	}
	if err != nil {
		return InstallResult{}, err
	}
	defer prepared.Close()
	releaseRoot, manifest := prepared.Root, prepared.Manifest

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
	if err := ensureCommandLink(installDir); err != nil {
		result.Warnings = append(result.Warnings, "CLI-Link konnte nicht erstellt werden: "+err.Error())
	}

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
