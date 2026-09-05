package cameraaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"camera-appliance/camera-manager/internal/secrets"
)

type Identity struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Username       string `json:"username"`
	PasswordSet    bool   `json:"password_set"`
	PasswordSource string `json:"password_source,omitempty"`
}

const (
	credentialIdentityIDsKey = "camera.identity.ids"
)

func (s *Service) credentialIdentitiesFromSettings(settings map[string]string) []Identity {
	ids := credentialIdentityIDs(settings)
	identities := make([]Identity, 0, len(ids))
	for _, id := range ids {
		identity := Identity{
			ID:       id,
			Name:     strings.TrimSpace(settings[credentialIdentityKey(id, "name")]),
			Username: strings.TrimSpace(settings[credentialIdentityKey(id, "username")]),
		}
		if identity.Name == "" {
			identity.Name = id
		}
		secret := secrets.LoadIdentity(s.config.ConfigDir, id)
		identity.PasswordSet = secret.Value != ""
		identity.PasswordSource = secret.Source
		identities = append(identities, identity)
	}
	return identities
}

func credentialIdentityIDs(settings map[string]string) []string {
	raw := settings[credentialIdentityIDsKey]
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func appendCredentialIdentityID(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func removeCredentialIdentityID(ids []string, id string) []string {
	out := ids[:0]
	for _, existing := range ids {
		if existing != id {
			out = append(out, existing)
		}
	}
	return out
}

func credentialIdentityKey(id, field string) string {
	return "camera.identity." + sanitizeCredentialIdentityID(id) + "." + field
}

func newCredentialIdentityID(name string) string {
	base := sanitizeCredentialIdentityID(strings.ToLower(strings.TrimSpace(name)))
	if base == "" {
		base = "identity"
	}
	return fmt.Sprintf("%s_%d", base, time.Now().UTC().UnixNano())
}

func sanitizeCredentialIdentityID(value string) string {
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
	return strings.Trim(b.String(), "_")
}

type IdentityInput struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	CopyPasswordFromID string `json:"copy_password_from_id"`
}

func (s *Service) SaveIdentity(ctx context.Context, req IdentityInput) (Identity, error) {
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)
	req.CopyPasswordFromID = strings.TrimSpace(req.CopyPasswordFromID)
	if req.Name == "" {
		return Identity{}, failure(InvalidInput, errors.New("name is required"))
	}
	if req.Username == "" {
		return Identity{}, failure(InvalidInput, errors.New("username is required"))
	}
	if req.ID == "" {
		req.ID = newCredentialIdentityID(req.Name)
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return Identity{}, err
	}
	ids := appendCredentialIdentityID(credentialIdentityIDs(settings), req.ID)
	values := map[string]string{
		credentialIdentityIDsKey:                  strings.Join(ids, ","),
		credentialIdentityKey(req.ID, "name"):     req.Name,
		credentialIdentityKey(req.ID, "username"): req.Username,
	}
	if err := s.store.PutSettings(ctx, values); err != nil {
		return Identity{}, err
	}
	source := ""
	if strings.TrimSpace(req.Password) != "" {
		source, err = secrets.SaveIdentity(s.config.ConfigDir, req.ID, req.Password)
		if err != nil {
			return Identity{}, err
		}
	} else if req.CopyPasswordFromID != "" && req.CopyPasswordFromID != req.ID {
		secret := secrets.LoadIdentity(s.config.ConfigDir, req.CopyPasswordFromID)
		if secret.Value != "" {
			source, err = secrets.SaveIdentity(s.config.ConfigDir, req.ID, secret.Value)
			if err != nil {
				return Identity{}, err
			}
		}
	}
	_ = s.store.AddEvent(ctx, "info", "credentials.identity.updated", "Kamera-Identität wurde gespeichert", map[string]string{"identity_id": req.ID, "password_source": source})
	settings, _ = s.store.Settings(ctx)
	for _, identity := range s.credentialIdentitiesFromSettings(settings) {
		if identity.ID == req.ID {
			return identity, nil
		}
	}
	return Identity{ID: req.ID, Name: req.Name, Username: req.Username}, nil
}

func (s *Service) DeleteIdentity(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return failure(InvalidInput, errors.New("identity id is required"))
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return err
	}
	ids := removeCredentialIdentityID(credentialIdentityIDs(settings), id)
	if err := s.store.PutSettings(ctx, map[string]string{credentialIdentityIDsKey: strings.Join(ids, ",")}); err != nil {
		return err
	}
	// Remove the stored secret as well so credentials never outlive their
	// identity entry.
	secrets.DeleteIdentity(s.config.ConfigDir, id)
	_ = s.store.AddEvent(ctx, "info", "credentials.identity.deleted", "Kamera-Identität wurde entfernt", map[string]string{"identity_id": id})
	return nil
}

func (s *Service) Identities(ctx context.Context) ([]Identity, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return s.credentialIdentitiesFromSettings(settings), nil
}
