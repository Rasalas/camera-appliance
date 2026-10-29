package secrets

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	envKey         = "TAPO_CAMERA_PASSWORD"
	keyringService = "camera-appliance"
	keyringAccount = "tapo-camera-password"

	// Lookups sit on viewer and render paths, so a stuck keyring must fail fast.
	keyringLookupTimeout = time.Second
	keyringWriteTimeout  = 5 * time.Second
)

var (
	secretToolCommand = "secret-tool"
	localEnvMu        sync.Mutex
)

type Result struct {
	Value  string
	Source string
}

func Load(configDir string) Result {
	if value := os.Getenv(envKey); value != "" {
		return Result{Value: value, Source: "environment"}
	}
	if value, ok := portableValue(configDir, envKey); ok {
		return Result{Value: value, Source: "Backup"}
	}
	if value, ok := lookupKeyring(keyringAccount); ok {
		return Result{Value: value, Source: "keyring"}
	}
	if value := readLocalEnvKey(configDir, envKey); value != "" {
		return Result{Value: value, Source: "local.env"}
	}
	if value := readEnvKey(filepath.Join(configDir, "secrets.env"), envKey); value != "" {
		return Result{Value: value, Source: "secrets.env"}
	}
	return Result{}
}

func Save(configDir, password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", errors.New("password is empty")
	}
	if err := updatePortable(configDir, envKey, password); err != nil {
		return "", err
	}
	if saveKeyring(keyringAccount, password) == nil {
		return "keyring", nil
	}
	if err := writeLocalEnvKey(configDir, envKey, password); err != nil {
		return "", err
	}
	return "local.env", nil
}

func LoadCamera(configDir, deviceID string) Result {
	if deviceID == "" {
		return Load(configDir)
	}
	if value, ok := portableValue(configDir, localEnvCameraKey(deviceID)); ok {
		if value == "" {
			return inheritedCameraPassword(configDir)
		}
		return Result{Value: value, Source: "Backup"}
	}
	if value, ok := lookupKeyring(cameraKey(deviceID)); ok {
		return Result{Value: value, Source: "keyring"}
	}
	if value := readLocalEnvKey(configDir, localEnvCameraKey(deviceID)); value != "" {
		return Result{Value: value, Source: "local.env"}
	}
	return inheritedCameraPassword(configDir)
}

func inheritedCameraPassword(configDir string) Result {
	global := Load(configDir)
	if global.Value != "" {
		global.Source = "global:" + global.Source
	}
	return global
}

func LoadIdentity(configDir, identityID string) Result {
	if identityID == "" {
		return Result{}
	}
	if value, ok := portableValue(configDir, localEnvIdentityKey(identityID)); ok {
		return Result{Value: value, Source: "Backup"}
	}
	if value, ok := lookupKeyring(identityKey(identityID)); ok {
		return Result{Value: value, Source: "keyring"}
	}
	if value := readLocalEnvKey(configDir, localEnvIdentityKey(identityID)); value != "" {
		return Result{Value: value, Source: "local.env"}
	}
	return Result{}
}

func SaveIdentity(configDir, identityID, password string) (string, error) {
	password = strings.TrimSpace(password)
	if identityID == "" {
		return "", errors.New("identity id is empty")
	}
	if password == "" {
		return "", errors.New("password is empty")
	}
	if err := updatePortable(configDir, localEnvIdentityKey(identityID), password); err != nil {
		return "", err
	}
	if saveKeyring(identityKey(identityID), password) == nil {
		return "keyring", nil
	}
	if err := writeLocalEnvKey(configDir, localEnvIdentityKey(identityID), password); err != nil {
		return "", err
	}
	return "local.env", nil
}

func SaveCamera(configDir, deviceID, password string) (string, error) {
	password = strings.TrimSpace(password)
	if deviceID == "" {
		return "", errors.New("device id is empty")
	}
	if password == "" {
		return "", errors.New("password is empty")
	}
	if err := updatePortable(configDir, localEnvCameraKey(deviceID), password); err != nil {
		return "", err
	}
	if saveKeyring(cameraKey(deviceID), password) == nil {
		return "keyring", nil
	}
	if err := writeLocalEnvKey(configDir, localEnvCameraKey(deviceID), password); err != nil {
		return "", err
	}
	return "local.env", nil
}

func lookupKeyring(account string) (string, bool) {
	if runtime.GOOS != "linux" {
		return "", false
	}
	if _, err := exec.LookPath(secretToolCommand); err != nil {
		return "", false
	}
	out, err := runSecretTool(keyringLookupTimeout, nil, "lookup", "application", keyringService, "key", account)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(out))
	return value, value != ""
}

func saveKeyring(account, password string) error {
	if runtime.GOOS != "linux" {
		return errors.New("keyring is only implemented for linux secret-tool")
	}
	if _, err := exec.LookPath(secretToolCommand); err != nil {
		return err
	}
	_, err := runSecretTool(keyringWriteTimeout, strings.NewReader(password), "store", "--label", "camera-appliance camera password", "application", keyringService, "key", account)
	return err
}

func runSecretTool(timeout time.Duration, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, secretToolCommand, args...)
	cmd.Stdin = stdin
	// Without WaitDelay, Output waits for grandchildren that still hold stdout.
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd.Output()
}

func readLocalEnvKey(configDir, key string) string {
	return readEnvKey(filepath.Join(configDir, "local.env"), key)
}

func readEnvKey(path, key string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, key+"=") {
			return unshellQuote(strings.TrimSpace(strings.TrimPrefix(line, key+"=")))
		}
	}
	return ""
}

// unshellQuote removes exactly one layer of quoting written by shellQuote.
// Legacy hand-edited double-quoted values are handled too.
func unshellQuote(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "'\"'\"'", "'")
	}
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value[1 : len(value)-1]
	}
	return value
}

func writeLocalEnvKey(configDir, key, password string) error {
	return updateLocalEnv(configDir, func(existing []byte) ([]byte, bool) {
		var out bytes.Buffer
		found := false
		scanner := bufio.NewScanner(bytes.NewReader(existing))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
				out.WriteString(key + "=" + shellQuote(password) + "\n")
				found = true
				continue
			}
			out.WriteString(line + "\n")
		}
		if !found {
			out.WriteString(key + "=" + shellQuote(password) + "\n")
		}
		return out.Bytes(), true
	})
}

// updateLocalEnv serializes read-modify-write of local.env and replaces the
// file atomically. edit returns false when nothing needs to be written.
func updateLocalEnv(configDir string, edit func(existing []byte) ([]byte, bool)) error {
	localEnvMu.Lock()
	defer localEnvMu.Unlock()
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return err
	}
	unlock, err := lockDir(configDir)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(configDir, "local.env")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		// Writing anyway would drop every other key in the file.
		return err
	}
	data, changed := edit(existing)
	if !changed {
		return nil
	}
	return writeFileAtomic(path, data)
}

// writeFileAtomic writes an owner-only file via a synced temp file and rename,
// so a crash leaves either the old or the new content.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
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
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	// Best effort: some filesystems reject fsync on directories.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// DeleteCamera removes a per-camera secret from the keyring and local.env.
func DeleteCamera(configDir, deviceID string) {
	if deviceID == "" {
		return
	}
	_ = updatePortable(configDir, localEnvCameraKey(deviceID), "")
	deleteKeyring(cameraKey(deviceID))
	removeLocalEnvKey(configDir, localEnvCameraKey(deviceID))
}

// DeleteIdentity removes a credential identity's secret from the keyring and
// local.env.
func DeleteIdentity(configDir, identityID string) {
	if identityID == "" {
		return
	}
	_ = updatePortable(configDir, localEnvIdentityKey(identityID), "")
	deleteKeyring(identityKey(identityID))
	removeLocalEnvKey(configDir, localEnvIdentityKey(identityID))
}

func deleteKeyring(account string) {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath(secretToolCommand); err != nil {
		return
	}
	_, _ = runSecretTool(keyringWriteTimeout, nil, "clear", "application", keyringService, "key", account)
}

func removeLocalEnvKey(configDir, key string) {
	if _, err := os.Stat(filepath.Join(configDir, "local.env")); err != nil {
		return
	}
	_ = updateLocalEnv(configDir, func(existing []byte) ([]byte, bool) {
		var out bytes.Buffer
		found := false
		scanner := bufio.NewScanner(bytes.NewReader(existing))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
				found = true
				continue
			}
			out.WriteString(line + "\n")
		}
		return out.Bytes(), found
	})
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func cameraKey(deviceID string) string {
	return "camera-password-" + sanitizeKey(deviceID)
}

func localEnvCameraKey(deviceID string) string {
	return "CAMERA_PASSWORD_" + strings.ToUpper(sanitizeKey(deviceID))
}

func identityKey(identityID string) string {
	return "credential-identity-" + sanitizeKey(identityID)
}

func localEnvIdentityKey(identityID string) string {
	return "CAMERA_IDENTITY_PASSWORD_" + strings.ToUpper(sanitizeKey(identityID))
}

func sanitizeKey(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
