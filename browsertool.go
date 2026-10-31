package main

// The agent's Browser tool: the project's headless browser (cdp.go), to open the app it is building,
// read it, click, type and take screenshots it can see. Screenshots are kept in .infuse/shots and shown
// in the transcript.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type browserTool struct{}

func (browserTool) Category() string { return "net" }
func (browserTool) Spec() ToolSpec {
	return ToolSpec{Name: "Browser", Description: "Um navegador de verdade, sem janela, para abrir e conferir páginas (inclusive o app em localhost). action: navigate (url), screenshot (full_page opcional; você vê a imagem), text (selector opcional: lê o texto visível), click (selector CSS ou text=Texto do botão), type (selector, text, submit opcional), press (key: Enter, Tab, Escape, ArrowDown…), eval (script JS, devolve o valor), console (mensagens e erros da página desde a última consulta), viewport (width, height — ex.: 375x812 para celular), back. Use screenshot depois de mudanças visuais para conferir o resultado.",
		InputSchema: schema(`{"type":"object","properties":{"action":{"type":"string","enum":["navigate","screenshot","text","click","type","press","eval","console","viewport","back"]},"url":{"type":"string"},"selector":{"type":"string"},"text":{"type":"string"},"submit":{"type":"boolean"},"key":{"type":"string"},"script":{"type":"string"},"full_page":{"type":"boolean"},"width":{"type":"integer"},"height":{"type":"integer"}},"required":["action"]}`)}
}

var keyCodes = map[string]int{"Enter": 13, "Tab": 9, "Escape": 27, "Backspace": 8, "Delete": 46, "ArrowDown": 40, "ArrowUp": 38, "ArrowLeft": 37, "ArrowRight": 39, "Home": 36, "End": 35, "PageDown": 34, "PageUp": 33, "Space": 32}

// findElementJS locates a CSS selector or text=… and scrolls it into view; returns its center or null.
func findElementJS(sel string) string {
	q, _ := json.Marshal(sel)
	return fmt.Sprintf(`(() => { const s = %s; let el = null;
  if (s.startsWith('text=')) { const want = s.slice(5).trim().toLowerCase(); const all = [...document.querySelectorAll('button, a, [role=button], input[type=submit], label, [onclick], summary, li, span, div')];
    el = all.find(e => (e.innerText || e.value || '').trim().toLowerCase() === want) || all.find(e => (e.innerText || e.value || '').trim().toLowerCase().includes(want)); }
  else el = document.querySelector(s);
  if (!el) return null; el.scrollIntoView({ block: 'center', inline: 'center' }); const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2, y: r.top + r.height / 2, tag: el.tagName.toLowerCase(), text: (el.innerText || el.value || '').trim().slice(0, 80) }; })()`, q)
}

func (browserTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Action, URL, Selector, Text, Key, Script string
		Submit                                   bool
		FullPage                                 bool `json:"full_page"`
		Width, Height                            int
	}
	_ = json.Unmarshal(input, &in)
	b, err := agentBrowser(tc.S.Root)
	if err != nil {
		return err.Error(), true
	}
	where := func() string {
		v, _ := b.eval(ctx, `location.href + ' — ' + document.title`)
		var s string
		_ = json.Unmarshal(v, &s)
		return s
	}
	switch in.Action {
	case "navigate":
		u := strings.TrimSpace(in.URL)
		if u == "" {
			return "informe url", true
		}
		if !strings.Contains(u, "://") && !strings.HasPrefix(u, "about:") {
			if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.") {
				u = "http://" + u
			} else {
				u = "https://" + u
			}
		}
		Journal(tc.Root, JournalEntry{Kind: "fetch", Command: u, Detail: "navegador do agente"})
		raw, err := b.call(ctx, "Page.navigate", map[string]any{"url": u})
		if err != nil {
			return "não abriu: " + err.Error(), true
		}
		var nav struct {
			ErrorText string `json:"errorText"`
		}
		_ = json.Unmarshal(raw, &nav)
		if nav.ErrorText != "" {
			return "não abriu " + u + ": " + nav.ErrorText, true
		}
		b.waitLoad(ctx)
		return "aberto: " + where(), false
	case "back":
		_, _ = b.eval(ctx, "history.back()")
		time.Sleep(400 * time.Millisecond)
		b.waitLoad(ctx)
		return "voltou: " + where(), false
	case "screenshot":
		raw, err := b.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": in.FullPage})
		if err != nil {
			return err.Error(), true
		}
		var shot struct {
			Data string `json:"data"`
		}
		_ = json.Unmarshal(raw, &shot)
		png, err := base64.StdEncoding.DecodeString(shot.Data)
		if err != nil || len(png) == 0 {
			return "captura vazia", true
		}
		name := fmt.Sprintf("%s.png", time.Now().Format("20060102-150405.000"))
		dir := filepath.Join(tc.S.Root, infuseDirName, "shots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name), png, 0o644)
		if len(shot.Data) < 7<<20 {
			tc.S.mu.Lock()
			tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: "image/png", Data: shot.Data})
			tc.S.mu.Unlock()
		}
		return fmt.Sprintf("captura de %s anexada para você ver · .infuse/shots/%s", where(), name), false
	case "text":
		js := "document.body ? document.body.innerText : ''"
		if in.Selector != "" {
			q, _ := json.Marshal(in.Selector)
			js = fmt.Sprintf("(() => { const e = document.querySelector(%s); return e ? e.innerText : null })()", q)
		}
		v, err := b.eval(ctx, js)
		if err != nil {
			return err.Error(), true
		}
		var s *string
		_ = json.Unmarshal(v, &s)
		if s == nil {
			return "elemento não encontrado: " + in.Selector, true
		}
		return clip(externalContent(where(), *s)), false
	case "click":
		v, err := b.eval(ctx, findElementJS(in.Selector))
		if err != nil {
			return err.Error(), true
		}
		var pos *struct {
			X, Y      float64
			Tag, Text string
		}
		_ = json.Unmarshal(v, &pos)
		if pos == nil {
			return "elemento não encontrado: " + in.Selector, true
		}
		for _, t := range []string{"mousePressed", "mouseReleased"} {
			if _, err := b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": t, "x": pos.X, "y": pos.Y, "button": "left", "clickCount": 1}); err != nil {
				return err.Error(), true
			}
		}
		time.Sleep(300 * time.Millisecond)
		b.waitLoad(ctx)
		return fmt.Sprintf("clicou em <%s> %q · agora em %s", pos.Tag, pos.Text, where()), false
	case "type":
		q, _ := json.Marshal(in.Selector)
		v, err := b.eval(ctx, fmt.Sprintf("(() => { const e = document.querySelector(%s); if (!e) return false; e.scrollIntoView({block:'center'}); e.focus(); if ('select' in e) e.select(); return true })()", q))
		if err != nil {
			return err.Error(), true
		}
		if string(v) != "true" {
			return "campo não encontrado: " + in.Selector, true
		}
		if _, err := b.call(ctx, "Input.insertText", map[string]any{"text": in.Text}); err != nil {
			return err.Error(), true
		}
		if in.Submit {
			pressKey(ctx, b, "Enter")
			time.Sleep(300 * time.Millisecond)
			b.waitLoad(ctx)
		}
		return "digitado em " + in.Selector + " · " + where(), false
	case "press":
		if _, ok := keyCodes[in.Key]; !ok {
			return "tecla desconhecida: " + in.Key, true
		}
		pressKey(ctx, b, in.Key)
		time.Sleep(200 * time.Millisecond)
		return "tecla " + in.Key + " · " + where(), false
	case "eval":
		v, err := b.eval(ctx, in.Script)
		if err != nil {
			return "erro no script: " + err.Error(), true
		}
		return clip(externalContent(where(), string(v))), false
	case "console":
		b.mu.Lock()
		lines := b.console
		b.console = nil
		b.mu.Unlock()
		if len(lines) == 0 {
			return "nenhuma mensagem no console desde a última consulta", false
		}
		return clip(externalContent(where(), strings.Join(lines, "\n"))), false
	case "viewport":
		if in.Width <= 0 || in.Height <= 0 {
			return "informe width e height", true
		}
		mobile := in.Width < 768
		if _, err := b.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": in.Width, "height": in.Height, "deviceScaleFactor": 1, "mobile": mobile}); err != nil {
			return err.Error(), true
		}
		return fmt.Sprintf("tela de %dx%d%s", in.Width, in.Height, map[bool]string{true: " (celular)", false: ""}[mobile]), false
	}
	return "ação desconhecida: " + in.Action, true
}

func pressKey(ctx context.Context, b *cdpBrowser, key string) {
	code := keyCodes[key]
	text := ""
	if key == "Enter" {
		text = "\r"
	}
	for _, t := range []string{"keyDown", "keyUp"} {
		p := map[string]any{"type": t, "key": key, "code": key, "windowsVirtualKeyCode": code, "nativeVirtualKeyCode": code}
		if t == "keyDown" && text != "" {
			p["text"] = text
		}
		_, _ = b.call(ctx, "Input.dispatchKeyEvent", p)
	}
}

func init() { registerTool(browserTool{}) }
