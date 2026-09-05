package snapshotupload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"github.com/google/uuid"
)

// Naming stores filenames and remote directory overrides independently of the
// schedule so manual and automatic captures share per-camera file settings.
type Naming struct {
	Mode      string `json:"mode"`
	Filename  string `json:"filename"`
	Directory string `json:"directory"`
}

var jpegFilename = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9_.-]*\.(jpg|jpeg)$`)

func (n Naming) Validate() error {
	if len(n.Directory) > 1024 || hasControl(n.Directory) || strings.ContainsAny(n.Directory, `\:`) {
		return errors.New("Bitte ein Server-Verzeichnis mit / als Trennzeichen eingeben, ohne Steuerzeichen oder URL; höchstens 1024 Zeichen.")
	}
	for _, part := range strings.Split(strings.TrimSpace(n.Directory), "/") {
		if part == ".." {
			return errors.New("Das Kamera-Verzeichnis darf keine übergeordneten Pfade mit .. enthalten.")
		}
	}
	if n.Mode != "unique" && n.Mode != "fixed" {
		return errors.New("Bitte neue Dateien oder einen festen Dateinamen wählen.")
	}
	if (n.Mode == "fixed" || n.Filename != "") && (len(n.Filename) > 120 || !jpegFilename.MatchString(n.Filename)) {
		return errors.New("Dateiname: höchstens 120 Zeichen, beginnend mit Buchstabe oder Zahl, nur A–Z, a–z, 0–9, Punkt, Bindestrich und Unterstrich; Endung .jpg oder .jpeg. Keine Verzeichnisse.")
	}
	return nil
}

func (s *Service) GetNaming(ctx context.Context, deviceID string) (Naming, error) {
	if _, err := s.store.Device(ctx, deviceID); err != nil {
		return Naming{}, &cameraaccess.Failure{Kind: cameraaccess.NotFound, Cause: errors.New("Kamera nicht gefunden.")}
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return Naming{}, err
	}
	n := Naming{Mode: "unique"}
	if raw := settings["snapshot.naming."+deviceID]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			return Naming{}, errors.New("Gespeicherte Dateieinstellungen sind beschädigt.")
		}
	}
	if err := n.Validate(); err != nil {
		return Naming{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	return n, nil
}

func (s *Service) SaveNaming(ctx context.Context, deviceID string, n Naming) (Naming, error) {
	if _, err := s.store.Device(ctx, deviceID); err != nil {
		return Naming{}, &cameraaccess.Failure{Kind: cameraaccess.NotFound, Cause: errors.New("Kamera nicht gefunden.")}
	}
	if err := n.Validate(); err != nil {
		return Naming{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	n.Directory = strings.TrimSpace(n.Directory)
	if n.Directory != "" {
		n.Directory = path.Clean(n.Directory)
	}
	data, _ := json.Marshal(n)
	if err := s.store.PutSettings(ctx, map[string]string{"snapshot.naming." + deviceID: string(data)}); err != nil {
		return Naming{}, err
	}
	return n, nil
}

func (n Naming) filename(deviceID string, now time.Time) string {
	if n.Mode == "fixed" {
		return n.Filename
	}
	hash := sha256.Sum256([]byte(deviceID))
	return "camera-" + hex.EncodeToString(hash[:4]) + "-" + now.UTC().Format("20060102T150405.000Z") + "-" + uuid.NewString() + ".jpg"
}
