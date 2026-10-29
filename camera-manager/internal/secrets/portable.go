package secrets

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PortableFile is private credential material, included only in protected backups.
const PortableFile = "restored-secrets.json"

type portable map[string]string

func portableValue(dir, key string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, PortableFile))
	if err != nil {
		return "", false
	}
	var values portable
	if json.Unmarshal(data, &values) != nil {
		return "", false
	}
	value, ok := values[key]
	return value, ok
}

// ExportPortable resolves the credentials actually used by the snapshot rather
// than copying only fallback files. The caller must protect the resulting bytes.
func ExportPortable(dir string, devices, identities []string) ([]byte, error) {
	values := portable{}
	global := os.Getenv(envKey)
	if global == "" {
		value, found, err := exportValue(dir, envKey, keyringAccount)
		if err != nil {
			return nil, err
		}
		global = value
		if !found {
			global = readEnvKey(filepath.Join(dir, "secrets.env"), envKey)
		}
	}
	values[envKey] = global
	for _, id := range devices {
		value, found, err := exportValue(dir, localEnvCameraKey(id), cameraKey(id))
		if err != nil {
			return nil, err
		}
		if !found {
			value = global
		}
		values[localEnvCameraKey(id)] = value
	}
	for _, id := range identities {
		value, _, err := exportValue(dir, localEnvIdentityKey(id), identityKey(id))
		if err != nil {
			return nil, err
		}
		values[localEnvIdentityKey(id)] = value
	}
	return json.Marshal(values)
}

// Backup lookups can wait longer than viewer lookups, but must report a failed
// lookup rather than silently archive an empty or different fallback password.
func exportValue(dir, key, account string) (string, bool, error) {
	if value, ok := portableValue(dir, key); ok {
		return value, true, nil
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath(secretToolCommand); err == nil {
			out, err := runSecretTool(keyringWriteTimeout, nil, "lookup", "application", keyringService, "key", account)
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 {
					return "", false, errors.New("Keyring-Zugangsdaten konnten nicht gesichert werden")
				}
			} else if value := strings.TrimSpace(string(out)); value != "" {
				return value, true, nil
			}
		}
	}
	value := readLocalEnvKey(dir, key)
	return value, value != "", nil
}

func ValidatePortable(data []byte) error {
	var values portable
	if err := json.Unmarshal(data, &values); err != nil {
		return errors.New("ungültige Zugangsdaten-Sicherung")
	}
	if values == nil {
		return errors.New("ungültige Zugangsdaten-Sicherung")
	}
	return nil
}

// Keep subsequent edits authoritative over a restored keyring-independent file.
func updatePortable(dir, key, value string) error {
	path := filepath.Join(dir, PortableFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	localEnvMu.Lock()
	defer localEnvMu.Unlock()
	unlock, err := lockDir(dir)
	if err != nil {
		return err
	}
	defer unlock()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var values portable
	if json.Unmarshal(data, &values) != nil || values == nil {
		return errors.New("ungültige lokale Zugangsdaten")
	}
	if value == "" {
		delete(values, key)
	} else {
		values[key] = value
	}
	data, err = json.Marshal(values)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// ExportLegacyPortable uses only the staged files, never the destination keyring
// or process environment, when upgrading an archive without a portable entry.
func ExportLegacyPortable(dir string) ([]byte, error) {
	values := portable{}
	for _, name := range []string{"secrets.env", "local.env"} {
		f, err := os.Open(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
			if ok && (key == envKey || strings.HasPrefix(key, "CAMERA_PASSWORD_") || strings.HasPrefix(key, "CAMERA_IDENTITY_PASSWORD_")) {
				values[key] = unshellQuote(strings.TrimSpace(value))
			}
		}
		scanErr := scanner.Err()
		closeErr := f.Close()
		if err := errors.Join(scanErr, closeErr); err != nil {
			return nil, err
		}
	}
	return json.Marshal(values)
}
