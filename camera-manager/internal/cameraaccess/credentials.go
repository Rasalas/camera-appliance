package cameraaccess

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"camera-appliance/camera-manager/internal/secrets"
	"camera-appliance/camera-manager/internal/state"
)

type credentialCandidate struct {
	Source     string
	IdentityID string
	Username   string
	Password   string
	Stream     string
}

func applySavedCredentials(ctx context.Context, s *Service, deviceID string, username, password, stream *string) {
	settings, _ := s.store.Settings(ctx)
	if strings.TrimSpace(*username) == "" {
		*username = settings["camera.credentials."+deviceID+".username"]
	}
	if strings.TrimSpace(*stream) == "" {
		*stream = settings["camera.credentials."+deviceID+".stream"]
	}
	if strings.TrimSpace(*password) == "" {
		*password = secrets.LoadCamera(s.config.ConfigDir, deviceID).Value
	}
}

func (s *Service) frameCredentialCandidates(ctx context.Context, device state.Device, username, password, stream string) ([]credentialCandidate, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		stream = settings["camera.credentials."+device.ID+".stream"]
	}
	if stream == "" {
		stream = "stream2"
	}
	username = strings.TrimSpace(username)
	// Passwords are used verbatim; only whitespace-only input is treated as
	// empty so saved passwords with surrounding spaces keep working.
	if strings.TrimSpace(password) == "" {
		password = secrets.LoadCamera(s.config.ConfigDir, device.ID).Value
	}
	var candidates []credentialCandidate
	if username != "" && password != "" {
		candidates = append(candidates, credentialCandidate{Source: "kamera", Username: username, Password: password, Stream: stream})
	}
	if shouldTryCredentialIdentities(device, settings) {
		for _, identity := range s.credentialIdentitiesFromSettings(settings) {
			secret := secrets.LoadIdentity(s.config.ConfigDir, identity.ID)
			if identity.Username == "" || secret.Value == "" {
				continue
			}
			candidate := credentialCandidate{Source: "identität " + identity.Name, IdentityID: identity.ID, Username: identity.Username, Password: secret.Value, Stream: stream}
			if !sameCredentialCandidate(candidates, candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("username and password are required for frame capture")
	}
	if len(candidates) > 8 {
		candidates = candidates[:8]
	}
	return candidates, nil
}

func sameCredentialCandidate(candidates []credentialCandidate, candidate credentialCandidate) bool {
	for _, existing := range candidates {
		if existing.Username == candidate.Username && existing.Password == candidate.Password && existing.Stream == candidate.Stream {
			return true
		}
	}
	return false
}

func shouldTryCredentialIdentities(device state.Device, settings map[string]string) bool {
	if settings["camera.disable_identity_probe"] == "true" {
		return false
	}
	if device.MACAddress != "" || device.ONVIFEndpointRef != "" || device.SerialNumber != "" || device.HardwareID != "" || device.Hostname != "" {
		return true
	}
	var raw map[string]any
	if len(device.RawJSON) > 0 && json.Unmarshal(device.RawJSON, &raw) == nil {
		if raw["manual"] == true || raw["rtsp_port_open"] == true || raw["onvif_port_open"] == true {
			return true
		}
	}
	return false
}

func (s *Service) rememberIdentityForDevice(ctx context.Context, deviceID string, candidate credentialCandidate) error {
	if candidate.IdentityID == "" {
		return nil
	}
	values := map[string]string{
		"camera.credentials." + deviceID + ".username":    candidate.Username,
		"camera.credentials." + deviceID + ".stream":      candidate.Stream,
		"camera.credentials." + deviceID + ".identity_id": candidate.IdentityID,
	}
	if err := s.store.PutSettings(ctx, values); err != nil {
		return err
	}
	_, err := secrets.SaveCamera(s.config.ConfigDir, deviceID, candidate.Password)
	return err
}

type CredentialsInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Stream   string `json:"stream"`
}

type Credentials struct {
	Status         string `json:"status,omitempty"`
	Username       string `json:"username"`
	Stream         string `json:"stream"`
	PasswordSet    bool   `json:"password_set"`
	PasswordSource string `json:"password_source"`
}

func (s *Service) Credentials(ctx context.Context, deviceID string) (Credentials, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return Credentials{}, err
	}
	secret := secrets.LoadCamera(s.config.ConfigDir, deviceID)
	return Credentials{Username: settings["camera.credentials."+deviceID+".username"], Stream: settings["camera.credentials."+deviceID+".stream"], PasswordSet: secret.Value != "", PasswordSource: secret.Source}, nil
}

func (s *Service) SaveCredentials(ctx context.Context, deviceID string, req CredentialsInput) (Credentials, error) {
	if req.Stream == "" {
		req.Stream = "stream2"
	}
	source := ""
	if strings.TrimSpace(req.Password) != "" {
		var err error
		// Persist the password first: a failure here must not leave a saved
		// username that silently pairs with no credentials.
		source, err = secrets.SaveCamera(s.config.ConfigDir, deviceID, req.Password)
		if err != nil {
			return Credentials{}, err
		}
	}
	values := map[string]string{
		"camera.credentials." + deviceID + ".username": strings.TrimSpace(req.Username),
		"camera.credentials." + deviceID + ".stream":   strings.TrimSpace(req.Stream),
	}
	if err := s.store.PutSettings(ctx, values); err != nil {
		return Credentials{}, err
	}
	_ = s.store.AddEvent(ctx, "info", "camera.credentials.updated", "Kamera-Zugangsdaten wurden gespeichert", map[string]string{"device_id": deviceID, "password_source": source})
	secret := secrets.LoadCamera(s.config.ConfigDir, deviceID)
	return Credentials{Status: "ok", Username: values["camera.credentials."+deviceID+".username"], Stream: values["camera.credentials."+deviceID+".stream"], PasswordSet: secret.Value != "", PasswordSource: secret.Source}, nil
}
