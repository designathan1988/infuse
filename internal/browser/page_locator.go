package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"time"
)

// Playwright-style locators and visibility waits.

func locatorJS(sel string) string {
	q, _ := json.Marshal(sel)
	return fmt.Sprintf(`(() => { const sel = %s;
  const vis = e => { const r = e.getBoundingClientRect(), cs = getComputedStyle(e); return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none' && cs.opacity !== '0'; };
  const name = e => (e.getAttribute('aria-label') || (e.labels && e.labels[0] && e.labels[0].innerText) || e.innerText || e.value || e.getAttribute('alt') || e.getAttribute('title') || e.getAttribute('placeholder') || '').trim().replace(/\s+/g, ' ');
  const pick = (list, want) => { list = list.filter(vis); if (!want) return list[0]; const w = want.toLowerCase(); return list.find(e => name(e).toLowerCase() === w) || list.filter(e => name(e).toLowerCase().includes(w)).sort((a, b) => name(a).length - name(b).length)[0]; };
  const roles = { button: 'button,[role=button],input[type=submit],input[type=button],input[type=reset]', link: 'a[href],[role=link]', textbox: 'input:not([type]),input[type=text],input[type=email],input[type=password],input[type=search],input[type=tel],input[type=url],input[type=number],textarea,[role=textbox],[contenteditable=true]', checkbox: 'input[type=checkbox],[role=checkbox]', radio: 'input[type=radio],[role=radio]', heading: 'h1,h2,h3,h4,h5,h6,[role=heading]', combobox: 'select,[role=combobox]', option: 'option,[role=option]', tab: '[role=tab]', menuitem: '[role=menuitem]', img: 'img,[role=img]', dialog: 'dialog,[role=dialog]', listitem: 'li,[role=listitem]', navigation: 'nav,[role=navigation]', switch: '[role=switch]', row: 'tr,[role=row]', cell: 'td,[role=cell]' };
  let el = null, m;
  if (sel.startsWith('text=')) el = pick([...document.querySelectorAll('a,button,[role],label,input[type=submit],input[type=button],summary,li,td,th,span,p,div,h1,h2,h3,h4,h5,h6,strong,em')], sel.slice(5).replace(/^["']|["']$/g, ''));
  else if ((m = sel.match(/^role=([\w-]+)(?:\[name=["']?(.*?)["']?\])?$/))) el = pick([...document.querySelectorAll(roles[m[1]] || '[role=' + m[1] + ']')], m[2]);
  else if (sel.startsWith('label=')) { const w = sel.slice(6).replace(/^["']|["']$/g, '').toLowerCase(); const lab = [...document.querySelectorAll('label')].find(l => l.innerText.trim().toLowerCase().includes(w)); el = lab ? (lab.control || lab.querySelector('input,select,textarea')) : document.querySelector('[aria-label*="' + CSS.escape(w) + '" i]'); }
  else if (sel.startsWith('placeholder=')) el = pick([...document.querySelectorAll('[placeholder]')].filter(e => e.placeholder.toLowerCase().includes(sel.slice(12).replace(/^["']|["']$/g, '').toLowerCase())));
  else if (sel.startsWith('testid=')) el = document.querySelector('[data-testid="' + CSS.escape(sel.slice(7).replace(/^["']|["']$/g, '')) + '"]');
  else if (sel.startsWith('xpath=') || sel.startsWith('//')) el = document.evaluate(sel.replace(/^xpath=/, ''), document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
  else el = document.querySelector(sel);
  if (!el) return null;
  window.__infuseEl = el;
  el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' }); const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height, px: r.left + scrollX, py: r.top + scrollY, tag: el.tagName.toLowerCase(), text: name(el).slice(0, 80), visible: vis(el), disabled: !!el.disabled }; })()`, q)
}

type located struct {
	X, Y, W, H float64
	PX, PY     float64 // top-left in page coordinates (for clipped captures)
	Tag, Text  string
	Visible    bool
	Disabled   bool
}

// locate finds an element, waiting up to timeout for it to appear and become visible.
func (p *Page) locate(ctx context.Context, sel string, timeout time.Duration) (*located, error) {
	deadline := time.Now().Add(timeout)
	for {
		v, err := p.b.eval(ctx, locatorJS(sel))
		if err != nil {
			return nil, err
		}
		var l *located
		_ = json.Unmarshal(v, &l)
		if l != nil && l.Visible {
			return l, nil
		}
		if time.Now().After(deadline) {
			if l != nil {
				return nil, core.ErrMsg(core.M("browser.not_visible", "selector", sel))
			}
			return nil, core.ErrMsg(core.M("browser.not_found", "selector", sel))
		}
		sleepCtx(ctx, 150*time.Millisecond)
	}
}

// ---------------------------------------------------------------- recording (for the model, not for watching)

// A recording becomes what a model can analyse: the keyframes where the screen really changed, each a
// separate sharp image, with a timeline that ties every keyframe to the actions, console messages and
// failed requests around it, and a zoomed crop of the changed region when the change is small.
