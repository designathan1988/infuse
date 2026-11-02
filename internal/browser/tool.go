package browser

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
	"infuse/internal/config"
	"infuse/internal/core"
	"strings"
	"time"
)

// Host is what the tool needs from the session that runs it: the project, its settings, a place to
// attach images for the model, output clipping and the message recorder.
type Host interface {
	RootDir() string
	ProjectFile(path string) (string, error) // a project file the agent may read, by its real path
	Settings() config.Settings
	AttachImage(core.Block)
	Clip(string) string
	Say(core.Msg) string
	SayErr(error) string
	NoteScreenshot(url string) // what the capture looked at, for the turn's visual-evidence gate
}

type Tool struct{}

func rawSchema(s string) json.RawMessage { return json.RawMessage(s) }

func (Tool) Category() string { return "net" }
func (Tool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Browser", Description: `A real browser (without a window) that you use the way a person does: the cursor appears and moves, clicks have a pause, typing is key by key. Use it to open the app (localhost), check screens, reproduce bugs and diagnose.
Locators (selector): CSS, text=Sign in, role=button[name="Sign in"], label=Email, placeholder=Search, testid=save, xpath=//...
Actions: navigate(url) · back · reload · click(selector; button=right; count=2 for a double click) · hover(selector) · type(selector, text, submit) · press(key: Enter, Tab, Control+A, Shift+Tab…) · scroll(dy or selector) · drag(selector, to) · select(selector, value) · upload(selector, path in the project) · wait_for(selector or text, timeout ms) · text(optional selector) · dom(selector: HTML) · eval(script) ·
screenshot(full_page, selector for just that element) — you see the image in high resolution ·
record_start / record_stop — for something that happens over time (an animation, a transition, loading, an intermittent error): you receive the keyframes where the screen changed, each one as an image, with the timeline of actions, console errors and network failures between them, and an enlarged crop of small changes ·
console · network(only_issues) · performance · a11y (accessibility tree) · storage (cookies and localStorage) · throttle(network: offline | 3g | 4g | none) · emulate(device: iphone | pixel | ipad | laptop | desktop) · viewport(width, height) · pdf.`,
		InputSchema: rawSchema(`{"type":"object","properties":{"action":{"type":"string","enum":["navigate","back","reload","click","hover","type","press","scroll","drag","select","upload","wait_for","text","dom","eval","screenshot","record_start","record_stop","console","network","performance","a11y","storage","throttle","emulate","viewport","pdf"]},"url":{"type":"string"},"selector":{"type":"string"},"to":{"type":"string","description":"drag: the target's locator"},"text":{"type":"string"},"value":{"type":"string"},"path":{"type":"string"},"submit":{"type":"boolean"},"key":{"type":"string"},"button":{"type":"string","enum":["left","right","middle"]},"count":{"type":"integer"},"dy":{"type":"number"},"script":{"type":"string"},"full_page":{"type":"boolean"},"timeout":{"type":"integer"},"max_frames":{"type":"integer"},"only_issues":{"type":"boolean"},"network":{"type":"string"},"device":{"type":"string"},"width":{"type":"integer"},"height":{"type":"integer"}},"required":["action"]}`)}
}

// attach hands an image to the model, at most 1600 px wide (the file on disk keeps full resolution).
func attachImage(h Host, jpg []byte) {
	if im, err := jpeg.Decode(bytes.NewReader(jpg)); err == nil && im.Bounds().Dx() > 1600 {
		var buf bytes.Buffer
		_ = jpeg.Encode(&buf, shrink(im, 1600), &jpeg.Options{Quality: 88})
		jpg = buf.Bytes()
	}
	h.AttachImage(core.Block{Type: "image", MediaType: "image/jpeg", Data: base64.StdEncoding.EncodeToString(jpg)})
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

func (Tool) Run(ctx context.Context, h Host, input json.RawMessage) (string, bool) {
	var in actionInput
	var raw map[string]any
	_ = json.Unmarshal(input, &raw)
	_ = json.Unmarshal(input, &in)
	in.FullPage, _ = raw["full_page"].(bool)
	in.OnlyIssues, _ = raw["only_issues"].(bool)
	if v, ok := raw["max_frames"].(float64); ok {
		in.MaxFrames = int(v)
	}
	p, err := AgentPageFor(h.RootDir(), h.Settings())
	if err != nil {
		return h.SayErr(err), true
	}
	g := gesturesFor(h.Settings())
	wait := time.Duration(in.Timeout) * time.Millisecond
	if wait <= 0 {
		wait = 8 * time.Second
	}
	a := &action{ctx: ctx, h: h, p: p, g: g, wait: wait, in: in}
	if !strings.HasPrefix(in.Action, "record_") {
		what := core.FirstNonEmpty(core.FirstNonEmpty(in.Selector, in.URL), core.FirstNonEmpty(in.Key, in.Text))
		if in.Action == "scroll" && in.Selector == "" {
			what = fmt.Sprintf("%+.0f px", core.FirstNonZero(in.Dy, 600))
		}
		p.note(core.M("browser.ev_action", "action", strings.TrimSpace(in.Action+" "+what)))
	}
	switch in.Action {
	case "navigate", "back", "reload":
		return a.browse()
	case "click", "hover", "type", "press", "scroll", "drag", "select", "upload", "wait_for":
		return a.interact()
	case "text", "dom", "eval", "screenshot":
		return a.content()
	case "record_start", "record_stop":
		return a.record()
	case "console", "network", "performance", "a11y", "storage", "throttle", "emulate", "viewport", "pdf":
		return a.inspect()
	}
	return h.Say(core.M("tool.unknown_action", "action", in.Action)), true
}

type actionInput struct {
	Action, URL, Selector, To, Text, Value, Path, Key, Button, Script, Network, Device string
	Submit, FullPage, OnlyIssues                                                       bool
	Count, Timeout, MaxFrames, Width, Height                                           int
	Dy                                                                                 float64
}

type action struct {
	ctx  context.Context
	h    Host
	p    *Page
	g    gestureStyle
	wait time.Duration
	in   actionInput
}

func (a *action) where() string {
	v, _ := a.p.b.eval(a.ctx, `location.href + ' — ' + document.title`)
	var w string
	_ = json.Unmarshal(v, &w)
	return w
}

// locateErr finds an element; the error says why not (no selector, not found, not visible).
func (a *action) locateErr(sel string) (*located, error) {
	if sel == "" {
		return nil, core.ErrMsg(core.M("browser.give_selector"))
	}
	return a.p.locate(a.ctx, sel, a.wait)
}

// locate is locateErr for the tool's output: the reason, recorded with the call.
func (a *action) locate(sel string) (*located, string) {
	l, err := a.locateErr(sel)
	if err != nil {
		return nil, a.h.SayErr(err)
	}
	return l, ""
}
