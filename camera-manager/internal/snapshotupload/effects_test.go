package snapshotupload

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"
)

func patternedJPEG(t *testing.T) []byte {
	t.Helper()
	i := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			b := uint8(30)
			if (x/3+y/3)%2 == 0 {
				b = 220
			}
			i.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y), b, 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, i, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func decodeJPEG(t *testing.T, data []byte) image.Image {
	t.Helper()
	i, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return i
}
func rgb(i image.Image, x, y int) [3]uint32 {
	r, g, b, _ := i.At(x, y).RGBA()
	return [3]uint32{r >> 8, g >> 8, b >> 8}
}
func assertBlack(t *testing.T, i image.Image, x, y int) {
	t.Helper()
	for _, v := range rgb(i, x, y) {
		if v > 12 {
			t.Fatalf("pixel (%d,%d) is not black: %v", x, y, rgb(i, x, y))
		}
	}
}

func TestMaskPixelsAndCropCoordinates(t *testing.T) {
	data := patternedJPEG(t)
	source := decodeJPEG(t, data)
	settings := ImageSettings{Masks: []Mask{{ID: "black", Mode: "black", X: 10, Y: 10, Width: 30, Height: 30}, {ID: "pixel", Mode: "pixelate", X: 60, Y: 10, Width: 35, Height: 50}}}
	masked := applyMasks(source, settings.Masks)
	for y := 24; y < 96; y++ {
		for x := 32; x < 128; x++ {
			if rgb(masked, x, y) != [3]uint32{} {
				t.Fatal("black rectangle left source pixels")
			}
		}
	}
	if rgb(masked, 10, 200) != rgb(source, 10, 200) {
		t.Fatal("mask changed pixels outside its rectangle")
	}
	// The first 16x16 mosaic cell must be a uniform average, not a sharp sample.
	var sum [3]uint32
	for y := 24; y < 40; y++ {
		for x := 192; x < 208; x++ {
			for c, v := range rgb(source, x, y) {
				sum[c] += v
			}
		}
	}
	average := [3]uint32{sum[0] / 256, sum[1] / 256, sum[2] / 256}
	for y := 24; y < 40; y++ {
		for x := 192; x < 208; x++ {
			if rgb(masked, x, y) != average {
				t.Fatal("pixelation is not a coarse block average")
			}
		}
	}
	for _, crop := range []Crop{{}, {Enabled: true, X: 20, Y: 0, Width: 60, Height: 80}} {
		out, w, h, err := prepareUploadImage(data, crop, settings, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		i := decodeJPEG(t, out)
		x := 0
		if crop.Enabled {
			x = 64
			if w != 192 || h != 192 {
				t.Fatalf("wrong crop size: %dx%d", w, h)
			}
		}
		assertBlack(t, i, 64-x, 50)
		p, q := rgb(i, 196-x, 28), rgb(i, 201-x, 33)
		for c := range p {
			if abs(int(p[c])-int(q[c])) > 15 {
				t.Fatal("encoded mosaic retained fine image details")
			}
		}
	}
	// Later pixel masks cannot reveal an overlapping black region.
	settings.Masks = append(settings.Masks, Mask{ID: "overlap", Mode: "pixelate", X: 0, Y: 0, Width: 100, Height: 100})
	assertBlack(t, applyMasks(source, settings.Masks), 80, 50)
}

func TestTimestampUsesLocalCaptureTimeAfterCropWithoutRevealingMasks(t *testing.T) {
	data := patternedJPEG(t)
	at := time.Date(2026, 9, 5, 12, 34, 56, 0, time.FixedZone("CEST", 2*60*60))
	empty := ImageSettings{Masks: []Mask{}}
	plain, _, _, err := prepareUploadImage(data, Crop{}, empty, at)
	if err != nil || !bytes.Equal(plain, data) {
		t.Fatal("disabled effects changed the original JPEG")
	}
	settings := ImageSettings{Masks: []Mask{{ID: "all", Mode: "black", Width: 100, Height: 100}}, Timestamp: true}
	crop := Crop{Enabled: true, X: 10, Y: 20, Width: 70, Height: 60}
	out, w, h, err := prepareUploadImage(data, crop, settings, at)
	if err != nil || w != 224 || h != 144 {
		t.Fatalf("timestamp crop: %dx%d %v", w, h, err)
	}
	img := decodeJPEG(t, out)
	left, top := w-4-119, h-4-13
	white := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := rgb(img, x, y)
			if x < left-8 || y < top-8 || x >= w-1 || y >= h-1 {
				assertBlack(t, img, x, y)
			}
			if p[0] > 200 && p[1] > 200 && p[2] > 200 {
				white++
			}
		}
	}
	if white < 150 {
		t.Fatal("date/time glyphs missing from actual JPEG")
	}
	// Verify the exact timestamp in raw rendered pixels, including timezone.
	raw := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := drawTimestamp(raw, at); err != nil {
		t.Fatal(err)
	}
	for index, char := range "05.09.2026 12:34:56" {
		for y, row := range stampFont[string(char)] {
			for x := 0; x < 5; x++ {
				want := uint32(0)
				if row&(1<<(4-x)) != 0 {
					want = 255
				}
				if rgb(raw, left+3+index*6+x, top+3+y)[0] != want {
					t.Fatal("wrong local timestamp text")
				}
			}
		}
	}
	utc, _, _, err := prepareUploadImage(data, crop, settings, at.UTC())
	if err != nil || bytes.Equal(out, utc) {
		t.Fatal("timestamp ignored device-local timezone")
	}
	settings.Timestamp = false
	black, _, _, err := prepareUploadImage(data, crop, settings, at)
	if err != nil {
		t.Fatal(err)
	}
	assertBlack(t, decodeJPEG(t, black), left+10, top+6)
	settings.Timestamp = true
	for _, bad := range []struct {
		crop Crop
		at   time.Time
	}{{crop, time.Time{}}, {Crop{Enabled: true, Width: 10, Height: 10}, at}} {
		if output, _, _, err := prepareUploadImage(data, bad.crop, settings, bad.at); err == nil || output != nil {
			t.Fatal("failed timestamp fell back to an unprocessed image")
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
