package browser

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Screen recordings delivered as keyframes with a timeline.

type recEvent struct {
	at time.Duration
	m  core.Msg
}

// note adds an event to the recording timeline (actions, console, network failures, navigation).
func (p *Page) note(m core.Msg) {
	p.mu.Lock()
	if p.rec {
		p.events = append(p.events, recEvent{time.Since(p.recStart), m})
	}
	p.mu.Unlock()
}

func (p *Page) startRecording() {
	p.mu.Lock()
	p.rec, p.recStart, p.frames, p.events, p.trail = true, time.Now(), nil, nil, nil
	w, h, sc := p.width, p.height, p.scale
	p.mu.Unlock()
	_, _ = p.b.call(context.Background(), "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 90, "maxWidth": int(float64(w) * sc), "maxHeight": int(float64(h) * sc), "everyNthFrame": 1})
}

type keyframe struct {
	at      time.Duration
	jpeg    []byte
	changed float64           // share of the screen that changed since the previous keyframe
	regions []image.Rectangle // each separate changed area (page CSS pixels), largest first
	crops   [][]byte          // a zoom of each small changed area
}

// The change grid: each cell is a small square of the page (about 11×11 CSS px on a 1440×900 page).
const sigW, sigH = 128, 80

// signature is a small grayscale thumbnail used to measure change between frames.
func signature(im image.Image) []uint8 {
	b := im.Bounds()
	out := make([]uint8, sigW*sigH)
	for y := 0; y < sigH; y++ {
		for x := 0; x < sigW; x++ {
			// the mean of 4 samples per cell: a thin text change still moves it
			var sum uint32
			for _, o := range [4][2]int{{1, 1}, {3, 1}, {1, 3}, {3, 3}} {
				r, g, bl, _ := im.At(b.Min.X+(x*4+o[0])*b.Dx()/(sigW*4), b.Min.Y+(y*4+o[1])*b.Dy()/(sigH*4)).RGBA()
				sum += (r*299 + g*587 + bl*114) / 1000 >> 8
			}
			out[y*sigW+x] = uint8(sum / 4)
		}
	}
	return out
}

// changedCells marks the cells that differ, leaving out the masked ones (where the agent's own cursor was).
func changedCells(a, b []uint8, mask []bool) ([]bool, int) {
	out := make([]bool, len(a))
	n := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > 14 && (mask == nil || !mask[i]) {
			out[i] = true
			n++
		}
	}
	return out, n
}

// changeRegions groups changed cells into separate areas (cells up to 2 apart belong together).
func changeRegions(cells []bool) []image.Rectangle {
	seen := make([]bool, len(cells))
	var out []image.Rectangle
	for i, c := range cells {
		if !c || seen[i] {
			continue
		}
		box := image.Rect(i%sigW, i/sigW, i%sigW+1, i/sigW+1)
		queue := []int{i}
		seen[i] = true
		for len(queue) > 0 {
			j := queue[0]
			queue = queue[1:]
			x, y := j%sigW, j/sigW
			box = box.Union(image.Rect(x, y, x+1, y+1))
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= sigW || ny >= sigH {
						continue
					}
					k := ny*sigW + nx
					if cells[k] && !seen[k] {
						seen[k] = true
						queue = append(queue, k)
					}
				}
			}
		}
		out = append(out, box)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dx()*out[i].Dy() > out[j].Dx()*out[j].Dy() })
	return out
}

// cursorMask covers the cells the agent's cursor (and its click ripple) passed over between two frames.
func (p *Page) cursorMask(from, to time.Duration, cssW, cssH int) []bool {
	p.mu.Lock()
	trail := append([]cursorPoint{}, p.trail...)
	p.mu.Unlock()
	mask := make([]bool, sigW*sigH)
	rx, ry := 34*sigW/max(cssW, 1)+1, 34*sigH/max(cssH, 1)+1
	var before *cursorPoint
	for i := range trail {
		pt := trail[i]
		if pt.at < from-80*time.Millisecond {
			before = &trail[i]
			continue
		}
		if pt.at > to {
			break
		}
		markCursor(mask, pt, cssW, cssH, rx, ry)
	}
	if before != nil {
		markCursor(mask, *before, cssW, cssH, rx, ry)
	}
	return mask
}

func markCursor(mask []bool, pt cursorPoint, cssW, cssH, rx, ry int) {
	cx, cy := int(pt.x)*sigW/max(cssW, 1), int(pt.y)*sigH/max(cssH, 1)
	for y := cy - ry; y <= cy+ry; y++ {
		for x := cx - rx; x <= cx+rx; x++ {
			if x >= 0 && y >= 0 && x < sigW && y < sigH {
				mask[y*sigW+x] = true
			}
		}
	}
}

// stopRecording picks the keyframes, saves them in dir and returns them with the timeline.
func (p *Page) stopRecording(dir string, maxFrames int) ([]keyframe, []recEvent, time.Duration, int, error) {
	_, _ = p.b.call(context.Background(), "Page.stopScreencast", nil)
	p.mu.Lock()
	frames, events := p.frames, p.events
	p.rec, p.frames, p.events = false, nil, nil
	dur := time.Since(p.recStart)
	cssW, cssH := p.width, p.height
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.trail = nil; p.mu.Unlock() }()
	if len(frames) == 0 {
		return nil, events, dur, 0, core.ErrMsg(core.M("browser.no_frames"))
	}
	if maxFrames <= 0 || maxFrames > 24 {
		maxFrames = 12
	}
	type cand struct {
		f       recFrame
		im      image.Image
		sig     []uint8
		changed float64
		regions []image.Rectangle
	}
	var cands []cand
	for i, f := range frames {
		im, err := jpeg.Decode(bytes.NewReader(f.jpeg))
		if err != nil {
			continue
		}
		sig := signature(im)
		if len(cands) == 0 {
			cands = append(cands, cand{f, im, sig, 1, nil})
			continue
		}
		last := cands[len(cands)-1]
		cells, n := changedCells(last.sig, sig, p.cursorMask(last.f.at, f.at, cssW, cssH))
		if n == 0 {
			if i == len(frames)-1 && last.f.at != f.at {
				cands = append(cands, cand{f, im, sig, 0, nil}) // the final state, even if unchanged
			}
			continue
		}
		// a change that undoes the previous one (a caret blinking, a hover flicker) cancels it out
		if len(cands) >= 2 {
			if _, back := changedCells(cands[len(cands)-2].sig, sig, p.cursorMask(cands[len(cands)-2].f.at, f.at, cssW, cssH)); back == 0 {
				cands = cands[:len(cands)-1]
				continue
			}
		}
		cands = append(cands, cand{f, im, sig, float64(n) / float64(sigW*sigH), changeRegions(cells)})
	}
	// too many: keep the first, the last and the biggest changes, in time order
	if len(cands) > maxFrames {
		idx := make([]int, 0, len(cands)-2)
		for i := 1; i < len(cands)-1; i++ {
			idx = append(idx, i)
		}
		sort.Slice(idx, func(a, b int) bool { return cands[idx[a]].changed > cands[idx[b]].changed })
		keep := map[int]bool{0: true, len(cands) - 1: true}
		for _, i := range idx[:maxFrames-2] {
			keep[i] = true
		}
		var kept []cand
		for i, c := range cands {
			if keep[i] {
				kept = append(kept, c)
			}
		}
		cands = kept
	}
	_ = os.MkdirAll(dir, 0o755)
	var out []keyframe
	for i, c := range cands {
		kf := keyframe{at: c.f.at, jpeg: c.f.jpeg, changed: c.changed}
		b := c.im.Bounds()
		for ri, cell := range c.regions {
			if ri >= 4 {
				break
			}
			px := image.Rect(cell.Min.X*b.Dx()/sigW, cell.Min.Y*b.Dy()/sigH, cell.Max.X*b.Dx()/sigW, cell.Max.Y*b.Dy()/sigH)
			kf.regions = append(kf.regions, image.Rect(px.Min.X*cssW/b.Dx(), px.Min.Y*cssH/b.Dy(), px.Max.X*cssW/b.Dx(), px.Max.Y*cssH/b.Dy()))
			if ri < 2 && float64(px.Dx()*px.Dy())/float64(b.Dx()*b.Dy()) < 0.25 { // a small change: zoom on it
				pad := 24 * b.Dx() / max(cssW, 1)
				crop := px.Inset(-pad).Intersect(b)
				sub := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
				draw.Draw(sub, sub.Bounds(), c.im, crop.Min, draw.Src)
				var buf bytes.Buffer
				_ = jpeg.Encode(&buf, sub, &jpeg.Options{Quality: 92})
				kf.crops = append(kf.crops, buf.Bytes())
			}
		}
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d-%06.2fs.jpg", i+1, c.f.at.Seconds())), c.f.jpeg, 0o644)
		out = append(out, kf)
	}
	return out, events, dur, len(frames), nil
}

// ---------------------------------------------------------------- diagnostics

// networkReport lists the page's requests (only the failed ones, with onlyIssues), one per line, and
// how many it lists.
