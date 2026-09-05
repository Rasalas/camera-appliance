package display

import (
	"strconv"
	"strings"

	"camera-appliance/camera-manager/internal/config"
)

const (
	ViewerLayoutAuto        = "auto"
	ViewerLayoutFocusLeft   = "focus_left"
	ViewerLayoutFocusMiddle = "focus_middle"
	ViewerLayoutFocusRight  = "focus_right"

	ViewerLayoutGrid2x2          = "grid_2x2"
	ViewerLayoutFourPlusLarge    = "four_plus_large"
	ViewerLayoutVerticalPlusGrid = "vertical_plus_grid"
	ViewerLayoutLargeOnly        = "large_only"
	ViewerLayoutCustom           = "custom"
	defaultViewerLayoutID        = ViewerLayoutFourPlusLarge
	viewerLayoutSettingID        = "viewer.layout.id"
	viewerLayoutSettingMode      = "viewer.layout.mode"
	viewerLayoutSettingFocusSlot = "viewer.layout.focus_slot_id"
	viewerLayoutSettingSplit     = "viewer.layout.split_percent"
	viewerLayoutSettingGap       = "viewer.layout.gap_px"
	viewerLayoutSettingSlotOrder = "viewer.layout.slot_order"
	viewerLayoutSettingCustom    = "viewer.layout.custom"
	viewerLayoutSettingMosaic    = "viewer.layout.mosaic"
)

type ViewerLayout struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Mode         string               `json:"mode"`
	FocusSlotID  string               `json:"focus_slot_id"`
	SlotOrder    []string             `json:"slot_order"`
	SplitPercent int                  `json:"split_percent"`
	GapPX        int                  `json:"gap_px"`
	Cells        []ViewerLayoutCell   `json:"cells"`
	Custom       ViewerCustomLayout   `json:"custom"`
	Mosaic       string               `json:"mosaic"`
	Options      []ViewerLayoutOption `json:"options"`
}

type ViewerLayoutCell struct {
	ID        string `json:"id"`
	SlotID    string `json:"slot_id,omitempty"`
	Area      string `json:"area"`
	Size      string `json:"size"`
	Transform string `json:"transform,omitempty"`
}

type ViewerLayoutOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func Resolve(settings map[string]string, slots []config.Slot) ViewerLayout {
	id := normalizedViewerLayoutID(settings[viewerLayoutSettingID])
	mode := strings.TrimSpace(settings[viewerLayoutSettingMode])
	if id == "" {
		id = layoutIDFromMode(mode)
	}
	if id == "" {
		id = defaultViewerLayoutID
	}
	mode = normalizedViewerLayoutMode(mode, id)
	focus := strings.TrimSpace(settings[viewerLayoutSettingFocusSlot])
	if focus == "" || !slotExists(slots, focus) {
		focus = defaultFocusSlotID(slots)
	}
	slotOrder := viewerSlotOrder(settings[viewerLayoutSettingSlotOrder], slots)
	orderedSlots := orderSlots(slots, slotOrder)
	custom := viewerCustomLayoutFromSettings(settings[viewerLayoutSettingCustom], focus, orderedSlots)
	option := viewerLayoutOption(id)
	return ViewerLayout{
		ID:           id,
		Name:         option.Name,
		Mode:         mode,
		FocusSlotID:  focus,
		SlotOrder:    slotOrder,
		SplitPercent: boundedIntSetting(settings, viewerLayoutSettingSplit, 58, 12, 88),
		GapPX:        boundedIntSetting(settings, viewerLayoutSettingGap, 10, 2, 20),
		Cells:        viewerLayoutCells(id, focus, orderedSlots),
		Custom:       custom,
		Mosaic:       strings.TrimSpace(settings[viewerLayoutSettingMosaic]),
		Options:      defaultViewerLayoutOptions(),
	}
}

func defaultViewerLayoutOptions() []ViewerLayoutOption {
	return []ViewerLayoutOption{
		{ID: ViewerLayoutGrid2x2, Name: "2x2", Description: "Vier gleich große Kameras im Raster."},
		{ID: ViewerLayoutFourPlusLarge, Name: "4 plus groß", Description: "Vier Raster-Kameras mit einer prominenten Ansicht."},
		{ID: ViewerLayoutVerticalPlusGrid, Name: "Vertikal plus Raster", Description: "Eine hochformatige Kamera neben einem Raster."},
		{ID: ViewerLayoutLargeOnly, Name: "Große Ansicht", Description: "Nur die prominente Kamera bildschirmfüllend."},
		{ID: ViewerLayoutCustom, Name: "Frei", Description: "Kameras per Drag-and-drop auf Zonen und Größen legen."},
	}
}

func viewerLayoutOption(id string) ViewerLayoutOption {
	for _, option := range defaultViewerLayoutOptions() {
		if option.ID == id {
			return option
		}
	}
	return viewerLayoutOption(defaultViewerLayoutID)
}

func normalizedViewerLayoutID(raw string) string {
	switch strings.TrimSpace(raw) {
	case ViewerLayoutGrid2x2:
		return ViewerLayoutGrid2x2
	case ViewerLayoutFourPlusLarge:
		return ViewerLayoutFourPlusLarge
	case ViewerLayoutVerticalPlusGrid:
		return ViewerLayoutVerticalPlusGrid
	case ViewerLayoutLargeOnly:
		return ViewerLayoutLargeOnly
	case ViewerLayoutCustom:
		return ViewerLayoutCustom
	default:
		return ""
	}
}

func layoutIDFromMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ViewerLayoutGrid2x2:
		return ViewerLayoutGrid2x2
	case ViewerLayoutVerticalPlusGrid:
		return ViewerLayoutVerticalPlusGrid
	case ViewerLayoutLargeOnly:
		return ViewerLayoutLargeOnly
	case ViewerLayoutCustom:
		return ViewerLayoutCustom
	case ViewerLayoutAuto, ViewerLayoutFocusLeft, ViewerLayoutFocusMiddle, ViewerLayoutFocusRight:
		return ViewerLayoutFourPlusLarge
	default:
		return ""
	}
}

func normalizedViewerLayoutMode(raw, id string) string {
	mode := strings.TrimSpace(raw)
	if id == ViewerLayoutFourPlusLarge || id == ViewerLayoutVerticalPlusGrid {
		switch mode {
		case ViewerLayoutFocusLeft, ViewerLayoutFocusMiddle, ViewerLayoutFocusRight:
			return mode
		default:
			if id == ViewerLayoutVerticalPlusGrid {
				return ViewerLayoutFocusRight
			}
			return ViewerLayoutAuto
		}
	}
	switch id {
	case ViewerLayoutGrid2x2:
		return ViewerLayoutGrid2x2
	case ViewerLayoutVerticalPlusGrid:
		return ViewerLayoutVerticalPlusGrid
	case ViewerLayoutLargeOnly:
		return ViewerLayoutLargeOnly
	case ViewerLayoutCustom:
		return ViewerLayoutCustom
	default:
		return ViewerLayoutAuto
	}
}

func viewerLayoutCells(id, focusSlotID string, slots []config.Slot) []ViewerLayoutCell {
	switch id {
	case ViewerLayoutGrid2x2:
		return gridCells(slots, focusSlotID, 4)
	case ViewerLayoutVerticalPlusGrid:
		return focusCells(slots, focusSlotID, "portrait", "portrait", 4)
	case ViewerLayoutLargeOnly:
		return []ViewerLayoutCell{{ID: "large", SlotID: focusSlotID, Area: "main", Size: "full"}}
	case ViewerLayoutCustom:
		return gridCells(slots, focusSlotID, len(slots))
	default:
		return focusCells(slots, focusSlotID, "large", "large", 4)
	}
}

func viewerSlotOrder(raw string, slots []config.Slot) []string {
	seen := map[string]bool{}
	order := []string{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" || seen[id] || !slotExists(slots, id) {
			continue
		}
		seen[id] = true
		order = append(order, id)
	}
	for _, slot := range slots {
		if !seen[slot.ID] {
			order = append(order, slot.ID)
		}
	}
	return order
}

func orderSlots(slots []config.Slot, order []string) []config.Slot {
	byID := map[string]config.Slot{}
	for _, slot := range slots {
		byID[slot.ID] = slot
	}
	ordered := []config.Slot{}
	seen := map[string]bool{}
	for _, id := range order {
		slot, ok := byID[id]
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, slot)
	}
	for _, slot := range slots {
		if !seen[slot.ID] {
			ordered = append(ordered, slot)
		}
	}
	return ordered
}

func gridCells(slots []config.Slot, focusSlotID string, limit int) []ViewerLayoutCell {
	cells := []ViewerLayoutCell{}
	for _, slot := range slots {
		if slot.ID == focusSlotID && slot.Role == "large" {
			continue
		}
		cells = append(cells, ViewerLayoutCell{
			ID:     "cell-" + slot.ID,
			SlotID: slot.ID,
			Area:   "grid",
			Size:   "equal",
		})
		if len(cells) >= limit {
			break
		}
	}
	return cells
}

func focusCells(slots []config.Slot, focusSlotID, focusArea, focusSize string, gridLimit int) []ViewerLayoutCell {
	cells := []ViewerLayoutCell{{ID: focusArea, SlotID: focusSlotID, Area: focusArea, Size: focusSize}}
	for _, slot := range slots {
		if slot.ID == focusSlotID {
			continue
		}
		cells = append(cells, ViewerLayoutCell{
			ID:     "cell-" + slot.ID,
			SlotID: slot.ID,
			Area:   "grid",
			Size:   "equal",
		})
		if len(cells)-1 >= gridLimit {
			break
		}
	}
	return cells
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

func boundedIntSetting(settings map[string]string, key string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil {
		value = fallback
	}
	return boundedInt(value, min, max)
}

func boundedInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
