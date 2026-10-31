package browser

import (
	"context"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"math"
	"math/rand"
	"strings"
	"time"
)

// Human-like cursor, clicks, keys, typing and scrolling.

type gestureStyle struct {
	stepPx    float64       // path resolution
	stepDelay time.Duration // between path points
	preClick  time.Duration // hover before pressing
	press     time.Duration // button held
	keyMin    time.Duration // typing rhythm
	keyMax    time.Duration
}

func gesturesFor(st config.Settings) gestureStyle {
	switch st.BrowserGestures {
	case "instant":
		return gestureStyle{stepPx: 1e9}
	case "slow":
		return gestureStyle{stepPx: 10, stepDelay: 16 * time.Millisecond, preClick: 260 * time.Millisecond, press: 140 * time.Millisecond, keyMin: 110 * time.Millisecond, keyMax: 260 * time.Millisecond}
	}
	return gestureStyle{stepPx: 18, stepDelay: 9 * time.Millisecond, preClick: 110 * time.Millisecond, press: 80 * time.Millisecond, keyMin: 45 * time.Millisecond, keyMax: 140 * time.Millisecond}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// moveTo travels from the cursor to (x, y) along a curved path with ease-in-out, like a hand would.
func (p *Page) moveTo(ctx context.Context, g gestureStyle, x, y float64, buttons int) {
	p.mu.Lock()
	x0, y0 := p.x, p.y
	w, h := p.width, p.height
	p.mu.Unlock()
	if x0 < 0 {
		x0, y0 = float64(w)*0.5+rand.Float64()*40, float64(h)*0.9
	}
	d := math.Hypot(x-x0, y-y0)
	steps := int(math.Max(1, math.Min(60, d/g.stepPx)))
	// a control point off the straight line: hands do not move in straight lines
	nx, ny := -(y - y0), x-x0
	if d > 0 {
		nx, ny = nx/d, ny/d
	}
	bend := (rand.Float64() - 0.5) * d * 0.3
	cx, cy := (x0+x)/2+nx*bend, (y0+y)/2+ny*bend
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		t = t * t * (3 - 2*t) // ease in and out
		px := (1-t)*(1-t)*x0 + 2*(1-t)*t*cx + t*t*x
		py := (1-t)*(1-t)*y0 + 2*(1-t)*t*cy + t*t*y
		if i == steps {
			px, py = x, y
		}
		p.mouse(ctx, "mouseMoved", px, py, buttons, 0)
		sleepCtx(ctx, g.stepDelay)
	}
}

func (p *Page) mouse(ctx context.Context, typ string, x, y float64, buttons, clicks int) {
	params := map[string]any{"type": typ, "x": x, "y": y, "buttons": buttons}
	if typ != "mouseMoved" {
		params["button"] = "left"
		params["clickCount"] = clicks
	} else if buttons == 1 {
		params["button"] = "left"
	}
	_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", params)
	p.mu.Lock()
	p.x, p.y = x, y
	if p.rec {
		p.trail = append(p.trail, cursorPoint{time.Since(p.recStart), x, y})
	}
	p.mu.Unlock()
	_, _ = p.b.eval(ctx, fmt.Sprintf("window.__infuseCursor && __infuseCursor.move(%f,%f)", x, y))
}

func (p *Page) click(ctx context.Context, g gestureStyle, x, y float64, button string, clicks int) {
	p.moveTo(ctx, g, x, y, 0)
	sleepCtx(ctx, g.preClick)
	_, _ = p.b.eval(ctx, fmt.Sprintf("window.__infuseCursor && __infuseCursor.click(%f,%f)", x, y))
	for c := 1; c <= clicks; c++ {
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": x, "y": y, "button": button, "buttons": 1, "clickCount": c})
		sleepCtx(ctx, g.press)
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": x, "y": y, "button": button, "buttons": 0, "clickCount": c})
		if c < clicks {
			sleepCtx(ctx, 60*time.Millisecond)
		}
	}
}

// keyInfo describes a key for DevTools: its key, code and Windows key code.
func keyInfo(k string) (key, code string, vk int) {
	named := map[string][2]any{"Enter": {"Enter", 13}, "Tab": {"Tab", 9}, "Escape": {"Escape", 27}, "Backspace": {"Backspace", 8}, "Delete": {"Delete", 46}, "ArrowDown": {"ArrowDown", 40}, "ArrowUp": {"ArrowUp", 38}, "ArrowLeft": {"ArrowLeft", 37}, "ArrowRight": {"ArrowRight", 39}, "Home": {"Home", 36}, "End": {"End", 35}, "PageDown": {"PageDown", 34}, "PageUp": {"PageUp", 33}, "Space": {"Space", 32}, " ": {"Space", 32}, "F5": {"F5", 116}, "F12": {"F12", 123}}
	if v, ok := named[k]; ok {
		if k == "Space" || k == " " {
			return " ", "Space", 32
		}
		return k, v[0].(string), v[1].(int)
	}
	if len(k) == 1 {
		c := k[0]
		switch {
		case c >= 'a' && c <= 'z':
			return k, "Key" + strings.ToUpper(k), int(c - 32)
		case c >= 'A' && c <= 'Z':
			return k, "Key" + k, int(c)
		case c >= '0' && c <= '9':
			return k, "Digit" + k, int(c)
		}
		return k, "", 0
	}
	return k, k, 0
}

// press sends a key or a combination like "Control+A" or "Shift+Tab", as a browser receives it.
func (p *Page) press(ctx context.Context, combo string) error {
	parts := strings.Split(combo, "+")
	if strings.HasSuffix(combo, "++") { // "Control++"
		parts = append(parts[:len(parts)-2], "+")
	}
	mods := 0
	modBits := map[string]int{"alt": 1, "control": 2, "ctrl": 2, "meta": 4, "cmd": 4, "shift": 8}
	var keyName string
	for _, part := range parts {
		if bit, ok := modBits[strings.ToLower(part)]; ok {
			mods |= bit
		} else {
			keyName = part
		}
	}
	if keyName == "" {
		return core.ErrMsg(core.M("browser.no_key", "combo", combo))
	}
	key, code, vk := keyInfo(keyName)
	down := map[string]any{"type": "keyDown", "key": key, "code": code, "windowsVirtualKeyCode": vk, "nativeVirtualKeyCode": vk, "modifiers": mods}
	if mods&(2|4|1) == 0 { // text only without Ctrl/Alt/Meta
		switch {
		case key == "Enter":
			down["text"] = "\r"
		case len([]rune(key)) == 1:
			txt := key
			if mods&8 != 0 {
				txt = strings.ToUpper(txt)
			}
			down["text"] = txt
		}
	}
	if mods&2 != 0 && strings.EqualFold(key, "a") {
		down["commands"] = []string{"selectAll"}
	}
	if _, err := p.b.call(ctx, "Input.dispatchKeyEvent", down); err != nil {
		return err
	}
	_, err := p.b.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key, "code": code, "windowsVirtualKeyCode": vk, "nativeVirtualKeyCode": vk, "modifiers": mods})
	return err
}

// typeText types like a person: key by key, at an uneven rhythm (accented characters as text input).
func (p *Page) typeText(ctx context.Context, g gestureStyle, text string) {
	if g.keyMax == 0 {
		_, _ = p.b.call(ctx, "Input.insertText", map[string]any{"text": text})
		return
	}
	for _, r := range text {
		ch := string(r)
		key, code, vk := keyInfo(ch)
		if r == '\n' {
			_ = p.press(ctx, "Enter")
		} else if vk != 0 || r == ' ' || (r < 128 && r > 31) {
			down := map[string]any{"type": "keyDown", "key": key, "code": code, "windowsVirtualKeyCode": vk, "text": ch, "unmodifiedText": ch}
			_, _ = p.b.call(ctx, "Input.dispatchKeyEvent", down)
			_, _ = p.b.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key, "code": code, "windowsVirtualKeyCode": vk})
		} else {
			_, _ = p.b.call(ctx, "Input.insertText", map[string]any{"text": ch})
		}
		d := g.keyMin + time.Duration(rand.Int63n(int64(g.keyMax-g.keyMin)+1))
		if r == ' ' || r == '.' || r == ',' {
			d += g.keyMin // people pause between words
		}
		sleepCtx(ctx, d)
	}
}

// scroll turns the wheel in steps at the cursor (or the page centre).
func (p *Page) scroll(ctx context.Context, g gestureStyle, dy float64) {
	p.mu.Lock()
	x, y := p.x, p.y
	if x < 0 {
		x, y = float64(p.width)/2, float64(p.height)/2
	}
	p.mu.Unlock()
	steps := int(math.Max(1, math.Abs(dy)/120))
	if g.stepDelay == 0 {
		steps = 1
	}
	for i := 0; i < steps; i++ {
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": dy / float64(steps)})
		sleepCtx(ctx, g.stepDelay*5)
	}
}

// ---------------------------------------------------------------- Playwright-style locators

// locatorJS finds an element (role=, text=, label=, placeholder=, testid=, xpath= or CSS), scrolls it
// into view and returns its centre, tag, text and whether it can be interacted with.
