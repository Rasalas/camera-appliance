package snapshotupload

import (
	_ "embed"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"time"
)

//go:embed stamp-font.json
var stampFontJSON []byte
var stampFont = func() map[string][]int {
	var glyphs map[string][]int
	if err := json.Unmarshal(stampFontJSON, &glyphs); err != nil {
		panic(err)
	}
	return glyphs
}()

func frameRegion(x, y, width, height float64, b image.Rectangle) image.Rectangle {
	return image.Rect(b.Min.X+int(math.Floor(x*float64(b.Dx())/100)), b.Min.Y+int(math.Floor(y*float64(b.Dy())/100)), b.Min.X+min(b.Dx(), int(math.Ceil((x+width)*float64(b.Dx())/100))), b.Min.Y+min(b.Dy(), int(math.Ceil((y+height)*float64(b.Dy())/100))))
}

func applyMasks(original image.Image, masks []Mask) *image.RGBA {
	out := image.NewRGBA(original.Bounds())
	draw.Draw(out, out.Bounds(), original, original.Bounds().Min, draw.Src)
	for _, m := range masks {
		if m.Mode != "pixelate" {
			continue
		}
		r := frameRegion(m.X, m.Y, m.Width, m.Height, out.Bounds())
		// At most eight coarse blocks on the longer side, at least 16 source
		// pixels per block. Average the source, never sample a single sharp pixel.
		block := max(16, (max(r.Dx(), r.Dy())+7)/8)
		for y := r.Min.Y; y < r.Max.Y; y += block {
			for x := r.Min.X; x < r.Max.X; x += block {
				cell := image.Rect(x, y, min(x+block, r.Max.X), min(y+block, r.Max.Y))
				var red, green, blue uint64
				for py := cell.Min.Y; py < cell.Max.Y; py++ {
					for px := cell.Min.X; px < cell.Max.X; px++ {
						rr, gg, bb, _ := original.At(px, py).RGBA()
						red += uint64(rr >> 8)
						green += uint64(gg >> 8)
						blue += uint64(bb >> 8)
					}
				}
				n := uint64(cell.Dx() * cell.Dy())
				fill := image.NewUniform(color.RGBA{uint8(red / n), uint8(green / n), uint8(blue / n), 255})
				draw.Draw(out, cell, fill, image.Point{}, draw.Src)
			}
		}
	}
	// Black always wins in overlaps, regardless of the order in the editor.
	for _, m := range masks {
		if m.Mode == "black" {
			draw.Draw(out, frameRegion(m.X, m.Y, m.Width, m.Height, out.Bounds()), image.Black, image.Point{}, draw.Src)
		}
	}
	return out
}

func drawTimestamp(img *image.RGBA, at time.Time) error {
	if at.IsZero() {
		return errors.New("Aufnahmezeit fehlt. Upload mit Zeitangabe abgebrochen.")
	}
	text := at.Format("02.01.2006 15:04:05")
	b := img.Bounds()
	if b.Dx() < 127 || b.Dy() < 21 {
		return errors.New("Für Datum und Uhrzeit muss das fertige Bild mindestens 127 × 21 Pixel groß sein.")
	}
	scale := min(max(2, b.Dx()/600), b.Dx()/127, b.Dy()/21)
	padding, margin := 3*scale, 4*scale
	width, height := (len(text)*6-1)*scale+2*padding, 7*scale+2*padding
	if b.Dx() < width+2*margin || b.Dy() < height+2*margin {
		return errors.New("Für Datum und Uhrzeit muss das fertige Bild mindestens 127 × 21 Pixel groß sein.")
	}
	box := image.Rect(b.Max.X-margin-width, b.Max.Y-margin-height, b.Max.X-margin, b.Max.Y-margin)
	draw.Draw(img, box, image.Black, image.Point{}, draw.Src)
	for i, ch := range text {
		for y, row := range stampFont[string(ch)] {
			for x := 0; x < 5; x++ {
				if row&(1<<(4-x)) != 0 {
					left, top := box.Min.X+padding+(i*6+x)*scale, box.Min.Y+padding+y*scale
					draw.Draw(img, image.Rect(left, top, left+scale, top+scale), image.White, image.Point{}, draw.Src)
				}
			}
		}
	}
	return nil
}
