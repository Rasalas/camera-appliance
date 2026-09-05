package display_test

import (
	"reflect"
	"testing"

	"camera-appliance/camera-manager/internal/config"
	"camera-appliance/camera-manager/internal/display"
)

func TestResolveNormalizesLayoutWithoutChangingInputs(t *testing.T) {
	slots := config.DefaultSlots()
	before := append([]config.Slot(nil), slots...)
	settings := map[string]string{
		"viewer.layout.id": "custom", "viewer.layout.focus_slot_id": "missing",
		"viewer.layout.slot_order": "cam2,missing,cam2,cam1",
		"viewer.layout.custom":     `{"columns":[0,500],"rows":[50],"cells":[{"slot_id":"cam2","column":100,"row":0,"column_span":100,"row_span":100},{"slot_id":"unknown"},{"slot_id":"cam2"}]}`,
	}
	got := display.Resolve(settings, slots)
	if got.ID != "custom" || got.FocusSlotID != "cam5" || got.SlotOrder[0] != "cam2" {
		t.Fatalf("layout %+v", got)
	}
	if len(got.Custom.Cells) != len(slots) || got.Custom.Columns[0] != 1 || got.Custom.Columns[1] != 100 {
		t.Fatalf("custom layout %+v", got.Custom)
	}
	first := got.Custom.Cells[0]
	if first.SlotID != "cam2" || first.Column != 2 || first.Row != 1 || first.ColumnSpan != 1 || first.RowSpan != 1 {
		t.Fatalf("invalid cell survived: %+v", first)
	}
	if !reflect.DeepEqual(slots, before) || settings["viewer.layout.focus_slot_id"] != "missing" {
		t.Fatal("resolution mutated caller inputs")
	}
	if again := display.Resolve(settings, slots); !reflect.DeepEqual(got, again) {
		t.Fatal("layout resolution is not deterministic")
	}
}

func TestTransformKeepsCropInsideImageAndUsesCameraID(t *testing.T) {
	settings := map[string]string{"camera.display.device.rotation": "90", "camera.display.device.mirror": "true", "camera.display.device.fit_mode": "cover", "camera.display.device.crop_x": "90", "camera.display.device.crop_y": "99", "camera.display.device.crop_width": "60", "camera.display.device.crop_height": "20"}
	got := display.Transform(settings, "device")
	if got.Rotation != 90 || !got.Mirror || got.FitMode != "cover" || got.Crop.X != 40 || got.Crop.Y != 80 {
		t.Fatalf("transform %+v", got)
	}
	other := display.Transform(settings, "other")
	if other.Rotation != 0 || other.Mirror || other.FitMode != "contain" || other.Crop.Width != 100 {
		t.Fatalf("camera settings crossed devices: %+v", other)
	}
}
