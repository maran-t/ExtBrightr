package main

import (
	"math"
	"strconv"
)

// Software renderer for the flyout and the tray glyph. Everything is drawn
// with signed-distance coverage so edges are anti-aliased at any DPI scale.
// Text is not drawn here: renderFlyout returns the strings and boxes, and the
// Windows side draws them with GDI on top of the same bitmap.

type rgb struct{ r, g, b uint8 }

type canvas struct {
	pix  []byte // BGRA, top-down, 4 bytes per pixel
	w, h int
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func mix8(dst, src uint8, a float64) uint8 {
	return uint8(float64(dst) + (float64(src)-float64(dst))*a + 0.5)
}

func mixRGB(under, over rgb, a float64) rgb {
	return rgb{mix8(under.r, over.r, a), mix8(under.g, over.g, a), mix8(under.b, over.b, a)}
}

func (c *canvas) fill(col rgb) {
	for i := 0; i+3 < len(c.pix); i += 4 {
		c.pix[i], c.pix[i+1], c.pix[i+2], c.pix[i+3] = col.b, col.g, col.r, 255
	}
}

// shade blends col over every pixel in the box, weighted by cover(px, py),
// which is evaluated at pixel centres.
func (c *canvas) shade(x0, y0, x1, y1 float64, col rgb, alpha float64, cover func(px, py float64) float64) {
	ix0, iy0 := int(math.Floor(x0)), int(math.Floor(y0))
	ix1, iy1 := int(math.Ceil(x1)), int(math.Ceil(y1))
	if ix0 < 0 {
		ix0 = 0
	}
	if iy0 < 0 {
		iy0 = 0
	}
	if ix1 > c.w {
		ix1 = c.w
	}
	if iy1 > c.h {
		iy1 = c.h
	}
	for y := iy0; y < iy1; y++ {
		for x := ix0; x < ix1; x++ {
			a := clamp01(cover(float64(x)+0.5, float64(y)+0.5)) * alpha
			if a <= 0 {
				continue
			}
			i := (y*c.w + x) * 4
			c.pix[i] = mix8(c.pix[i], col.b, a)
			c.pix[i+1] = mix8(c.pix[i+1], col.g, a)
			c.pix[i+2] = mix8(c.pix[i+2], col.r, a)
		}
	}
}

func sdRoundRect(px, py, x0, y0, x1, y1, r float64) float64 {
	qx := math.Abs(px-(x0+x1)/2) - ((x1-x0)/2 - r)
	qy := math.Abs(py-(y0+y1)/2) - ((y1-y0)/2 - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func (c *canvas) fillRoundRect(x0, y0, x1, y1, r float64, col rgb, alpha float64) {
	if x1 <= x0 || y1 <= y0 {
		return
	}
	r = math.Min(r, math.Min(x1-x0, y1-y0)/2)
	c.shade(x0, y0, x1, y1, col, alpha, func(px, py float64) float64 {
		return 0.5 - sdRoundRect(px, py, x0, y0, x1, y1, r)
	})
}

// strokeRoundRect draws a border of thickness t just inside the rectangle.
func (c *canvas) strokeRoundRect(x0, y0, x1, y1, r, t float64, col rgb, alpha float64) {
	c.shade(x0, y0, x1, y1, col, alpha, func(px, py float64) float64 {
		d := sdRoundRect(px, py, x0, y0, x1, y1, r)
		return clamp01(0.5-d) - clamp01(0.5-(d+t))
	})
}

func (c *canvas) fillCircle(cx, cy, r float64, col rgb, alpha float64) {
	c.shade(cx-r-1, cy-r-1, cx+r+1, cy+r+1, col, alpha, func(px, py float64) float64 {
		return 0.5 - (math.Hypot(px-cx, py-cy) - r)
	})
}

func (c *canvas) strokeCircle(cx, cy, r, t float64, col rgb, alpha float64) {
	c.shade(cx-r-1, cy-r-1, cx+r+1, cy+r+1, col, alpha, func(px, py float64) float64 {
		d := math.Hypot(px-cx, py-cy) - r
		return clamp01(0.5-d) - clamp01(0.5-(d+t))
	})
}

// sunCoverage is the brightness glyph: a ring with eight rays, fitted to a
// square of the given size centred on (cx, cy) and drawn with the given
// stroke width.
func sunCoverage(px, py, cx, cy, size, stroke float64) float64 {
	half := stroke / 2
	r0, r1 := size*0.355, size*0.5-half
	g := sunGeom{ring: size*0.235 - half, r0: r0, r1: r1, d0: r0, d1: r1, stroke: stroke}
	return g.coverage(px-cx, py-cy)
}

// sunGeom lays out the brightness glyph around its centre. The ring's stroke
// is centred on radius ring; the four straight rays are round-capped lines
// whose centrelines run from r0 to r1, and the four diagonal ones from d0 to
// d1 (equal radii give a dot).
type sunGeom struct {
	ring, r0, r1, d0, d1, stroke float64
}

// coverage is how much of the pixel centred at offset (dx, dy) from the
// glyph's centre is inked, from 0 to 1.
func (g sunGeom) coverage(dx, dy float64) float64 {
	half := g.stroke / 2
	cov := clamp01(0.5 - (math.Abs(math.Hypot(dx, dy)-g.ring) - half))

	for k := 0; k < 8; k++ {
		ang := float64(k) * math.Pi / 4
		ux, uy := math.Cos(ang), math.Sin(ang)
		lo, hi := g.r0, g.r1
		if k%2 == 1 {
			lo, hi = g.d0, g.d1
		}
		along := math.Min(math.Max(dx*ux+dy*uy, lo), hi)
		d := math.Hypot(dx-along*ux, dy-along*uy) - half
		cov = math.Max(cov, clamp01(0.5-d))
	}
	return cov
}

func (c *canvas) sun(cx, cy, size float64, col rgb, alpha float64) {
	h := size/2 + 1
	stroke := math.Max(1, size*0.075)
	c.shade(cx-h, cy-h, cx+h, cy+h, col, alpha, func(px, py float64) float64 {
		return sunCoverage(px, py, cx, cy, size, stroke)
	})
}

// trayInk is the colour Windows 11 draws its own tray icons in: the
// TextFillColorPrimary brush, which is white on a dark taskbar and black at
// 89.4% opacity (#E4000000) on a light one. Pure black reads heavier than the
// icons next to it.
func trayInk(dark bool) (rgb, float64) {
	if dark {
		return white, 1
	}
	return black, 0xE4 / 255.0
}

// renderSunIcon returns an n x n BGRA image with straight alpha, as wanted by
// CreateIconIndirect for a 32-bit icon.
//
// The glyph is laid out on a 16-unit grid scaled to the icon size: 1.3-unit
// strokes (between the Segoe Fluent icons' 1 unit, which looks thin here, and
// a heavy 1.6), a ring 8.5 units across, a 0.7-unit gap, straight rays 3 units
// long and diagonal rays 2.4. The shorter diagonals keep the outline round, so
// the glyph reaches the edge of the box only at its four tips.
func renderSunIcon(n int, col rgb, alpha float64) []byte {
	pix := make([]byte, n*n*4)
	u := float64(n) / 16
	c := float64(n) / 2
	const stroke = 1.3
	g := sunGeom{
		ring:   3.6 * u,
		r0:     5.6 * u,
		r1:     (5.6 + 3.0 - stroke) * u,
		d0:     5.6 * u,
		d1:     (5.6 + 2.4 - stroke) * u,
		stroke: stroke * u,
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			a := g.coverage(float64(x)+0.5-c, float64(y)+0.5-c)
			i := (y*n + x) * 4
			pix[i], pix[i+1], pix[i+2] = col.b, col.g, col.r
			pix[i+3] = uint8(a*alpha*255 + 0.5)
		}
	}
	return pix
}

// --- flyout ---------------------------------------------------------------

// Layout, in device-independent pixels (96 DPI).
const (
	flyW      = 360.0
	padX      = 20.0
	padTop    = 12.0
	rowH      = 72.0
	sliderY   = 52.0 // slider centre below the row top; the name sits at 4..28
	padBottom = 10.0
	emptyH    = 116.0
	thumbR    = 10.0
	glyphBox  = 20.0
	glyphGap  = 12.0
	valueW    = 56.0
)

const (
	styleTitle = iota // 14px semibold
	styleBody         // 14px regular
	styleSmall        // 12px regular
)

const (
	alignLeft = iota
	alignRight
	alignCenterWrap
)

type textItem struct {
	box   rect
	s     string
	col   rgb
	style int
	align int
}

type rowModel struct {
	name string
	pct  int
}

type flyModel struct {
	dark    bool
	rounded bool // Windows 11: DWM rounds the window, so round the border too
	accent  rgb
	scale   float64
	rows    []rowModel
	hover   int // row whose thumb is under the pointer, or -1
	press   int // row being dragged, or -1
	focus   int // row selected with the keyboard, or -1
}

type palette struct {
	bg         rgb
	border     rgb
	borderA    float64
	text       rgb
	text2      rgb
	track      rgb
	trackA     float64
	thumb      rgb
	thumbRingA float64
	shadowA    float64
	focusA     float64
	accent     rgb
}

var (
	white = rgb{255, 255, 255}
	black = rgb{0, 0, 0}
)

func paletteFor(dark bool, accent rgb) palette {
	if dark {
		bg := rgb{0x2C, 0x2C, 0x2C}
		return palette{
			bg: bg, border: white, borderA: 0.10,
			text: white, text2: mixRGB(bg, white, 0.786),
			track: white, trackA: 0.544,
			thumb: rgb{0x45, 0x45, 0x45}, thumbRingA: 0.10, shadowA: 0.22,
			focusA: 0.06, accent: accent,
		}
	}
	bg := rgb{0xF3, 0xF3, 0xF3}
	return palette{
		bg: bg, border: black, borderA: 0.14,
		text: mixRGB(bg, black, 0.896), text2: mixRGB(bg, black, 0.62),
		track: black, trackA: 0.446,
		thumb: white, thumbRingA: 0.08, shadowA: 0.12,
		focusA: 0.045, accent: accent,
	}
}

// defaultAccent is what Windows ships with, used when the user's accent
// palette cannot be read.
func defaultAccent(dark bool) rgb {
	if dark {
		return rgb{0x60, 0xCD, 0xFF}
	}
	return rgb{0x00, 0x5F, 0xB8}
}

func flyoutSize(rows int, s float64) (w, h int) {
	w = int(math.Round(flyW * s))
	if rows == 0 {
		return w, int(math.Round(emptyH * s))
	}
	return w, int(math.Round((padTop + rowH*float64(rows) + padBottom) * s))
}

// rowGeom is one monitor's row in device pixels. The track runs x0..x1; the
// thumb centre travels t0..t1 so the thumb never overhangs the track.
type rowGeom struct {
	top, bottom, cy float64
	x0, x1, t0, t1  float64
}

func rowGeometry(i int, s float64) rowGeom {
	top := math.Round((padTop + rowH*float64(i)) * s)
	g := rowGeom{
		top:    top,
		bottom: math.Round((padTop + rowH*float64(i+1)) * s),
		cy:     math.Round(top + sliderY*s),
		x0:     math.Round((padX + glyphBox + glyphGap) * s),
		x1:     math.Round((flyW - padX) * s),
	}
	g.t0, g.t1 = g.x0+thumbR*s, g.x1-thumbR*s
	return g
}

func thumbX(g rowGeom, pct int) float64 {
	return g.t0 + (g.t1-g.t0)*float64(pct)/100
}

func pctFromX(g rowGeom, x float64) int {
	p := int(math.Round((x - g.t0) / (g.t1 - g.t0) * 100))
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// rowAt returns the row containing y, or -1.
func rowAt(rows int, s, y float64) int {
	for i := 0; i < rows; i++ {
		g := rowGeometry(i, s)
		if y >= g.top && y < g.bottom {
			return i
		}
	}
	return -1
}

// onSlider reports whether (x, y) is close enough to row i's slider to grab it.
func onSlider(g rowGeom, s, x, y float64) bool {
	return math.Abs(y-g.cy) <= 16*s && x >= g.x0-8*s && x <= g.x1+8*s
}

const (
	emptyTitle = "No external monitor found"
	emptyBody  = "Turn on DDC/CI in your monitor's on-screen menu, then right-click the tray icon and choose Rescan monitors."
)

func px(v float64) int32 { return int32(math.Round(v)) }

func renderFlyout(cv *canvas, m *flyModel) []textItem {
	p := paletteFor(m.dark, m.accent)
	s := m.scale
	W, H := float64(cv.w), float64(cv.h)

	cv.fill(p.bg)
	radius := 0.0
	if m.rounded {
		radius = 8 * s
	}
	cv.strokeRoundRect(0, 0, W, H, radius, 1, p.border, p.borderA)

	var texts []textItem
	if len(m.rows) == 0 {
		cv.sun(W/2, 30*s, 22*s, p.text2, 1)
		texts = append(texts,
			textItem{rect{px(padX * s), px(46 * s), px(W - padX*s), px(68 * s)}, emptyTitle, p.text, styleTitle, alignCenterWrap},
			textItem{rect{px(padX * s), px(68 * s), px(W - padX*s), px(H - 8*s)}, emptyBody, p.text2, styleSmall, alignCenterWrap},
		)
		return texts
	}

	for i, r := range m.rows {
		g := rowGeometry(i, s)
		if m.focus == i && len(m.rows) > 1 {
			cv.fillRoundRect(8*s, g.top+2*s, W-8*s, g.bottom-2*s, 6*s, p.text, p.focusA)
		}

		split := (flyW - padX - valueW) * s
		texts = append(texts,
			textItem{rect{px(padX * s), px(g.top + 4*s), px(split - 8*s), px(g.top + 28*s)}, r.name, p.text, styleTitle, alignLeft},
			textItem{rect{px(split), px(g.top + 4*s), px((flyW - padX) * s), px(g.top + 28*s)}, strconv.Itoa(r.pct) + "%", p.text2, styleBody, alignRight},
		)

		cv.sun((padX+glyphBox/2)*s, g.cy, glyphBox*s, p.text2, 1)

		half := 2 * s
		tx := thumbX(g, r.pct)
		cv.fillRoundRect(g.x0, g.cy-half, g.x1, g.cy+half, half, p.track, p.trackA)
		cv.fillRoundRect(g.x0, g.cy-half, tx, g.cy+half, half, p.accent, 1)

		rad := thumbR * s
		cv.fillCircle(tx, g.cy+s, rad+0.5*s, black, p.shadowA)
		cv.fillCircle(tx, g.cy, rad, p.thumb, 1)
		cv.strokeCircle(tx, g.cy, rad, 1, p.text, p.thumbRingA)
		inner := 5.2
		switch i {
		case m.press:
			inner = 4.3
		case m.hover:
			inner = 7
		}
		cv.fillCircle(tx, g.cy, inner*s, p.accent, 1)
	}
	return texts
}
