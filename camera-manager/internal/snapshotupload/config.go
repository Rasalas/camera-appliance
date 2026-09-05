package snapshotupload

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"unicode"
)

const configKey = "snapshot.upload.config"

type Config struct {
	Protocol  string `json:"protocol"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Directory string `json:"directory"`
	HostKey   string `json:"host_key"`
}

type Settings struct {
	Config
	PasswordSet bool `json:"password_set"`
}

type SettingsInput struct {
	Config
	Password      string `json:"password"`
	ClearPassword bool   `json:"clear_password"`
}

func (c Config) target() string {
	// JSON avoids delimiter ambiguity, including IPv6 addresses.
	b, _ := json.Marshal([]string{c.Protocol, c.Host, strconv.Itoa(c.Port), c.Username, c.HostKey})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (c *Config) normalize() {
	c.Host = strings.ToLower(strings.TrimSpace(c.Host))
	c.Directory = strings.TrimSpace(c.Directory)
	c.HostKey = strings.TrimSpace(c.HostKey)
	if c.Directory == "" {
		c.Directory = "."
	}
}

func (c Config) Validate() error {
	if c.Protocol != "ftp" && c.Protocol != "sftp" {
		return errors.New("Bitte FTP oder SFTP wählen.")
	}
	if c.Host == "" || len(c.Host) > 253 || strings.ContainsAny(c.Host, "/\\@?#[]") || strings.IndexFunc(c.Host, unicode.IsSpace) >= 0 || hasControl(c.Host) || (strings.Contains(c.Host, ":") && net.ParseIP(c.Host) == nil) {
		return errors.New("Bitte einen Servernamen oder eine IP-Adresse ohne Protokoll und Port eingeben.")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("Der Port muss zwischen 1 und 65535 liegen.")
	}
	if c.Username == "" || len(c.Username) > 256 || hasControl(c.Username) {
		return errors.New("Bitte einen gültigen Benutzernamen eingeben.")
	}
	if len(c.Directory) > 1024 || hasControl(c.Directory) || strings.Contains(c.Directory, "\\") {
		return errors.New("Bitte ein gültiges Zielverzeichnis eingeben.")
	}
	if c.Protocol == "sftp" {
		b, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(c.HostKey, "SHA256:"))
		if !strings.HasPrefix(c.HostKey, "SHA256:") || err != nil || len(b) != 32 {
			return errors.New("Für SFTP ist der SHA256-Fingerabdruck des SSH-Hostschlüssels erforderlich.")
		}
	}
	return nil
}

func hasControl(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }
