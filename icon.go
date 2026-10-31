package main

// The application icon, drawn at runtime: a filter cone over a dark disc, neutral greys. One source for
// the window, the tray and the taskbar; no asset pipeline.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
)

var iconCache sync.Map // size → []byte

func iconPNG(size int) []byte {
	if b, ok := iconCache.Load(size); ok {
		return b.([]byte)
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	r := c - 0.5
	bg := color.NRGBA{0x2b, 0x2f, 0x36, 0xff}
	fg := color.NRGBA{0xe8, 0xea, 0xed, 0xff}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			d := dx*dx + dy*dy
			if d > r*r {
				continue
			}
			img.SetNRGBA(x, y, bg)
			// cone: a trapezoid narrowing downward, with a stem
			fy := (float64(y) + 0.5) / float64(size) // 0..1
			half := 0.0
			switch {
			case fy >= 0.24 && fy < 0.56:
				half = 0.30 - (fy-0.24)/(0.32)*0.22
			case fy >= 0.56 && fy < 0.78:
				half = 0.055
			}
			if half > 0 && dx/float64(size) > -half && dx/float64(size) < half {
				img.SetNRGBA(x, y, fg)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	iconCache.Store(size, buf.Bytes())
	return buf.Bytes()
}
