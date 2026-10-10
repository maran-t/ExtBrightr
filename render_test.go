package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func toImage(pix []byte, w, h int, under *rgb) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		b, g, r, a := pix[i*4], pix[i*4+1], pix[i*4+2], pix[i*4+3]
		c := color.NRGBA{r, g, b, a}
		if under != nil {
			m := mixRGB(*under, rgb{r, g, b}, float64(a)/255)
			c = color.NRGBA{m.r, m.g, m.b, 255}
		}
		img.SetNRGBA(i%w, i/w, c)
	}
	return img
}

func savePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// Set RENDER_OUT to a directory to dump the flyout and tray glyph as PNGs.
func TestRenderDump(t *testing.T) {
	dir := os.Getenv("RENDER_OUT")
	if dir == "" {
		t.Skip("RENDER_OUT not set")
	}
	rows := []rowModel{{"MAG 255F", 60}, {"DELL U2720Q", 0}, {"LG ULTRAGEAR", 100}}
	for _, tc := range []struct {
		name  string
		dark  bool
		scale float64
		rows  []rowModel
	}{
		{"dark-100-1", true, 1, rows[:1]},
		{"dark-150-3", true, 1.5, rows},
		{"light-125-2", false, 1.25, rows[:2]},
		{"dark-100-empty", true, 1, nil},
		{"light-200-1", false, 2, rows[:1]},
	} {
		w, h := flyoutSize(len(tc.rows), tc.scale)
		cv := &canvas{pix: make([]byte, w*h*4), w: w, h: h}
		m := &flyModel{dark: tc.dark, rounded: true, accent: defaultAccent(tc.dark), scale: tc.scale,
			rows: tc.rows, hover: 0, press: -1, focus: 1}
		renderFlyout(cv, m)
		savePNG(t, filepath.Join(dir, "fly-"+tc.name+".png"), toImage(cv.pix, w, h, nil))
	}
	taskbar := rgb{0x20, 0x20, 0x20}
	for _, n := range []int{16, 20, 24, 32, 48} {
		col, alpha := trayInk(true)
		pix := renderSunIcon(n, col, alpha)
		savePNG(t, filepath.Join(dir, "icon-"+string(rune('0'+n/10))+string(rune('0'+n%10))+".png"), toImage(pix, n, n, &taskbar))
	}
}
