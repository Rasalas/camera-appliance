package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Upload passwords are bound to a server/account, so editing the destination
// never sends a saved password to a different server. This file is included in
// protected backups, but never in settings or support bundles.
const UploadPasswordFile = "snapshot-upload-password.json"

type uploadPassword struct {
	Target   string `json:"target"`
	Password string `json:"password"`
}

func LoadUpload(configDir, target string) (string, error) {
	data, err := os.ReadFile(filepath.Join(configDir, UploadPasswordFile))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var secret uploadPassword
	if err := json.Unmarshal(data, &secret); err != nil {
		return "", err
	}
	if secret.Target != target {
		return "", nil
	}
	return secret.Password, nil
}

func SaveUpload(configDir, target, password string) error {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return err
	}
	data, err := json.Marshal(uploadPassword{Target: target, Password: password})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(configDir, ".upload-password-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(configDir, UploadPasswordFile))
}
