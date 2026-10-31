package browser

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"infuse/internal/core"
	"infuse/internal/security"
	"math"
	"os"
	"path/filepath"
	"time"
)

// Page content and captures: text, DOM, eval, screenshots.

func (a *action) content() (string, bool) {
	switch a.in.Action {
	case "text", "dom":
		js := "document.body ? document.body.innerText : ''"
		if a.in.Action == "dom" {
			js = "document.documentElement.outerHTML"
		}
		if a.in.Selector != "" {
			l, why := a.locate(a.in.Selector)
			if l == nil {
				return why, true
			}
			q, _ := json.Marshal(a.in.Selector)
			prop := "innerText"
			if a.in.Action == "dom" {
				prop = "outerHTML"
			}
			_ = q
			js = "window.__infuseEl ? window.__infuseEl." + prop + " : ''"
		}
		v, err := a.p.b.eval(a.ctx, js)
		if err != nil {
			return err.Error(), true
		}
		var str string
		_ = json.Unmarshal(v, &str)
		return a.h.Clip(security.ExternalContent(a.where(), str)), false
	case "eval":
		v, err := a.p.b.eval(a.ctx, a.in.Script)
		if err != nil {
			return a.h.Say(core.M("browser.script_error", "error", err.Error())), true
		}
		return a.h.Clip(security.ExternalContent(a.where(), string(v))), false
	case "screenshot":
		params := map[string]any{"format": "jpeg", "quality": 92, "captureBeyondViewport": a.in.FullPage}
		what := ""
		if a.in.Selector != "" {
			l, why := a.locate(a.in.Selector)
			if l == nil {
				return why, true
			}
			params["clip"] = map[string]any{"x": math.Max(0, l.PX-8), "y": math.Max(0, l.PY-8), "width": l.W + 16, "height": l.H + 16, "scale": 1}
			what = fmt.Sprintf("<%s> %q", l.Tag, l.Text)
		}
		raw, err := a.p.b.call(a.ctx, "Page.captureScreenshot", params)
		if err != nil {
			return err.Error(), true
		}
		var shot struct {
			Data string `json:"data"`
		}
		_ = json.Unmarshal(raw, &shot)
		jpg, err := base64.StdEncoding.DecodeString(shot.Data)
		if err != nil || len(jpg) == 0 {
			return a.h.Say(core.M("browser.empty_capture")), true
		}
		name := time.Now().Format("20060102-150405.000") + ".jpg"
		dir := filepath.Join(a.h.RootDir(), core.InfuseDirName, "shots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name), jpg, 0o644)
		cfg, _ := jpeg.DecodeConfig(bytes.NewReader(jpg))
		attachImage(a.h, jpg)
		if what == "" {
			return a.h.Say(core.M("browser.shot_screen", "w", cfg.Width, "h", cfg.Height, "where", a.where(), "file", name)), false
		}
		return a.h.Say(core.M("browser.shot_element", "what", what, "w", cfg.Width, "h", cfg.Height, "where", a.where(), "file", name)), false
	}
	return "", true
}
