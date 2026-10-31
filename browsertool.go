package main

// The agent's Browser tool, on the engine in agentbrowser.go: navigation, human gestures with a visible
// cursor, Playwright-style locators and waits, high-resolution captures, recordings delivered as
// keyframes with a timeline, and DevTools diagnostics. Captures go to .infuse/shots, recordings to
// .infuse/recordings; the images the model needs are attached to the tool result.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type browserTool struct{}

func (browserTool) Category() string { return "net" }
func (browserTool) Spec() ToolSpec {
	return ToolSpec{Name: "Browser", Description: `A real browser (without a window) that you use the way a person does: the cursor appears and moves, clicks have a pause, typing is key by key. Use it to open the app (localhost), check screens, reproduce bugs and diagnose.
Locators (selector): CSS, text=Sign in, role=button[name="Sign in"], label=Email, placeholder=Search, testid=save, xpath=//...
Actions: navigate(url) · back · reload · click(selector; button=right; count=2 for a double click) · hover(selector) · type(selector, text, submit) · press(key: Enter, Tab, Control+A, Shift+Tab…) · scroll(dy or selector) · drag(selector, to) · select(selector, value) · upload(selector, path in the project) · wait_for(selector or text, timeout ms) · text(optional selector) · dom(selector: HTML) · eval(script) ·
screenshot(full_page, selector for just that element) — you see the image in high resolution ·
record_start / record_stop — for something that happens over time (an animation, a transition, loading, an intermittent error): you receive the keyframes where the screen changed, each one as an image, with the timeline of actions, console errors and network failures between them, and an enlarged crop of small changes ·
console · network(only_issues) · performance · a11y (accessibility tree) · storage (cookies and localStorage) · throttle(network: offline | 3g | 4g | none) · emulate(device: iphone | pixel | ipad | laptop | desktop) · viewport(width, height) · pdf.`,
		InputSchema: schema(`{"type":"object","properties":{"action":{"type":"string","enum":["navigate","back","reload","click","hover","type","press","scroll","drag","select","upload","wait_for","text","dom","eval","screenshot","record_start","record_stop","console","network","performance","a11y","storage","throttle","emulate","viewport","pdf"]},"url":{"type":"string"},"selector":{"type":"string"},"to":{"type":"string","description":"drag: the target's locator"},"text":{"type":"string"},"value":{"type":"string"},"path":{"type":"string"},"submit":{"type":"boolean"},"key":{"type":"string"},"button":{"type":"string","enum":["left","right","middle"]},"count":{"type":"integer"},"dy":{"type":"number"},"script":{"type":"string"},"full_page":{"type":"boolean"},"timeout":{"type":"integer"},"max_frames":{"type":"integer"},"only_issues":{"type":"boolean"},"network":{"type":"string"},"device":{"type":"string"},"width":{"type":"integer"},"height":{"type":"integer"}},"required":["action"]}`)}
}

// attach hands an image to the model, at most 1600 px wide (the file on disk keeps full resolution).
func attachImage(s *Session, jpg []byte) {
	if im, err := jpeg.Decode(bytes.NewReader(jpg)); err == nil && im.Bounds().Dx() > 1600 {
		var buf bytes.Buffer
		_ = jpeg.Encode(&buf, shrink(im, 1600), &jpeg.Options{Quality: 88})
		jpg = buf.Bytes()
	}
	s.mu.Lock()
	s.pendingImages = append(s.pendingImages, Block{Type: "image", MediaType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(jpg)})
	s.mu.Unlock()
}

// shrink resizes to the given width with area averaging (text stays readable).
func shrink(src image.Image, width int) image.Image {
	b := src.Bounds()
	if b.Dx() <= width {
		return src
	}
	height := b.Dy() * width / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	fx, fy := float64(b.Dx())/float64(width), float64(b.Dy())/float64(height)
	for y := 0; y < height; y++ {
		y0, y1 := int(float64(y)*fy), max(int(float64(y+1)*fy), int(float64(y)*fy)+1)
		for x := 0; x < width; x++ {
			x0, x1 := int(float64(x)*fx), max(int(float64(x+1)*fx), int(float64(x)*fx)+1)
			var r, g, bl, n uint32
			for yy := y0; yy < min(y1, b.Dy()); yy++ {
				for xx := x0; xx < min(x1, b.Dx()); xx++ {
					cr, cg, cb, _ := src.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					r, g, bl, n = r+cr, g+cg, bl+cb, n+1
				}
			}
			if n > 0 {
				dst.Set(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
			}
		}
	}
	return dst
}

func (browserTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Action, URL, Selector, To, Text, Value, Path, Key, Button, Script, Network, Device string
		Submit, FullPage, OnlyIssues                                                       bool
		Count, Timeout, MaxFrames, Width, Height                                           int
		Dy                                                                                 float64
	}
	var raw map[string]any
	_ = json.Unmarshal(input, &raw)
	_ = json.Unmarshal(input, &in)
	in.FullPage, _ = raw["full_page"].(bool)
	in.OnlyIssues, _ = raw["only_issues"].(bool)
	if v, ok := raw["max_frames"].(float64); ok {
		in.MaxFrames = int(v)
	}
	s := tc.S
	p, err := agentPageFor(s)
	if err != nil {
		return err.Error(), true
	}
	g := s.settings.gestures()
	wait := time.Duration(in.Timeout) * time.Millisecond
	if wait <= 0 {
		wait = 8 * time.Second
	}
	where := func() string {
		v, _ := p.b.eval(ctx, `location.href + ' — ' + document.title`)
		var w string
		_ = json.Unmarshal(v, &w)
		return w
	}
	locate := func(sel string) (*located, string) {
		if sel == "" {
			return nil, "give a selector"
		}
		l, err := p.locate(ctx, sel, wait)
		if err != nil {
			return nil, err.Error()
		}
		return l, ""
	}
	if !strings.HasPrefix(in.Action, "record_") {
		what := firstNonEmpty(firstNonEmpty(in.Selector, in.URL), firstNonEmpty(in.Key, in.Text))
		if in.Action == "scroll" && in.Selector == "" {
			what = fmt.Sprintf("%+.0f px", firstNonZero(in.Dy, 600))
		}
		p.note(strings.TrimSpace("action: " + in.Action + " " + what))
	}
	switch in.Action {
	case "navigate":
		u := strings.TrimSpace(in.URL)
		if u == "" {
			return "give a url", true
		}
		if !strings.Contains(u, "://") && !strings.HasPrefix(u, "about:") {
			if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.") {
				u = "http://" + u
			} else {
				u = "https://" + u
			}
		}
		Journal(tc.Root, JournalEntry{Kind: "fetch", Command: u, Detail: "navegador do agente"})
		raw, err := p.b.call(ctx, "Page.navigate", map[string]any{"url": u})
		if err != nil {
			return "did not open: " + err.Error(), true
		}
		var nav struct {
			ErrorText string `json:"errorText"`
		}
		_ = json.Unmarshal(raw, &nav)
		if nav.ErrorText != "" {
			return fmt.Sprintf("did not open %s: %s", u, nav.ErrorText), true
		}
		p.b.waitLoad(ctx)
		return "opened: " + where(), false
	case "back", "reload":
		if in.Action == "back" {
			_, _ = p.b.eval(ctx, "history.back()")
		} else {
			_, _ = p.b.call(ctx, "Page.reload", nil)
		}
		sleepCtx(ctx, 400*time.Millisecond)
		p.b.waitLoad(ctx)
		return in.Action + ": " + where(), false
	case "click", "hover":
		l, why := locate(in.Selector)
		if l == nil {
			return why, true
		}
		if in.Action == "hover" {
			p.moveTo(ctx, g, l.X, l.Y, 0)
			sleepCtx(ctx, 250*time.Millisecond)
			return fmt.Sprintf("cursor over <%s> %q", l.Tag, l.Text), false
		}
		if l.Disabled {
			return fmt.Sprintf("<%s> %q is disabled", l.Tag, l.Text), true
		}
		count := max(1, in.Count)
		p.click(ctx, g, l.X, l.Y, firstNonEmpty(in.Button, "left"), count)
		sleepCtx(ctx, 300*time.Millisecond)
		p.b.waitLoad(ctx)
		return fmt.Sprintf("clicked <%s> %q · now at %s", l.Tag, l.Text, where()), false
	case "type":
		l, why := locate(in.Selector)
		if l == nil {
			return why, true
		}
		p.click(ctx, g, l.X, l.Y, "left", 1)
		_ = p.press(ctx, "Control+A") // replace what the field has, as a person selecting it would
		p.typeText(ctx, g, in.Text)
		if in.Submit {
			_ = p.press(ctx, "Enter")
			sleepCtx(ctx, 300*time.Millisecond)
			p.b.waitLoad(ctx)
		}
		return fmt.Sprintf("typed into %s · %s", in.Selector, where()), false
	case "press":
		if err := p.press(ctx, in.Key); err != nil {
			return err.Error(), true
		}
		sleepCtx(ctx, 200*time.Millisecond)
		return fmt.Sprintf("key %s · %s", in.Key, where()), false
	case "scroll":
		if in.Selector != "" {
			l, why := locate(in.Selector)
			if l == nil {
				return why, true
			}
			p.moveTo(ctx, g, l.X, l.Y, 0)
			return fmt.Sprintf("scrolled to <%s> %q", l.Tag, l.Text), false
		}
		dy := in.Dy
		if dy == 0 {
			dy = 600
		}
		p.scroll(ctx, g, dy)
		v, _ := p.b.eval(ctx, `Math.round(scrollY) + ' of ' + Math.round(document.documentElement.scrollHeight - innerHeight)`)
		return "scroll position: " + strings.Trim(string(v), `"`), false
	case "drag":
		from, why := locate(in.Selector)
		if from == nil {
			return why, true
		}
		to, why := locate(in.To)
		if to == nil {
			return why, true
		}
		p.moveTo(ctx, g, from.X, from.Y, 0)
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": from.X, "y": from.Y, "button": "left", "buttons": 1, "clickCount": 1})
		sleepCtx(ctx, g.press)
		p.moveTo(ctx, g, to.X, to.Y, 1)
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": to.X, "y": to.Y, "button": "left", "buttons": 0, "clickCount": 1})
		return fmt.Sprintf("dragged %q to %q", from.Text, to.Text), false
	case "select":
		l, why := locate(in.Selector)
		if l == nil {
			return why, true
		}
		p.click(ctx, g, l.X, l.Y, "left", 1)
		val, _ := json.Marshal(in.Value)
		v, err := p.b.eval(ctx, fmt.Sprintf(`(() => { const el = window.__infuseEl; const s = el && el.tagName === 'SELECT' ? el : el && el.querySelector && el.querySelector('select'); if (!s) return 'not a select'; const want = %s; const o = [...s.options].find(o => o.value === want || o.text.trim() === want); if (!o) return 'option not found: ' + [...s.options].map(o => o.text.trim()).join(' | '); s.value = o.value; s.dispatchEvent(new Event('input', {bubbles: true})); s.dispatchEvent(new Event('change', {bubbles: true})); return 'selected: ' + o.text.trim(); })()`, val))
		if err != nil {
			return err.Error(), true
		}
		_ = p.press(ctx, "Escape")
		res := strings.Trim(string(v), `"`)
		return res, !strings.HasPrefix(res, "selected: ")
	case "upload":
		abs := filepath.Join(tc.Root, filepath.FromSlash(in.Path))
		if !within(tc.Root, abs) || !exists(abs) {
			return "the file must exist inside the project: " + in.Path, true
		}
		if l, why := locate(in.Selector); l == nil {
			return why, true
		}
		r, err := p.b.call(ctx, "Runtime.evaluate", map[string]any{"expression": "window.__infuseEl"})
		if err != nil {
			return err.Error(), true
		}
		var obj struct {
			Result struct {
				ObjectID string `json:"objectId"`
			} `json:"result"`
		}
		_ = json.Unmarshal(r, &obj)
		if obj.Result.ObjectID == "" {
			return "file input not found: " + in.Selector, true
		}
		if _, err := p.b.call(ctx, "DOM.setFileInputFiles", map[string]any{"files": []string{abs}, "objectId": obj.Result.ObjectID}); err != nil {
			return err.Error(), true
		}
		return "file attached to the input: " + in.Path, false
	case "wait_for":
		target := firstNonEmpty(in.Selector, "text="+in.Text)
		start := time.Now()
		l, why := locate(target)
		if l == nil {
			return fmt.Sprintf("waited %d ms: %s", time.Since(start).Milliseconds(), why), true
		}
		return fmt.Sprintf("appeared after %d ms: <%s> %q", time.Since(start).Milliseconds(), l.Tag, l.Text), false
	case "text", "dom":
		js := "document.body ? document.body.innerText : ''"
		if in.Action == "dom" {
			js = "document.documentElement.outerHTML"
		}
		if in.Selector != "" {
			l, why := locate(in.Selector)
			if l == nil {
				return why, true
			}
			q, _ := json.Marshal(in.Selector)
			prop := "innerText"
			if in.Action == "dom" {
				prop = "outerHTML"
			}
			_ = q
			js = "window.__infuseEl ? window.__infuseEl." + prop + " : ''"
		}
		v, err := p.b.eval(ctx, js)
		if err != nil {
			return err.Error(), true
		}
		var str string
		_ = json.Unmarshal(v, &str)
		return clip(externalContent(where(), str)), false
	case "eval":
		v, err := p.b.eval(ctx, in.Script)
		if err != nil {
			return "script error: " + err.Error(), true
		}
		return clip(externalContent(where(), string(v))), false
	case "screenshot":
		params := map[string]any{"format": "jpeg", "quality": 92, "captureBeyondViewport": in.FullPage}
		what := ""
		if in.Selector != "" {
			l, why := locate(in.Selector)
			if l == nil {
				return why, true
			}
			params["clip"] = map[string]any{"x": math.Max(0, l.PX-8), "y": math.Max(0, l.PY-8), "width": l.W + 16, "height": l.H + 16, "scale": 1}
			what = fmt.Sprintf("<%s> %q", l.Tag, l.Text)
		}
		raw, err := p.b.call(ctx, "Page.captureScreenshot", params)
		if err != nil {
			return err.Error(), true
		}
		var shot struct {
			Data string `json:"data"`
		}
		_ = json.Unmarshal(raw, &shot)
		jpg, err := base64.StdEncoding.DecodeString(shot.Data)
		if err != nil || len(jpg) == 0 {
			return "empty capture", true
		}
		name := time.Now().Format("20060102-150405.000") + ".jpg"
		dir := filepath.Join(s.Root, infuseDirName, "shots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name), jpg, 0o644)
		cfg, _ := jpeg.DecodeConfig(bytes.NewReader(jpg))
		attachImage(s, jpg)
		if what == "" {
			return fmt.Sprintf("capture of the screen (%dx%d px) attached for you to see · %s · .infuse/shots/%s", cfg.Width, cfg.Height, where(), name), false
		}
		return fmt.Sprintf("capture of the element %s (%dx%d px) attached for you to see · %s · .infuse/shots/%s", what, cfg.Width, cfg.Height, where(), name), false
	case "record_start":
		p.startRecording()
		return "recording: perform the actions, then call record_stop. You will receive the frames where the screen changed, with the timeline.", false
	case "record_stop":
		dir := filepath.Join(s.Root, infuseDirName, "recordings", time.Now().Format("20060102-150405"))
		kfs, events, dur, total, err := p.stopRecording(dir, in.MaxFrames)
		if err != nil {
			return err.Error(), true
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Recording of %.1f s: %d frames captured, %d keyframes (where the screen changed), attached in order. Changes caused only by the agent's cursor do not count. Coordinates in page px (%dx%d). Files: %s\n\n", dur.Seconds(), total, len(kfs), p.width, p.height, relTo(s.Root, dir))
		ev := 0
		img := 0
		for i, k := range kfs {
			for ev < len(events) && events[ev].at <= k.at {
				fmt.Fprintf(&b, "    t=%.2fs · %s\n", events[ev].at.Seconds(), events[ev].text)
				ev++
			}
			img++
			attachImage(s, k.jpeg)
			line := fmt.Sprintf("Image %d · frame %d · t=%.2fs", img, i+1, k.at.Seconds())
			switch {
			case i == 0:
				line += " · start"
			case len(k.regions) == 0:
				line += " · final state (no change since the previous frame)"
			default:
				var where []string
				for _, r := range k.regions {
					where = append(where, fmt.Sprintf("x=%d y=%d %dx%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy()))
				}
				line += fmt.Sprintf(" · %.1f%% of the screen changed, in %d area(s): %s", k.changed*100, len(k.regions), strings.Join(where, "; "))
			}
			b.WriteString(line + "\n")
			for ci, crop := range k.crops {
				img++
				attachImage(s, crop)
				fmt.Fprintf(&b, "Image %d · enlarged crop of area %d of frame %d\n", img, ci+1, i+1)
			}
		}
		for ; ev < len(events); ev++ {
			fmt.Fprintf(&b, "    t=%.2fs · %s\n", events[ev].at.Seconds(), events[ev].text)
		}
		return b.String(), false
	case "console":
		p.b.mu.Lock()
		lines := p.b.console
		p.b.console = nil
		p.b.mu.Unlock()
		if len(lines) == 0 {
			return "no console messages since the last check", false
		}
		return clip(externalContent(where(), strings.Join(lines, "\n"))), false
	case "network":
		return clip(p.networkReport(in.OnlyIssues)), false
	case "performance":
		raw, _ := p.b.call(ctx, "Performance.getMetrics", nil)
		var m struct {
			Metrics []struct {
				Name  string  `json:"name"`
				Value float64 `json:"value"`
			} `json:"metrics"`
		}
		_ = json.Unmarshal(raw, &m)
		var b strings.Builder
		want := map[string]bool{"Documents": true, "Nodes": true, "JSEventListeners": true, "LayoutCount": true, "RecalcStyleCount": true, "LayoutDuration": true, "ScriptDuration": true, "TaskDuration": true, "JSHeapUsedSize": true, "JSHeapTotalSize": true}
		for _, x := range m.Metrics {
			if want[x.Name] {
				fmt.Fprintf(&b, "%s: %v\n", x.Name, math.Round(x.Value*1000)/1000)
			}
		}
		v, _ := p.b.eval(ctx, `JSON.stringify((() => { const n = performance.getEntriesByType('navigation')[0] || {}; const paint = Object.fromEntries(performance.getEntriesByType('paint').map(p => [p.name, Math.round(p.startTime)])); const lcp = performance.getEntriesByType('largest-contentful-paint').pop(); return { ttfb: Math.round(n.responseStart || 0), domContentLoaded: Math.round(n.domContentLoadedEventEnd || 0), load: Math.round(n.loadEventEnd || 0), ...paint, lcp: lcp ? Math.round(lcp.startTime) : null, resources: performance.getEntriesByType('resource').length }; })())`)
		var nav string
		_ = json.Unmarshal(v, &nav)
		b.WriteString("timings (ms): " + nav)
		return b.String(), false
	case "a11y":
		return clip(p.axTree(ctx)), false
	case "storage":
		v, _ := p.b.eval(ctx, `JSON.stringify({ cookies: document.cookie, localStorage: Object.fromEntries(Object.entries(localStorage)), sessionStorage: Object.fromEntries(Object.entries(sessionStorage)) }, null, 1)`)
		var str string
		_ = json.Unmarshal(v, &str)
		return clip(externalContent(where(), redactSecrets(str))), false
	case "throttle":
		presets := map[string][4]float64{"offline": {1, 0, 0, 0}, "3g": {0, 300, 750 * 1024 / 8, 250 * 1024 / 8}, "4g": {0, 60, 4 * 1024 * 1024 / 8, 3 * 1024 * 1024 / 8}, "none": {0, 0, -1, -1}}
		pr, ok := presets[in.Network]
		if !ok {
			return "network: offline | 3g | 4g | none", true
		}
		_, err := p.b.call(ctx, "Network.emulateNetworkConditions", map[string]any{"offline": pr[0] == 1, "latency": pr[1], "downloadThroughput": pr[2], "uploadThroughput": pr[3]})
		if err != nil {
			return err.Error(), true
		}
		return "network: " + in.Network, false
	case "emulate":
		if in.Device == "desktop" {
			w, h, sc := s.settings.browserViewport()
			p.mu.Lock()
			p.device = ""
			p.mu.Unlock()
			p.setMetrics(w, h, sc, false)
			_, _ = p.b.call(ctx, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": false})
			p.b.prepare()
			return fmt.Sprintf("desktop %dx%d (%.0fx)", w, h, sc), false
		}
		d, ok := devicePresets[in.Device]
		if !ok {
			return "device: " + strings.Join(sortedKeys(devicePresets), " | ") + " | desktop", true
		}
		p.mu.Lock()
		p.device = in.Device
		p.mu.Unlock()
		p.setMetrics(d.w, d.h, d.scale, d.mobile)
		_, _ = p.b.call(ctx, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": d.mobile, "maxTouchPoints": 5})
		if d.ua != "" {
			_, _ = p.b.call(ctx, "Network.setUserAgentOverride", map[string]any{"userAgent": d.ua})
		}
		_, _ = p.b.call(ctx, "Page.reload", nil)
		p.b.waitLoad(ctx)
		res := fmt.Sprintf(map[bool]string{true: "emulating %s: %dx%d at density %.2f (touch)", false: "emulating %s: %dx%d at density %.2f (no touch)"}[d.mobile], in.Device, d.w, d.h, d.scale)
		if v, err := p.b.eval(ctx, `innerWidth`); err == nil && d.mobile && strings.TrimSpace(string(v)) != fmt.Sprint(d.w) {
			res += fmt.Sprintf(". Warning: the page is laid out %s px wide and scaled down to fit in %d px — <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"> is missing, or something is wider than the screen", strings.TrimSpace(string(v)), d.w)
		}
		return res, false
	case "viewport":
		if in.Width <= 0 || in.Height <= 0 {
			return "give width and height", true
		}
		_, _, sc := s.settings.browserViewport()
		p.setMetrics(in.Width, in.Height, sc, in.Width < 768)
		return fmt.Sprintf("screen of %dx%d", in.Width, in.Height), false
	case "pdf":
		raw, err := p.b.call(ctx, "Page.printToPDF", map[string]any{"printBackground": true})
		if err != nil {
			return err.Error(), true
		}
		var r struct {
			Data string `json:"data"`
		}
		_ = json.Unmarshal(raw, &r)
		b, _ := base64.StdEncoding.DecodeString(r.Data)
		name := time.Now().Format("20060102-150405") + ".pdf"
		dir := filepath.Join(s.Root, infuseDirName, "shots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
		return fmt.Sprintf("PDF saved: .infuse/shots/%s (%d KB)", name, len(b)/1024), false
	}
	return "unknown action: " + in.Action, true
}

func firstNonZero(a, b float64) float64 {
	if a != 0 {
		return a
	}
	return b
}

func init() { registerTool(browserTool{}) }
