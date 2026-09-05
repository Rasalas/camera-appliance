package snapshotupload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"camera-appliance/camera-manager/internal/cameraaccess"
)

type Mask struct {
	ID     string  `json:"id"`
	Mode   string  `json:"mode"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Mask coordinates always refer to the original frame, independently of crop.
type ImageSettings struct {
	Masks     []Mask `json:"masks"`
	Timestamp bool   `json:"timestamp"`
}

var maskID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func (c ImageSettings) Validate() error {
	if c.Masks == nil || len(c.Masks) > 16 {
		return errors.New("Bitte eine Liste mit höchstens 16 Privatbereichen speichern.")
	}
	seen := map[string]bool{}
	for _, m := range c.Masks {
		if !maskID.MatchString(m.ID) || seen[m.ID] || (m.Mode != "black" && m.Mode != "pixelate") {
			return errors.New("Privatbereiche enthalten eine ungültige Kennung oder Darstellungsart.")
		}
		seen[m.ID] = true
		if err := (Crop{Enabled: true, X: m.X, Y: m.Y, Width: m.Width, Height: m.Height}).Validate(); err != nil {
			return errors.New("Jeder Privatbereich muss vollständig im Originalbild liegen und größer als 0 sein.")
		}
	}
	return nil
}

func (s *Service) GetImageSettings(ctx context.Context, id string) (ImageSettings, error) {
	if _, err := s.store.Device(ctx, id); err != nil {
		return ImageSettings{}, &cameraaccess.Failure{Kind: cameraaccess.NotFound, Cause: errors.New("Kamera nicht gefunden.")}
	}
	values, err := s.store.Settings(ctx)
	if err != nil {
		return ImageSettings{}, errors.New("Privatbereiche konnten nicht gelesen werden. Upload gesperrt.")
	}
	c := ImageSettings{Masks: []Mask{}}
	if raw, exists := values["snapshot.image."+id]; exists {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&c); err != nil {
			return ImageSettings{}, errors.New("Gespeicherte Privatbereiche sind beschädigt. Upload gesperrt.")
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return ImageSettings{}, errors.New("Gespeicherte Privatbereiche sind beschädigt. Upload gesperrt.")
		}
		// A present record must explicitly contain its mask list. A truncated
		// or null configuration must never silently disable privacy protection.
		var required struct {
			Masks json.RawMessage `json:"masks"`
		}
		_ = json.Unmarshal([]byte(raw), &required)
		if len(required.Masks) == 0 {
			return ImageSettings{}, errors.New("Gespeicherte Privatbereiche fehlen. Upload gesperrt.")
		}
	}
	if err := c.Validate(); err != nil {
		return ImageSettings{}, fmt.Errorf("%w: %s Upload gesperrt.", ErrInvalid, err)
	}
	return c, nil
}

func (s *Service) SaveImageSettings(ctx context.Context, id string, c ImageSettings) (ImageSettings, error) {
	if err := c.Validate(); err != nil {
		return ImageSettings{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	if _, err := s.store.Device(ctx, id); err != nil {
		return ImageSettings{}, &cameraaccess.Failure{Kind: cameraaccess.NotFound, Cause: errors.New("Kamera nicht gefunden.")}
	}
	// Acknowledgement waits for any current upload, so an older mask set cannot
	// be published after the editor has confirmed that the new one is saved.
	s.busy.Lock()
	defer s.busy.Unlock()
	data, _ := json.Marshal(c)
	if err := s.store.PutSettings(ctx, map[string]string{"snapshot.image." + id: string(data)}); err != nil {
		return ImageSettings{}, errors.New("Privatbereiche konnten nicht gespeichert werden.")
	}
	return c, nil
}
