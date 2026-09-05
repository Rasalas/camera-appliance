package snapshotupload

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"camera-appliance/camera-manager/internal/cameraaccess"
	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

var ErrInvalid = errors.New("Upload-Einstellungen ungültig")
var ErrBusy = errors.New("Ein Bild-Upload läuft bereits. Bitte warten.")
var ErrRemote = errors.New("Bild-Upload fehlgeschlagen")

type Capture func(context.Context, string, cameraaccess.CredentialsInput) (cameraaccess.Frame, error)

type Service struct {
	store      *state.Store
	configDir  string
	capture    Capture
	mu         sync.Mutex
	busy       sync.Mutex
	scheduleMu sync.Mutex
	runMu      sync.Mutex
	runner     sync.Mutex
	now        func() time.Time
	Send       func(context.Context, Config, string, string, []byte) error
}

func New(store *state.Store, configDir string, capture Capture) *Service {
	return &Service{store: store, configDir: configDir, capture: capture, Send: transfer, now: time.Now}
}

func (s *Service) configuration(ctx context.Context) (Config, string, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return Config{}, "", errors.New("Upload-Einstellungen konnten nicht gelesen werden.")
	}
	c := Config{Protocol: "sftp", Port: 22, Directory: "."}
	if raw := settings[configKey]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return Config{}, "", errors.New("Gespeicherte Upload-Einstellungen sind beschädigt. Bitte erneut speichern.")
		}
	}
	p, err := secrets.LoadUpload(s.configDir, c.target())
	if err != nil {
		return Config{}, "", errors.New("Das Upload-Passwort konnte nicht gelesen werden.")
	}
	return c, p, nil
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, p, err := s.configuration(ctx)
	return Settings{Config: c, PasswordSet: p != ""}, err
}

func (s *Service) SaveSettings(ctx context.Context, input SettingsInput) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	input.Config.normalize()
	if err := input.Config.Validate(); err != nil {
		return Settings{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	if len(input.Password) > 4096 || hasControl(input.Password) {
		return Settings{}, fmt.Errorf("%w: Passwort enthält Steuerzeichen oder ist zu lang.", ErrInvalid)
	}
	if input.ClearPassword && input.Password != "" {
		return Settings{}, fmt.Errorf("%w: Passwort entweder ersetzen oder löschen.", ErrInvalid)
	}
	p, err := secrets.LoadUpload(s.configDir, input.Config.target())
	if err != nil {
		return Settings{}, errors.New("Das Upload-Passwort konnte nicht gelesen werden.")
	}
	if input.Password != "" {
		p = input.Password
	}
	if input.ClearPassword {
		p = ""
	}
	if p == "" && !input.ClearPassword {
		return Settings{}, fmt.Errorf("%w: Bitte ein Passwort für diesen Server und dieses Benutzerkonto eingeben.", ErrInvalid)
	}
	if err := secrets.SaveUpload(s.configDir, input.Config.target(), p); err != nil {
		return Settings{}, errors.New("Das Upload-Passwort konnte nicht geschützt gespeichert werden.")
	}
	data, _ := json.Marshal(input.Config)
	if err := s.store.PutSettings(ctx, map[string]string{configKey: string(data)}); err != nil {
		return Settings{}, errors.New("Upload-Einstellungen konnten nicht gespeichert werden. Bitte erneut versuchen.")
	}
	return Settings{Config: input.Config, PasswordSet: p != ""}, nil
}

func (s *Service) Crop(ctx context.Context, deviceID string) (Crop, error) {
	if _, err := s.store.Device(ctx, deviceID); err != nil {
		return Crop{}, &cameraaccess.Failure{Kind: cameraaccess.NotFound, Cause: errors.New("Kamera nicht gefunden.")}
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return Crop{}, err
	}
	c := Crop{Width: 100, Height: 100}
	if raw := settings["snapshot.crop."+deviceID]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return Crop{}, errors.New("Gespeicherter Upload-Ausschnitt ist beschädigt.")
		}
	}
	return c, nil
}

func (s *Service) SaveCrop(ctx context.Context, deviceID string, c Crop) error {
	if _, err := s.Crop(ctx, deviceID); err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	data, _ := json.Marshal(c)
	return s.store.PutSettings(ctx, map[string]string{"snapshot.crop." + deviceID: string(data)})
}

type UploadInput struct {
	cameraaccess.CredentialsInput
	Crop *Crop `json:"crop"`
}

type Result struct {
	Filename string `json:"filename"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    int    `json:"bytes"`
}

func (s *Service) Upload(ctx context.Context, deviceID string, input UploadInput) (Result, error) {
	if !s.busy.TryLock() {
		return Result{}, ErrBusy
	}
	defer s.busy.Unlock()
	return s.upload(ctx, deviceID, input)
}

func (s *Service) upload(ctx context.Context, deviceID string, input UploadInput) (Result, error) {
	if input.Crop == nil {
		return Result{}, fmt.Errorf("%w: Bitte Gesamtbild oder Bildausschnitt wählen.", ErrInvalid)
	}
	if err := input.Crop.Validate(); err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	naming, err := s.GetNaming(ctx, deviceID)
	if err != nil {
		return Result{}, err
	}
	s.mu.Lock()
	c, p, err := s.configuration(ctx)
	s.mu.Unlock()
	if err != nil {
		return Result{}, err
	}
	if naming.Directory != "" {
		c.Directory = naming.Directory
	}
	if err := c.Validate(); err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	if p == "" {
		return Result{}, fmt.Errorf("%w: Bitte zuerst das Serverpasswort unter System → Bild-Upload speichern.", ErrInvalid)
	}
	frame, err := s.capture(ctx, deviceID, input.CredentialsInput)
	if err != nil {
		return Result{}, err
	}
	data, err := base64.StdEncoding.DecodeString(frame.ImageBase64)
	if err != nil {
		return Result{}, errors.New("Das Kamerabild konnte nicht gelesen werden.")
	}
	data, width, height, err := prepareImage(data, *input.Crop)
	if err != nil {
		return Result{}, err
	}
	filename := naming.filename(deviceID, s.now())
	if err := s.Send(ctx, c, p, filename, data); err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrRemote, err)
	}
	return Result{Filename: filename, Width: width, Height: height, Bytes: len(data)}, nil
}
