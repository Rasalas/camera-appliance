package update

import (
	"encoding/json"
	"os"
	"path/filepath"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/version"
)

type lastUpdate struct {
	InstallDir  string       `json:"install_dir"`
	BackupPath  string       `json:"backup_path"`
	RollbackDir string       `json:"rollback_dir"`
	AppliedAt   string       `json:"applied_at"`
	OldVersion  version.Info `json:"old_version"`
	NewVersion  Manifest     `json:"new_version"`
}

func writeLastUpdate(cfg config.Config, last lastUpdate) error {
	if err := os.MkdirAll(cfg.BackupDir(), 0o750); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(cfg.BackupDir(), lastUpdateFile), last)
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
