package browser

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"path/filepath"
	"time"
)

// Pointer and keyboard interaction: click, hover, type, press, scroll, drag, select, upload, waits.

func (a *action) interact() (string, bool) {
	switch a.in.Action {
	case "click", "hover":
		return a.interactClickOrHover()
	case "type":
		return a.interactType()
	case "press":
		return a.interactPress()
	case "scroll":
		return a.interactScroll()
	case "drag":
		return a.interactDrag()
	case "select":
		return a.interactSelect()
	case "upload":
		return a.interactUpload()
	case "wait_for":
		return a.interactWaitFor()
	}
	return "", true
}

func (a *action) interactClickOrHover() (string, bool) {
	l, why := a.locate(a.in.Selector)
	if l == nil {
		return why, true
	}
	if a.in.Action == "hover" {
		a.p.moveTo(a.ctx, a.g, l.X, l.Y, 0)
		sleepCtx(a.ctx, 250*time.Millisecond)
		return a.h.Say(core.M("browser.hover", "tag", l.Tag, "text", l.Text)), false
	}
	if l.Disabled {
		return a.h.Say(core.M("browser.disabled", "tag", l.Tag, "text", l.Text)), true
	}
	count := max(1, a.in.Count)
	a.p.click(a.ctx, a.g, l.X, l.Y, core.FirstNonEmpty(a.in.Button, "left"), count)
	sleepCtx(a.ctx, 300*time.Millisecond)
	a.p.b.waitLoad(a.ctx)
	return a.h.Say(core.M("browser.clicked", "tag", l.Tag, "text", l.Text, "where", a.where())), false
}

func (a *action) interactType() (string, bool) {
	l, why := a.locate(a.in.Selector)
	if l == nil {
		return why, true
	}
	a.p.click(a.ctx, a.g, l.X, l.Y, "left", 1)
	_ = a.p.press(a.ctx, "Control+A") // replace what the field has, as a person selecting it would
	a.p.typeText(a.ctx, a.g, a.in.Text)
	if a.in.Submit {
		_ = a.p.press(a.ctx, "Enter")
		sleepCtx(a.ctx, 300*time.Millisecond)
		a.p.b.waitLoad(a.ctx)
	}
	return a.h.Say(core.M("browser.typed", "selector", a.in.Selector, "where", a.where())), false
}

func (a *action) interactPress() (string, bool) {
	if err := a.p.press(a.ctx, a.in.Key); err != nil {
		return a.h.SayErr(err), true
	}
	sleepCtx(a.ctx, 200*time.Millisecond)
	return a.h.Say(core.M("browser.key", "key", a.in.Key, "where", a.where())), false
}

func (a *action) interactScroll() (string, bool) {
	if a.in.Selector != "" {
		l, why := a.locate(a.in.Selector)
		if l == nil {
			return why, true
		}
		a.p.moveTo(a.ctx, a.g, l.X, l.Y, 0)
		return a.h.Say(core.M("browser.scrolled_to", "tag", l.Tag, "text", l.Text)), false
	}
	dy := a.in.Dy
	if dy == 0 {
		dy = 600
	}
	a.p.scroll(a.ctx, a.g, dy)
	v, _ := a.p.b.eval(a.ctx, `[Math.round(scrollY), Math.round(document.documentElement.scrollHeight - innerHeight)]`)
	var pos [2]int
	_ = json.Unmarshal(v, &pos)
	return a.h.Say(core.M("browser.scroll_pos", "pos", pos[0], "max", pos[1])), false
}

func (a *action) interactDrag() (string, bool) {
	from, why := a.locate(a.in.Selector)
	if from == nil {
		return why, true
	}
	to, why := a.locate(a.in.To)
	if to == nil {
		return why, true
	}
	a.p.moveTo(a.ctx, a.g, from.X, from.Y, 0)
	_, _ = a.p.b.call(a.ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": from.X, "y": from.Y, "button": "left", "buttons": 1, "clickCount": 1})
	sleepCtx(a.ctx, a.g.press)
	a.p.moveTo(a.ctx, a.g, to.X, to.Y, 1)
	_, _ = a.p.b.call(a.ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": to.X, "y": to.Y, "button": "left", "buttons": 0, "clickCount": 1})
	return a.h.Say(core.M("browser.dragged", "from", from.Text, "to", to.Text)), false
}

func (a *action) interactSelect() (string, bool) {
	l, why := a.locate(a.in.Selector)
	if l == nil {
		return why, true
	}
	a.p.click(a.ctx, a.g, l.X, l.Y, "left", 1)
	val, _ := json.Marshal(a.in.Value)
	// the page answers with data (what happened, the option or the options there are), never a sentence
	v, err := a.p.b.eval(a.ctx, fmt.Sprintf(`(() => { const el = window.__infuseEl; const s = el && el.tagName === 'SELECT' ? el : el && el.querySelector && el.querySelector('select'); if (!s) return {st: 'not_select'}; const want = %s; const o = [...s.options].find(o => o.value === want || o.text.trim() === want); if (!o) return {st: 'no_option', text: [...s.options].map(o => o.text.trim()).join(' | ')}; s.value = o.value; s.dispatchEvent(new Event('input', {bubbles: true})); s.dispatchEvent(new Event('change', {bubbles: true})); return {st: 'ok', text: o.text.trim()}; })()`, val))
	if err != nil {
		return err.Error(), true
	}
	_ = a.p.press(a.ctx, "Escape")
	var sel struct{ St, Text string }
	_ = json.Unmarshal(v, &sel)
	switch sel.St {
	case "ok":
		return a.h.Say(core.M("browser.selected", "option", sel.Text)), false
	case "no_option":
		return a.h.Say(core.M("browser.no_option", "options", sel.Text)), true
	}
	return a.h.Say(core.M("browser.not_select")), true
}

func (a *action) interactUpload() (string, bool) {
	abs := filepath.Join(a.h.RootDir(), filepath.FromSlash(a.in.Path))
	if !core.Within(a.h.RootDir(), abs) || !core.Exists(abs) {
		return a.h.Say(core.M("browser.upload_outside", "path", a.in.Path)), true
	}
	if l, why := a.locate(a.in.Selector); l == nil {
		return why, true
	}
	r, err := a.p.b.call(a.ctx, "Runtime.evaluate", map[string]any{"expression": "window.__infuseEl"})
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
		return a.h.Say(core.M("browser.no_file_input", "selector", a.in.Selector)), true
	}
	if _, err := a.p.b.call(a.ctx, "DOM.setFileInputFiles", map[string]any{"files": []string{abs}, "objectId": obj.Result.ObjectID}); err != nil {
		return err.Error(), true
	}
	return a.h.Say(core.M("browser.uploaded", "path", a.in.Path)), false
}

func (a *action) interactWaitFor() (string, bool) {
	target := core.FirstNonEmpty(a.in.Selector, "text="+a.in.Text)
	start := time.Now()
	l, err := a.locateErr(target)
	if l == nil {
		return a.h.Say(core.M("browser.waited", "ms", time.Since(start).Milliseconds(), "why", core.ErrArg(err))), true
	}
	return a.h.Say(core.M("browser.appeared", "ms", time.Since(start).Milliseconds(), "tag", l.Tag, "text", l.Text)), false
}
