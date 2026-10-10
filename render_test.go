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

		// The acrylic flyout, saved with its alpha (premultiplied) as is.
		m.acrylic = true
		renderFlyout(cv, m)
		raw := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < w*h; i++ {
			raw.Pix[i*4], raw.Pix[i*4+1], raw.Pix[i*4+2], raw.Pix[i*4+3] = cv.pix[i*4+2], cv.pix[i*4+1], cv.pix[i*4], cv.pix[i*4+3]
		}
		savePNG(t, filepath.Join(dir, "acrylic-"+tc.name+".png"), raw)
	}
	taskbar := rgb{0x20, 0x20, 0x20}
	for _, n := range []int{16, 20, 24, 32, 48} {
		col, alpha := trayInk(true)
		pix := renderSunIcon(n, col, alpha)
		savePNG(t, filepath.Join(dir, "icon-"+string(rune('0'+n/10))+string(rune('0'+n%10))+".png"), toImage(pix, n, n, &taskbar))
	}
}

// On the acrylic flyout everything is composited onto a transparent canvas;
// on the opaque one alpha must stay 255.
func TestAcrylicCanvas(t *testing.T) {
	for _, dark := range []bool{false, true} {
		w, h := flyoutSize(2, 1)
		cv := &canvas{pix: make([]byte, w*h*4), w: w, h: h}
		m := &flyModel{dark: dark, rounded: true, acrylic: true, accent: defaultAccent(dark), scale: 1,
			rows: []rowModel{{"A", 50}, {"B", 0}}, hover: -1, press: -1, focus: -1}
		renderFlyout(cv, m)
		mid := ((h/2)*w + 4*w/5) * 4 // empty background between the rows, right side
		if a := cv.pix[mid+3]; a != 0 {
			t.Errorf("dark=%v: background alpha %d, want 0 so the backdrop shows", dark, a)
		}
		g := rowGeometry(0, 1)
		thumb := (int(g.cy)*w + int(thumbX(g, 50))) * 4
		if a := cv.pix[thumb+3]; a != 255 {
			t.Errorf("dark=%v: thumb alpha %d, want 255", dark, a)
		}
		for i := 0; i < len(cv.pix); i += 4 {
			if a := cv.pix[i+3]; cv.pix[i] > a || cv.pix[i+1] > a || cv.pix[i+2] > a {
				t.Fatalf("dark=%v: pixel %d is not premultiplied: %v", dark, i/4, cv.pix[i:i+4])
			}
		}

		m.acrylic = false
		renderFlyout(cv, m)
		for i := 3; i < len(cv.pix); i += 4 {
			if cv.pix[i] != 255 {
				t.Fatalf("dark=%v: opaque flyout has alpha %d at pixel %d", dark, cv.pix[i], i/4)
			}
		}
	}
}

func TestBlendText(t *testing.T) {
	cv := &canvas{pix: make([]byte, 4*4), w: 4, h: 1}
	mask := []byte{
		255, 255, 255, 0, // full coverage
		128, 128, 128, 0, // half
		0, 0, 0, 0, // none
		255, 255, 255, 0, // full, but outside the box
	}
	cv.blendText(mask, rect{0, 0, 3, 1}, white, 1)
	want := []byte{255, 255, 255, 255, 128, 128, 128, 128, 0, 0, 0, 0, 0, 0, 0, 0}
	for i := range want {
		if cv.pix[i] != want[i] {
			t.Fatalf("white text: got %v, want %v", cv.pix, want)
		}
	}

	// Dark ink at 50% over an opaque white pixel gives mid grey.
	cv = &canvas{pix: []byte{255, 255, 255, 255}, w: 1, h: 1}
	cv.blendText([]byte{255, 255, 255, 0}, rect{0, 0, 1, 1}, black, 0.5)
	if got := cv.pix; got[0] != 128 || got[3] != 255 {
		t.Fatalf("dark text: got %v", got)
	}
}
