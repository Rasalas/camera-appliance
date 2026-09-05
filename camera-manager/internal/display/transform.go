package display

import (
	"strconv"
	"strings"
)

type CameraDisplay struct {
	Rotation int         `json:"rotation"`
	Mirror   bool        `json:"mirror"`
	Flip     bool        `json:"flip"`
	FitMode  string      `json:"fit_mode"`
	Crop     DisplayCrop `json:"crop"`
}

type DisplayCrop struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func Transform(settings map[string]string, deviceID string) CameraDisplay {
	display := CameraDisplay{
		Rotation: 0,
		Mirror:   false,
		Flip:     false,
		FitMode:  "contain",
		Crop: DisplayCrop{
			X:      0,
			Y:      0,
			Width:  100,
			Height: 100,
		},
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return display
	}
	prefix := "camera.display." + deviceID + "."
	display.Rotation = normalizedRotation(settings[prefix+"rotation"])
	display.Mirror = boolSetting(settings, prefix+"mirror", false)
	display.Flip = boolSetting(settings, prefix+"flip", false)
	display.FitMode = normalizedFitMode(settings[prefix+"fit_mode"])
	display.Crop = normalizedCrop(DisplayCrop{
		X:      boundedIntSetting(settings, prefix+"crop_x", 0, 0, 99),
		Y:      boundedIntSetting(settings, prefix+"crop_y", 0, 0, 99),
		Width:  boundedIntSetting(settings, prefix+"crop_width", 100, 1, 100),
		Height: boundedIntSetting(settings, prefix+"crop_height", 100, 1, 100),
	})
	return display
}

func normalizedRotation(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	switch value {
	case 90, 180, 270:
		return value
	default:
		return 0
	}
}

func normalizedFitMode(raw string) string {
	// Default to "contain" so the whole camera image is visible; "cover" is an
	// explicit per-camera opt-in (e.g. when cropping/zooming in the viewer).
	switch strings.TrimSpace(raw) {
	case "cover":
		return "cover"
	default:
		return "contain"
	}
}

func normalizedCrop(crop DisplayCrop) DisplayCrop {
	if crop.Width < 1 {
		crop.Width = 1
	}
	if crop.Height < 1 {
		crop.Height = 1
	}
	if crop.Width > 100 {
		crop.Width = 100
	}
	if crop.Height > 100 {
		crop.Height = 100
	}
	if crop.X < 0 {
		crop.X = 0
	}
	if crop.Y < 0 {
		crop.Y = 0
	}
	if crop.X+crop.Width > 100 {
		crop.X = 100 - crop.Width
	}
	if crop.Y+crop.Height > 100 {
		crop.Y = 100 - crop.Height
	}
	return crop
}

func boolSetting(settings map[string]string, key string, fallback bool) bool {
	raw := strings.TrimSpace(settings[key])
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1" || raw == "yes" || raw == "on"
}
