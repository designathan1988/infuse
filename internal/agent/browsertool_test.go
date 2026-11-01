package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"infuse/internal/browser"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const browserTestPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Bench</title>
<style>body{font:16px system-ui;margin:24px} #toast{position:fixed;right:20px;bottom:20px;padding:12px 18px;background:#1b5e20;color:#fff;border-radius:8px;opacity:0;transform:translateY(20px);transition:all .5s}
#toast.on{opacity:1;transform:none} #long{height:2200px} #footer{width:300px;height:80px;background:rgb(220,20,20);color:#fff} button:hover{outline:3px solid blue}</style></head>
<body><h1>Test bench</h1>
<label for="name">Name</label> <input id="name" value="old">
<button id="save" onclick="save()">Save</button>
<label for="color">Color</label> <select id="color"><option value="r">Red</option><option value="g">Green</option></select>
<div id="double" ondblclick="this.dataset.n=(+this.dataset.n||0)+1" style="padding:8px;background:#eee;width:120px">double</div>
<div id="right" oncontextmenu="event.preventDefault();this.dataset.n=(+this.dataset.n||0)+1" style="padding:8px;background:#ddd;width:120px">right</div>
<textarea id="note">old text</textarea>
<input type="file" id="upload">
<div id="toast"></div><div id="long"></div><div id="footer">footer</div>
<script>localStorage.setItem('theme','dark');
function save(){ setTimeout(()=>{ const t=document.getElementById('toast'); t.textContent='Saved: '+document.getElementById('name').value; t.classList.add('on'); console.error('simulated failure'); fetch('/missing'); }, 600); }</script></body></html>`

// TestAgentBrowserForReal drives a real headless Edge/Chrome through every action of the Browser tool.
// It needs a browser on the machine: INFUSE_BROWSER_TEST=1 go test -run TestAgentBrowserForReal
func TestAgentBrowserForReal(t *testing.T) {
	if os.Getenv("INFUSE_BROWSER_TEST") == "" {
		t.Skip("INFUSE_BROWSER_TEST=1 to run with the real browser")
	}
	e := newBrowserTestEnv(t)
	e.typingAndRecording()
	e.pointerAndKeyboard()
	e.captures()
	e.diagnostics()
	e.emulation()
	e.run(`{"action":"back"}`)
}

type browserTestEnv struct {
	t    *testing.T
	s    *Session
	root string
	srv  string
	out  string
	imgs int
}

func newBrowserTestEnv(t *testing.T) *browserTestEnv {
	t.Helper()
	testHome(t) // the user's project list and browser profiles stay untouched
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(browserTestPage))
	}))
	t.Cleanup(srv.Close)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "attachment.txt"), []byte("content"), 0o644)
	s := newSession(root)
	t.Cleanup(closeAgentBrowsers)
	return &browserTestEnv{t: t, s: s, root: root, srv: srv.URL, out: os.Getenv("INFUSE_BROWSER_TEST_OUT")}
}

func (e *browserTestEnv) run(in string) string {
	e.t.Helper()
	before := len(e.s.pendingImages)
	res, isErr := browserToolAdapter{}.Run(context.Background(), &ToolContext{S: e.s, Root: e.root, Cwd: e.root}, json.RawMessage(in))
	if isErr {
		e.t.Fatalf("%s → %s", in, res)
	}
	if e.out != "" {
		for _, b := range e.s.pendingImages[before:] {
			jpg, _ := base64.StdEncoding.DecodeString(b.Data)
			e.imgs++
			_ = os.WriteFile(filepath.Join(e.out, fmt.Sprintf("%02d.jpg", e.imgs)), jpg, 0o644)
		}
	}
	e.t.Logf("%s\n→ %s", in, res)
	return res
}

func (e *browserTestEnv) js(expr string) string {
	e.t.Helper()
	p, _ := browser.AgentPageFor(e.root, e.s.settings)
	v, err := p.Eval(context.Background(), expr)
	if err != nil {
		e.t.Fatal(err)
	}
	var x any
	_ = json.Unmarshal(v, &x)
	return fmt.Sprint(x)
}

func (e *browserTestEnv) typingAndRecording() {
	t := e.t
	if r := e.run(`{"action":"navigate","url":"` + e.srv + `"}`); !strings.Contains(r, "Bench") {
		t.Fatal(r)
	}
	if e.js(`!!document.getElementById('__infuse_cursor') || 'none'`) != "true" {
		e.run(`{"action":"hover","selector":"h1"}`)
	}
	e.run(`{"action":"type","selector":"label=Name","text":"John Test"}`)
	if v := e.js(`document.getElementById('name').value`); v != "John Test" {
		t.Fatalf("typed %q", v)
	}
	// a recording around something that happens over time
	e.run(`{"action":"record_start"}`)
	e.run(`{"action":"click","selector":"role=button[name=\"Save\"]"}`)
	e.run(`{"action":"wait_for","text":"Saved: John Test"}`)
	e.run(`{"action":"scroll","dy":300}`)
	rec := e.run(`{"action":"record_stop"}`)
	for _, want := range []string{"keyframes", M("browser.ev_console", "type", "error", "text", "simulated failure").String(), "/missing → 404", M("browser.ev_action", "action", "click").String()} {
		if !strings.Contains(rec, want) {
			t.Fatalf("recording without %q:\n%s", want, rec)
		}
	}
	if !strings.Contains(rec, "Image 2 ·") {
		t.Fatalf("one keyframe only:\n%s", rec)
	}
	e.run(`{"action":"scroll","dy":-2000}`)
}

func (e *browserTestEnv) pointerAndKeyboard() {
	t := e.t
	if r := e.run(`{"action":"select","selector":"label=Color","value":"Green"}`); !strings.Contains(r, M("browser.selected", "option", "Green").String()) || e.js(`document.getElementById('color').value`) != "g" {
		t.Fatal(r)
	}
	e.run(`{"action":"click","selector":"#double","count":2}`)
	e.run(`{"action":"click","selector":"text=right","button":"right"}`)
	if e.js(`document.getElementById('double').dataset.n + '/' + document.getElementById('right').dataset.n`) != "1/1" {
		t.Fatal("double or right click not received")
	}
	e.run(`{"action":"click","selector":"#note"}`)
	e.run(`{"action":"press","key":"Control+A"}`)
	e.run(`{"action":"press","key":"Backspace"}`)
	if v := e.js(`document.getElementById('note').value`); v != "" {
		t.Fatalf("Control+A Backspace left %q", v)
	}
	e.run(`{"action":"hover","selector":"#save"}`)
	if e.js(`document.getElementById('save').matches(':hover')`) != "true" {
		t.Fatal("hover")
	}
	if r := e.run(`{"action":"text","selector":"#toast"}`); !strings.Contains(r, "Saved: John Test") {
		t.Fatal(r)
	}
}

func (e *browserTestEnv) captures() {
	t := e.t
	if r := e.run(`{"action":"screenshot"}`); !strings.Contains(r, "2880x1800") {
		t.Fatal(r)
	}
	r := e.run(`{"action":"screenshot","selector":"#footer"}`)
	jpg, _ := base64.StdEncoding.DecodeString(e.s.pendingImages[len(e.s.pendingImages)-1].Data)
	im, err := jpeg.Decode(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	b := im.Bounds()
	cr, cg, _, _ := im.At(b.Dx()/2, b.Dy()/2).RGBA()
	if cr>>8 < 180 || cg>>8 > 80 {
		t.Fatalf("element capture missed the element (centre %d,%d): %s", cr>>8, cg>>8, r)
	}
}

func (e *browserTestEnv) diagnostics() {
	t := e.t
	e.run(`{"action":"upload","selector":"#upload","path":"attachment.txt"}`)
	if e.js(`document.getElementById('upload').files[0]?.name`) != "attachment.txt" {
		t.Fatal("upload")
	}
	for in, want := range map[string]string{
		`{"action":"network","only_issues":true}`: "404",
		`{"action":"console"}`:                    "simulated failure",
		`{"action":"performance"}`:                "timings (ms)",
		`{"action":"a11y"}`:                       `button "Save"`,
		`{"action":"storage"}`:                    "dark",
		`{"action":"dom","selector":"#color"}`:    "Green",
	} {
		if r := e.run(in); !strings.Contains(r, want) {
			t.Fatalf("%s without %q: %s", in, want, r)
		}
	}
}

func (e *browserTestEnv) emulation() {
	t := e.t
	if r := e.run(`{"action":"emulate","device":"iphone"}`); !strings.Contains(r, "980 px") || !strings.Contains(r, "viewport") {
		t.Fatalf("missing viewport meta not reported: %s", r)
	}
	if e.js(`screen.width + 'x' + devicePixelRatio`) != "390x3" {
		t.Fatal("iphone emulation: " + e.js(`innerWidth + 'x' + devicePixelRatio + ' ' + navigator.userAgent`))
	}
	e.run(`{"action":"emulate","device":"desktop"}`)
	if e.js(`String(innerWidth)`) != "1440" {
		t.Fatal("back to desktop")
	}
	e.run(`{"action":"throttle","network":"offline"}`)
	if e.js(`fetch('/').then(() => 'online', () => 'offline')`) != "offline" {
		t.Fatal("offline")
	}
	e.run(`{"action":"throttle","network":"none"}`)
	if r := e.run(`{"action":"pdf"}`); !strings.Contains(r, ".pdf") {
		t.Fatal(r)
	}
}
