package browser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/security"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Diagnostics and emulation: console, network, performance, a11y, storage, throttling, devices, PDF.

func (a *action) inspect() (string, bool) {
	switch a.in.Action {
	case "console":
		a.p.b.mu.Lock()
		lines := a.p.b.console
		a.p.b.console = nil
		a.p.b.mu.Unlock()
		if len(lines) == 0 {
			return a.h.Say(core.M("browser.no_console")), false
		}
		return a.h.Clip(security.ExternalContent(a.where(), strings.Join(lines, "\n"))), false
	case "network":
		lines, n := a.p.networkReport(a.in.OnlyIssues)
		if n == 0 {
			return a.h.Say(core.M("browser.no_requests")), false
		}
		return a.h.Say(core.M("browser.requests", "count", n)) + "\n" + a.h.Clip(lines), false
	case "performance":
		raw, _ := a.p.b.call(a.ctx, "Performance.getMetrics", nil)
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
		v, _ := a.p.b.eval(a.ctx, `JSON.stringify((() => { const n = performance.getEntriesByType('navigation')[0] || {}; const paint = Object.fromEntries(performance.getEntriesByType('paint').map(p => [a.p.name, Math.round(a.p.startTime)])); const lcp = performance.getEntriesByType('largest-contentful-paint').pop(); return { ttfb: Math.round(n.responseStart || 0), domContentLoaded: Math.round(n.domContentLoadedEventEnd || 0), load: Math.round(n.loadEventEnd || 0), ...paint, lcp: lcp ? Math.round(lcp.startTime) : null, resources: performance.getEntriesByType('resource').length }; })())`)
		var nav string
		_ = json.Unmarshal(v, &nav)
		b.WriteString(a.h.Say(core.M("browser.timings", "values", nav)))
		return b.String(), false
	case "a11y":
		return a.h.Clip(a.p.axTree(a.ctx)), false
	case "storage":
		v, _ := a.p.b.eval(a.ctx, `JSON.stringify({ cookies: document.cookie, localStorage: Object.fromEntries(Object.entries(localStorage)), sessionStorage: Object.fromEntries(Object.entries(sessionStorage)) }, null, 1)`)
		var str string
		_ = json.Unmarshal(v, &str)
		return a.h.Clip(security.ExternalContent(a.where(), security.RedactSecrets(str))), false
	case "throttle":
		presets := map[string][4]float64{"offline": {1, 0, 0, 0}, "3g": {0, 300, 750 * 1024 / 8, 250 * 1024 / 8}, "4g": {0, 60, 4 * 1024 * 1024 / 8, 3 * 1024 * 1024 / 8}, "none": {0, 0, -1, -1}}
		pr, ok := presets[a.in.Network]
		if !ok {
			return a.h.Say(core.M("browser.network_options")), true
		}
		_, err := a.p.b.call(a.ctx, "Network.emulateNetworkConditions", map[string]any{"offline": pr[0] == 1, "latency": pr[1], "downloadThroughput": pr[2], "uploadThroughput": pr[3]})
		if err != nil {
			return err.Error(), true
		}
		return a.h.Say(core.M("browser.network_set", "network", a.in.Network)), false
	case "emulate":
		if a.in.Device == "desktop" {
			w, vh, sc := browserViewport(a.h.Settings())
			a.p.mu.Lock()
			a.p.device = ""
			a.p.mu.Unlock()
			a.p.setMetrics(w, vh, sc, false)
			_, _ = a.p.b.call(a.ctx, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": false})
			a.p.b.prepare()
			return a.h.Say(core.M("browser.desktop", "w", w, "h", vh, "scale", fmt.Sprintf("%.0f", sc))), false
		}
		d, ok := devicePresets[a.in.Device]
		if !ok {
			return a.h.Say(core.M("browser.device_options", "devices", strings.Join(core.SortedKeys(devicePresets), " | ")+" | desktop")), true
		}
		a.p.mu.Lock()
		a.p.device = a.in.Device
		a.p.mu.Unlock()
		a.p.setMetrics(d.w, d.h, d.scale, d.mobile)
		_, _ = a.p.b.call(a.ctx, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": d.mobile, "maxTouchPoints": 5})
		if d.ua != "" {
			_, _ = a.p.b.call(a.ctx, "Network.setUserAgentOverride", map[string]any{"userAgent": d.ua})
		}
		_, _ = a.p.b.call(a.ctx, "Page.reload", nil)
		a.p.b.waitLoad(a.ctx)
		emu := core.M("browser.emulating", "device", a.in.Device, "w", d.w, "h", d.h, "density", fmt.Sprintf("%.2f", d.scale))
		if d.mobile {
			emu = core.M("browser.emulating_touch", "device", a.in.Device, "w", d.w, "h", d.h, "density", fmt.Sprintf("%.2f", d.scale))
		}
		res := a.h.Say(emu)
		if v, err := a.p.b.eval(a.ctx, `innerWidth`); err == nil && d.mobile && strings.TrimSpace(string(v)) != fmt.Sprint(d.w) {
			res += ". " + a.h.Say(core.M("browser.viewport_warning", "width", strings.TrimSpace(string(v)), "screen", d.w))
		}
		return res, false
	case "viewport":
		if a.in.Width <= 0 || a.in.Height <= 0 {
			return a.h.Say(core.M("browser.give_size")), true
		}
		_, _, sc := browserViewport(a.h.Settings())
		a.p.setMetrics(a.in.Width, a.in.Height, sc, a.in.Width < 768)
		return a.h.Say(core.M("browser.viewport", "w", a.in.Width, "h", a.in.Height)), false
	case "pdf":
		raw, err := a.p.b.call(a.ctx, "Page.printToPDF", map[string]any{"printBackground": true})
		if err != nil {
			return err.Error(), true
		}
		var r struct {
			Data string `json:"data"`
		}
		_ = json.Unmarshal(raw, &r)
		b, _ := base64.StdEncoding.DecodeString(r.Data)
		name := time.Now().Format("20060102-150405") + ".pdf"
		dir := filepath.Join(a.h.RootDir(), core.InfuseDirName, "shots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
		return a.h.Say(core.M("browser.pdf", "file", name, "kb", len(b)/1024)), false
	}
	return "", true
}
