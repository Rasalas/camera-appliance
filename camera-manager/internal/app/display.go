package app

import (
	"encoding/json"
	"strconv"
	"strings"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/state"
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
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Mode         string               `json:"mode"`
	FocusSlotID  string               `json:"focus_slot_id"`
	SlotOrder    []string             `json:"slot_order"`
	SplitPercent int                  `json:"split_percent"`
	GapPX        int                  `json:"gap_px"`
	Cells        []ViewerLayoutCell   `json:"cells"`
	Custom       ViewerCustomLayout   `json:"custom"`
	Options      []ViewerLayoutOption `json:"options"`
}

type ViewerCustomLayout struct {
	Columns []int                    `json:"columns"`
	Rows    []int                    `json:"rows"`
	Cells   []ViewerCustomLayoutCell `json:"cells"`
}

type ViewerCustomLayoutCell struct {
	SlotID     string `json:"slot_id"`
	Column     int    `json:"column"`
	Row        int    `json:"row"`
	ColumnSpan int    `json:"column_span"`
	RowSpan    int    `json:"row_span"`
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

func viewerLayoutFromSettings(settings map[string]string, slots []config.Slot) ViewerLayout {
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
		SplitPercent: boundedIntSetting(settings, viewerLayoutSettingSplit, 58, 30, 76),
		GapPX:        boundedIntSetting(settings, viewerLayoutSettingGap, 10, 2, 20),
		Cells:        viewerLayoutCells(id, focus, orderedSlots),
		Custom:       custom,
		Options:      DefaultViewerLayoutOptions(),
	}
}

func DefaultViewerLayoutOptions() []ViewerLayoutOption {
	return []ViewerLayoutOption{
		{ID: ViewerLayoutGrid2x2, Name: "2x2", Description: "Vier gleich große Kameras im Raster."},
		{ID: ViewerLayoutFourPlusLarge, Name: "4 plus groß", Description: "Vier Raster-Kameras mit einer prominenten Ansicht."},
		{ID: ViewerLayoutVerticalPlusGrid, Name: "Vertikal plus Raster", Description: "Eine hochformatige Kamera neben einem Raster."},
		{ID: ViewerLayoutLargeOnly, Name: "Große Ansicht", Description: "Nur die prominente Kamera bildschirmfüllend."},
		{ID: ViewerLayoutCustom, Name: "Frei", Description: "Kameras per Drag-and-drop auf Zonen und Größen legen."},
	}
}

func viewerLayoutOption(id string) ViewerLayoutOption {
	for _, option := range DefaultViewerLayoutOptions() {
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

func viewerCustomLayoutFromSettings(raw, focusSlotID string, slots []config.Slot) ViewerCustomLayout {
	var parsed ViewerCustomLayout
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &parsed)
	}
	layout := sanitizeCustomLayout(parsed, focusSlotID, slots)
	if len(layout.Cells) == 0 {
		return defaultCustomLayout(focusSlotID, slots)
	}
	return layout
}

func sanitizeCustomLayout(layout ViewerCustomLayout, focusSlotID string, slots []config.Slot) ViewerCustomLayout {
	if len(slots) == 0 {
		return ViewerCustomLayout{Columns: []int{1}, Rows: []int{1}, Cells: []ViewerCustomLayoutCell{}}
	}
	layout.Columns = normalizedWeights(layout.Columns, []int{29, 29, 6, 36}, 1, 6)
	layout.Rows = normalizedWeights(layout.Rows, []int{50, 50}, 1, 4)
	maxColumn := len(layout.Columns)
	maxRow := len(layout.Rows)
	slotIDs := map[string]bool{}
	for _, slot := range slots {
		slotIDs[slot.ID] = true
	}
	seen := map[string]bool{}
	cells := []ViewerCustomLayoutCell{}
	for _, cell := range layout.Cells {
		if !slotIDs[cell.SlotID] || seen[cell.SlotID] {
			continue
		}
		cell.Column = boundedInt(cell.Column, 1, maxColumn)
		cell.Row = boundedInt(cell.Row, 1, maxRow)
		cell.ColumnSpan = boundedInt(cell.ColumnSpan, 1, maxColumn-cell.Column+1)
		cell.RowSpan = boundedInt(cell.RowSpan, 1, maxRow-cell.Row+1)
		seen[cell.SlotID] = true
		cells = append(cells, cell)
	}
	if len(cells) == 0 {
		return ViewerCustomLayout{}
	}
	for _, cell := range defaultCustomLayout(focusSlotID, slots).Cells {
		if seen[cell.SlotID] {
			continue
		}
		cells = append(cells, cell)
	}
	layout.Cells = cells
	return layout
}

func normalizedWeights(values, fallback []int, minLen, maxLen int) []int {
	if len(values) < minLen || len(values) > maxLen {
		values = fallback
	}
	out := make([]int, 0, len(values))
	for _, value := range values {
		out = append(out, boundedInt(value, 1, 100))
	}
	return out
}

func defaultCustomLayout(focusSlotID string, slots []config.Slot) ViewerCustomLayout {
	columns := []int{29, 29, 6, 36}
	rows := []int{50, 50}
	focus := focusSlotID
	if focus == "" || !slotExists(slots, focus) {
		focus = defaultFocusSlotID(slots)
	}
	cells := []ViewerCustomLayoutCell{{SlotID: focus, Column: 4, Row: 1, ColumnSpan: 1, RowSpan: 2}}
	positions := []ViewerCustomLayoutCell{
		{Column: 1, Row: 1, ColumnSpan: 1, RowSpan: 1},
		{Column: 2, Row: 1, ColumnSpan: 1, RowSpan: 1},
		{Column: 1, Row: 2, ColumnSpan: 1, RowSpan: 1},
		{Column: 2, Row: 2, ColumnSpan: 1, RowSpan: 1},
		{Column: 3, Row: 1, ColumnSpan: 1, RowSpan: 1},
		{Column: 3, Row: 2, ColumnSpan: 1, RowSpan: 1},
	}
	index := 0
	for _, slot := range slots {
		if slot.ID == focus {
			continue
		}
		position := positions[index%len(positions)]
		position.SlotID = slot.ID
		cells = append(cells, position)
		index++
	}
	return ViewerCustomLayout{Columns: columns, Rows: rows, Cells: cells}
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

func displayFromSettings(settings map[string]string, binding state.Binding) CameraDisplay {
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
