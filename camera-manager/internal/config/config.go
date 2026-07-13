package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"camera-appliance/camera-manager/internal/secrets"
)

const (
	DefaultBindAddr     = "127.0.0.1:8091"
	DefaultConfigDir    = "/etc/camera-appliance"
	DefaultStateDir     = "/var/lib/camera-appliance"
	DefaultGo2RTCURL    = "http://localhost:1984"
	DefaultGo2RTCRTSP   = "rtsp://localhost:8554"
	DefaultComposeFile  = "/opt/camera-appliance/compose.yaml"
	DefaultSlotsRelPath = "config/slots.yaml"
)

type Config struct {
	BindAddr           string
	ConfigDir          string
	StateDir           string
	Go2RTCURL          string
	Go2RTCRTSPURL      string
	Go2RTCRestart      string
	TapoPassword       string
	TapoPasswordSource string
	ComposeFile        string
	SlotsFile          string
	FrontendDist       string
	CaptureSSHHost     string
	ScanLimit          int
	RequestTimeout     time.Duration
}

type Slot struct {
	ID            string `json:"id" yaml:"id"`
	Label         string `json:"label" yaml:"label"`
	Role          string `json:"role" yaml:"role"`
	DefaultStream string `json:"default_stream" yaml:"default_stream"`
	Required      bool   `json:"required" yaml:"required"`
	SortOrder     int    `json:"sort_order" yaml:"sort_order"`
}

type slotFile struct {
	Slots []Slot `yaml:"slots"`
}

func Load() (Config, error) {
	loadEnvFile(".env")
	cfg := Config{
		BindAddr:       getenv("CAMERA_APPLIANCE_BIND_ADDR", DefaultBindAddr),
		ConfigDir:      getenv("CAMERA_APPLIANCE_CONFIG_DIR", DefaultConfigDir),
		StateDir:       getenv("CAMERA_APPLIANCE_STATE_DIR", DefaultStateDir),
		Go2RTCURL:      getenv("CAMERA_APPLIANCE_GO2RTC_URL", DefaultGo2RTCURL),
		Go2RTCRTSPURL:  getenv("CAMERA_APPLIANCE_GO2RTC_RTSP_URL", DefaultGo2RTCRTSP),
		Go2RTCRestart:  strings.TrimSpace(os.Getenv("CAMERA_APPLIANCE_GO2RTC_RESTART_COMMAND")),
		ComposeFile:    getenv("CAMERA_APPLIANCE_COMPOSE_FILE", DefaultComposeFile),
		SlotsFile:      getenv("CAMERA_APPLIANCE_SLOTS_FILE", DefaultSlotsRelPath),
		FrontendDist:   getenv("CAMERA_APPLIANCE_FRONTEND_DIST", "../frontend/dist"),
		CaptureSSHHost: getenv("CAMERA_APPLIANCE_CAPTURE_SSH_HOST", ""),
		ScanLimit:      getenvInt("CAMERA_APPLIANCE_SCAN_LIMIT", 254),
		RequestTimeout: time.Duration(getenvInt("CAMERA_APPLIANCE_TIMEOUT_MS", 800)) * time.Millisecond,
	}
	loadEnvFile(filepath.Join(cfg.ConfigDir, "secrets.env"))
	loadEnvFile(filepath.Join(cfg.ConfigDir, "local.env"))
	secret := secrets.Load(cfg.ConfigDir)
	cfg.TapoPassword = secret.Value
	cfg.TapoPasswordSource = secret.Source
	if env := os.Getenv("CAMERA_APPLIANCE_STATE_DIR"); env == "" {
		cfg.StateDir = writableStateDir(cfg.StateDir)
	}
	if env := os.Getenv("CAMERA_APPLIANCE_CONFIG_DIR"); env == "" && !pathExists(cfg.ConfigDir) {
		cfg.ConfigDir = "."
	}
	if env := os.Getenv("CAMERA_APPLIANCE_COMPOSE_FILE"); env == "" && !pathExists(cfg.ComposeFile) {
		cfg.ComposeFile = localComposeFile(cfg.ComposeFile)
	}
	if cfg.BindAddr == "" {
		return Config{}, errors.New("bind address is empty")
	}
	if _, _, err := net.SplitHostPort(cfg.BindAddr); err != nil {
		return Config{}, fmt.Errorf("invalid bind address %q: %w", cfg.BindAddr, err)
	}
	return cfg, nil
}

func (c Config) DBPath() string {
	return filepath.Join(c.StateDir, "state.db")
}

func (c Config) GeneratedDir() string {
	return filepath.Join(c.StateDir, "generated")
}

func (c Config) Go2RTCConfigPath() string {
	return filepath.Join(c.GeneratedDir(), "go2rtc.yaml")
}

func (c Config) BackupDir() string {
	return filepath.Join(c.StateDir, "backups")
}

func (c Config) ReferenceImageDir() string {
	return filepath.Join(c.StateDir, "reference-images")
}

func LoadSlots(path string) ([]Slot, error) {
	candidates := []string{path, filepath.Join("..", path), filepath.Join("..", "..", path)}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		var parsed slotFile
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if len(parsed.Slots) == 0 {
			break
		}
		return parsed.Slots, nil
	}
	return DefaultSlots(), nil
}

func DefaultSlots() []Slot {
	return []Slot{
		{ID: "cam1", Label: "Kamera 1", Role: "grid", DefaultStream: "stream2", Required: true, SortOrder: 1},
		{ID: "cam2", Label: "Kamera 2", Role: "grid", DefaultStream: "stream2", Required: true, SortOrder: 2},
		{ID: "cam3", Label: "Kamera 3", Role: "grid", DefaultStream: "stream2", Required: true, SortOrder: 3},
		{ID: "cam4", Label: "Kamera 4", Role: "grid", DefaultStream: "stream2", Required: true, SortOrder: 4},
		{ID: "cam5", Label: "Große Ansicht", Role: "large", DefaultStream: "stream2", Required: false, SortOrder: 5},
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func writableStateDir(primary string) string {
	if err := os.MkdirAll(primary, 0o750); err == nil {
		return primary
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "data")
	}
	return filepath.Join(os.TempDir(), "camera-appliance")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func localComposeFile(fallback string) string {
	for _, candidate := range []string{"compose.yaml", filepath.Join("..", "compose.yaml")} {
		if pathExists(candidate) {
			return candidate
		}
	}
	return fallback
}
