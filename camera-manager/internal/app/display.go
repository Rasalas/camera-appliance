package app

import (
	"strconv"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
)

const (
	ViewerLayoutAuto       = "auto"
	ViewerLayoutFocusLeft  = "focus_left"
	ViewerLayoutFocusRight = "focus_right"
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

type ViewerLayout struct {
	Mode         string `json:"mode"`
	FocusSlotID  string `json:"focus_slot_id"`
	SplitPercent int    `json:"split_percent"`
	GapPX        int    `json:"gap_px"`
}

func viewerLayoutFromSettings(settings map[string]string, slots []config.Slot) ViewerLayout {
	mode := strings.TrimSpace(settings["viewer.layout.mode"])
	switch mode {
	case ViewerLayoutFocusLeft, ViewerLayoutFocusRight:
	default:
		mode = ViewerLayoutAuto
	}
	focus := strings.TrimSpace(settings["viewer.layout.focus_slot_id"])
	if focus == "" || !slotExists(slots, focus) {
		focus = defaultFocusSlotID(slots)
	}
	return ViewerLayout{
		Mode:         mode,
		FocusSlotID:  focus,
		SplitPercent: boundedIntSetting(settings, "viewer.layout.split_percent", 58, 30, 76),
		GapPX:        boundedIntSetting(settings, "viewer.layout.gap_px", 10, 2, 20),
	}
}

func displayFromSettings(settings map[string]string, binding state.Binding) CameraDisplay {
	display := CameraDisplay{
		Rotation: 0,
		Mirror:   false,
		Flip:     false,
		FitMode:  "cover",
		Crop: DisplayCrop{
			X:      0,
			Y:      0,
			Width:  100,
			Height: 100,
		},
	}
	deviceID := strings.TrimSpace(binding.DeviceID)
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

func defaultFocusSlotID(slots []config.Slot) string {
	for _, slot := range slots {
		if slot.Role == "large" {
			return slot.ID
		}
	}
	if len(slots) > 0 {
		return slots[len(slots)-1].ID
	}
	return ""
}

func slotExists(slots []config.Slot, id string) bool {
	for _, slot := range slots {
		if slot.ID == id {
			return true
		}
	}
	return false
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
	switch strings.TrimSpace(raw) {
	case "contain":
		return "contain"
	default:
		return "cover"
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

func boundedIntSetting(settings map[string]string, key string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
