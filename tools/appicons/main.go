// Command appicons renders Sinet's committed PWA icon set.
//
// WHY A GENERATOR RATHER THAN THREE HAND-MADE PNGs. The install criteria want a
// 192px and a 512px icon and iOS wants a 180px apple-touch-icon (S15.11 A.7),
// which is three files that have to stay the same mark. A generator makes that
// structural: the geometry is written once, the three sizes fall out of it, and
// a regeneration is reviewable as a code diff rather than as three opaque
// binaries somebody has to trust.
//
// IT ADDS NO DEPENDENCY. image/png, image/draw and math are standard library,
// and tools/ is never part of the release artifact (CONVENTIONS §1). The mark
// is the same one web/public/icon.svg draws — a bright hub with three
// satellites — so the SVG favicon and the raster install icons agree.
//
// Run from the repo root:
//
//	go run ./tools/appicons
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// The palette, matching web/public/icon.svg and the manifest's theme colour.
var (
	background = color.NRGBA{0x10, 0x14, 0x18, 0xff}
	hub        = color.NRGBA{0xe6, 0xed, 0xf3, 0xff}
	satellite  = color.NRGBA{0x7d, 0x8f, 0xa1, 0xff}
)

// The mark is defined once in a 64×64 design space and scaled to each output
// size, so every icon is the same drawing rather than three drawings that
// resemble each other.
const design = 64.0

type vec struct{ x, y float64 }

var (
	hubAt   = vec{32, 32}
	nodes   = []vec{{32, 12}, {52, 42}, {12, 42}}
	hubR    = 6.0
	nodeR   = 4.0
	spokeW  = 2.5
	corner  = 12.0
	spokeIn = 1.6 // spokes stop short of the discs so the joins read cleanly
)

// sizes are the three the platform commits.
//
//	192 and 512 — the Chromium install criteria's own pair (web.dev
//	              install-criteria; MDN "Making PWAs installable"), re-verified
//	              2026-07-30.
//	180        — the iOS Home Screen tile (apple-touch-icon), which is what a
//	             person taps after the Add to Home Screen that iOS still
//	             requires before any web push at all.
var sizes = []struct {
	px   int
	name string
	// rounded is false for the iOS tile: iOS applies its own mask, and a
	// pre-rounded source shows a dark halo inside the system's corner radius.
	rounded bool
}{
	{192, "icon-192.png", true},
	{512, "icon-512.png", true},
	{180, "apple-touch-icon.png", false},
}

func main() {
	out := "web/public"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	for _, s := range sizes {
		if err := write(filepath.Join(out, s.name), s.px, s.rounded); err != nil {
			fmt.Fprintf(os.Stderr, "appicons: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%dx%d)\n", filepath.Join(out, s.name), s.px, s.px)
	}
}

func write(path string, px int, rounded bool) error {
	img := render(px, rounded)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// render draws the mark with 4× supersampling. Analytic coverage would be
// finer, but at these sizes a 16-sample box filter is indistinguishable and the
// code stays something a reader can check by eye.
func render(px int, rounded bool) image.Image {
	const ss = 4
	big := image.NewNRGBA(image.Rect(0, 0, px*ss, px*ss))
	scale := float64(px*ss) / design

	// Background: the full square, or a rounded square where the platform does
	// not apply its own mask.
	draw.Draw(big, big.Bounds(), image.NewUniform(color.NRGBA{}), image.Point{}, draw.Src)
	fill(big, background, func(x, y float64) bool {
		if !rounded {
			return true
		}
		return insideRoundedRect(x, y, design, design, corner)
	}, scale)

	// The three spokes, drawn first so the discs sit on top of them.
	for _, n := range nodes {
		a, b := shorten(hubAt, n, hubR+spokeIn, nodeR+spokeIn)
		fill(big, satellite, func(x, y float64) bool {
			return distanceToSegment(vec{x, y}, a, b) <= spokeW/2
		}, scale)
	}
	// The satellites, then the hub.
	for _, n := range nodes {
		node := n
		fill(big, satellite, func(x, y float64) bool { return dist(vec{x, y}, node) <= nodeR }, scale)
	}
	fill(big, hub, func(x, y float64) bool { return dist(vec{x, y}, hubAt) <= hubR }, scale)

	// Downsample.
	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			var r, g, b, a int
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					c := big.NRGBAAt(x*ss+dx, y*ss+dy)
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					a += int(c.A)
				}
			}
			n := ss * ss
			out.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return out
}

// fill paints every pixel whose design-space centre satisfies `in`.
func fill(dst *image.NRGBA, c color.NRGBA, in func(x, y float64) bool, scale float64) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if in((float64(x)+0.5)/scale, (float64(y)+0.5)/scale) {
				dst.SetNRGBA(x, y, c)
			}
		}
	}
}

func insideRoundedRect(x, y, w, h, r float64) bool {
	if x < 0 || y < 0 || x > w || y > h {
		return false
	}
	cx := math.Max(r, math.Min(x, w-r))
	cy := math.Max(r, math.Min(y, h-r))
	return dist(vec{x, y}, vec{cx, cy}) <= r
}

func dist(a, b vec) float64 { return math.Hypot(a.x-b.x, a.y-b.y) }

// shorten pulls a segment's ends in so a spoke does not poke through a disc.
func shorten(a, b vec, headroomA, headroomB float64) (vec, vec) {
	d := dist(a, b)
	if d == 0 {
		return a, b
	}
	ux, uy := (b.x-a.x)/d, (b.y-a.y)/d
	return vec{a.x + ux*headroomA, a.y + uy*headroomA}, vec{b.x - ux*headroomB, b.y - uy*headroomB}
}

func distanceToSegment(p, a, b vec) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	den := dx*dx + dy*dy
	if den == 0 {
		return dist(p, a)
	}
	t := ((p.x-a.x)*dx + (p.y-a.y)*dy) / den
	t = math.Max(0, math.Min(1, t))
	return dist(p, vec{a.x + t*dx, a.y + t*dy})
}
