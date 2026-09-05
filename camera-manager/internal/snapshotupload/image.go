package snapshotupload

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"math"
)

// Crop uses percentages of the original camera frame, before viewer transforms.
type Crop struct {
	Enabled bool    `json:"enabled"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
}

func (c Crop) Validate() error {
	if !c.Enabled {
		return nil
	}
	for _, v := range []float64{c.X, c.Y, c.Width, c.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("Der Bildausschnitt enthält ungültige Zahlen.")
		}
	}
	if c.X < 0 || c.Y < 0 || c.Width <= 0 || c.Height <= 0 || c.X+c.Width > 100 || c.Y+c.Height > 100 {
		return errors.New("Der Bildausschnitt muss innerhalb des Kamerabildes liegen; Breite und Höhe müssen größer als 0 sein.")
	}
	return nil
}

func prepareImage(data []byte, crop Crop) ([]byte, int, int, error) {
	if err := crop.Validate(); err != nil {
		return nil, 0, 0, err
	}
	if len(data) > 32<<20 {
		return nil, 0, 0, errors.New("Das Kamerabild ist zu groß, maximal 32 MB.")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return nil, 0, 0, errors.New("Die Kamera hat kein gültiges JPEG-Bild mit maximal 40 Megapixeln geliefert.")
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, errors.New("Das Kamerabild ist unvollständig oder beschädigt.")
	}
	if !crop.Enabled {
		return data, cfg.Width, cfg.Height, nil
	}
	x := int(math.Floor(crop.X * float64(cfg.Width) / 100))
	y := int(math.Floor(crop.Y * float64(cfg.Height) / 100))
	x2 := min(cfg.Width, int(math.Ceil((crop.X+crop.Width)*float64(cfg.Width)/100)))
	y2 := min(cfg.Height, int(math.Ceil((crop.Y+crop.Height)*float64(cfg.Height)/100)))
	region := image.Rect(x, y, x2, y2)
	sub := img.(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(region)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, sub, &jpeg.Options{Quality: 92}); err != nil {
		return nil, 0, 0, errors.New("Der Bildausschnitt konnte nicht erstellt werden.")
	}
	return out.Bytes(), region.Dx(), region.Dy(), nil
}
