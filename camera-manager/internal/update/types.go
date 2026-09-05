package update

import (
	"context"
	"net/http"
	"time"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/releasearchive"
	"camera-appliance/camera-manager/internal/version"
)

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
	Digest         string
	InstallDir     string
	NoRestart      bool
	AutoRollback   bool
	Restart        func(context.Context) error
	Healthcheck    func(context.Context) error
	HTTPClient     *http.Client
	Now            func() time.Time
	BackupOverride string
	// AllowInsecureURL permits http:// update URLs. It exists for local dev
	// and test setups only; production updates must use https.
	AllowInsecureURL bool
}

type InstallOptions struct {
	Config                  config.Config
	Archive                 string
	URL                     string
	Digest                  string
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
	// AllowInsecureURL permits http:// download URLs (dev/test only).
	AllowInsecureURL bool
}

type RollbackOptions struct {
	Config      config.Config
	InstallDir  string
	NoRestart   bool
	Restart     func(context.Context) error
	Healthcheck func(context.Context) error
}

func manifestFromVersion(info version.Info) Manifest {
	return Manifest{Version: info.Version, Commit: info.Commit, BuildTime: info.BuildTime}
}

func manifestAsVersionInfo(m Manifest) version.Info {
	return version.Info{Version: m.Version, Commit: m.Commit, BuildTime: m.BuildTime}
}

type Manifest = releasearchive.Manifest
