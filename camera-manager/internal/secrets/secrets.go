package secrets

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	envKey         = "TAPO_CAMERA_PASSWORD"
	keyringService = "camera-appliance"
	keyringAccount = "tapo-camera-password"
)

type Result struct {
	Value  string
	Source string
}

func Load(configDir string) Result {
	if value := os.Getenv(envKey); value != "" {
		return Result{Value: value, Source: "environment"}
	}
	if value, ok := lookupKeyring(keyringAccount); ok {
		return Result{Value: value, Source: "keyring"}
	}
	if value := readLocalEnvKey(configDir, envKey); value != "" {
		return Result{Value: value, Source: "local.env"}
	}
	return Result{}
}

func Save(configDir, password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", errors.New("password is empty")
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
	if value, ok := lookupKeyring(cameraKey(deviceID)); ok {
		return Result{Value: value, Source: "keyring"}
	}
	if value := readLocalEnvKey(configDir, localEnvCameraKey(deviceID)); value != "" {
		return Result{Value: value, Source: "local.env"}
	}
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
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return "", false
	}
	cmd := exec.Command("secret-tool", "lookup", "application", keyringService, "key", account)
	out, err := cmd.Output()
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
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return err
	}
	cmd := exec.Command("secret-tool", "store", "--label", "camera-appliance camera password", "application", keyringService, "key", account)
	cmd.Stdin = strings.NewReader(password)
	return cmd.Run()
}

func readLocalEnvKey(configDir, key string) string {
	file, err := os.Open(filepath.Join(configDir, "local.env"))
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
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(configDir, "local.env")
	existing, _ := os.ReadFile(path)
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
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		return err
	}
	// WriteFile only applies the mode at creation; tighten pre-existing files.
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return nil
}

// DeleteCamera removes a per-camera secret from the keyring and local.env.
func DeleteCamera(configDir, deviceID string) {
	if deviceID == "" {
		return
	}
	deleteKeyring(cameraKey(deviceID))
	removeLocalEnvKey(configDir, localEnvCameraKey(deviceID))
}

// DeleteIdentity removes a credential identity's secret from the keyring and
// local.env.
func DeleteIdentity(configDir, identityID string) {
	if identityID == "" {
		return
	}
	deleteKeyring(identityKey(identityID))
	removeLocalEnvKey(configDir, localEnvIdentityKey(identityID))
}

func deleteKeyring(account string) {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return
	}
	cmd := exec.Command("secret-tool", "clear", "application", keyringService, "key", account)
	_ = cmd.Run()
}

func removeLocalEnvKey(configDir, key string) {
	path := filepath.Join(configDir, "local.env")
	existing, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(existing))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			continue
		}
		out.WriteString(line + "\n")
	}
	if err := os.WriteFile(path, out.Bytes(), 0o600); err == nil {
		_ = os.Chmod(path, 0o600)
	}
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
