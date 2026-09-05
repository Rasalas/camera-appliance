package display

import (
	"encoding/json"
	"strings"

	"camera-appliance/camera-manager/internal/config"
)

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
