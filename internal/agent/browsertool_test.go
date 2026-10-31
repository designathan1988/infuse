package agent

import (
	"bytes"
	"context"
	"infuse/internal/browser"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const browserTestPage = `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><title>Bancada</title>
<style>body{font:16px system-ui;margin:24px} #toast{position:fixed;right:20px;bottom:20px;padding:12px 18px;background:#1b5e20;color:#fff;border-radius:8px;opacity:0;transform:translateY(20px);transition:all .5s}
#toast.on{opacity:1;transform:none} #longo{height:2200px} #rodape{width:300px;height:80px;background:rgb(220,20,20);color:#fff} button:hover{outline:3px solid blue}</style></head>
<body><h1>Bancada de teste</h1>
<label for="nome">Nome</label> <input id="nome" value="antigo">
<button id="salvar" onclick="salvar()">Salvar</button>
<label for="cor">Cor</label> <select id="cor"><option value="r">Vermelho</option><option value="g">Verde</option></select>
<div id="duplo" ondblclick="this.dataset.n=(+this.dataset.n||0)+1" style="padding:8px;background:#eee;width:120px">duplo</div>
<div id="direito" oncontextmenu="event.preventDefault();this.dataset.n=(+this.dataset.n||0)+1" style="padding:8px;background:#ddd;width:120px">direito</div>
<textarea id="nota">texto velho</textarea>
<input type="file" id="arq">
<div id="toast"></div><div id="longo"></div><div id="rodape">rodapé</div>
<script>localStorage.setItem('tema','escuro');
function salvar(){ setTimeout(()=>{ const t=document.getElementById('toast'); t.textContent='Salvo: '+document.getElementById('nome').value; t.classList.add('on'); console.error('falha simulada'); fetch('/faltando'); }, 600); }</script></body></html>`

// TestAgentBrowserForReal drives a real headless Edge/Chrome through every action of the Browser tool.
// It needs a browser on the machine: INFUSE_BROWSER_TEST=1 go test -run TestAgentBrowserForReal
func TestAgentBrowserForReal(t *testing.T) {
	if os.Getenv("INFUSE_BROWSER_TEST") == "" {
		t.Skip("INFUSE_BROWSER_TEST=1 para rodar com o navegador real")
	}
	testHome(t) // the user's project list and browser profiles stay untouched
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(browserTestPage))
	}))
	defer srv.Close()
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "anexo.txt"), []byte("conteúdo"), 0o644)
	s := newSession(root)
	defer closeAgentBrowsers()
	out := os.Getenv("INFUSE_BROWSER_TEST_OUT")
	imgs := 0
	run := func(in string) string {
		t.Helper()
		before := len(s.pendingImages)
		res, isErr := browserToolAdapter{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(in))
		if isErr {
			t.Fatalf("%s → %s", in, res)
		}
		if out != "" {
			for _, b := range s.pendingImages[before:] {
				jpg, _ := base64.StdEncoding.DecodeString(b.Data)
				imgs++
				_ = os.WriteFile(filepath.Join(out, fmt.Sprintf("%02d.jpg", imgs)), jpg, 0o644)
			}
		}
		t.Logf("%s\n→ %s", in, res)
		return res
	}
	js := func(expr string) string {
		p, _ := browser.AgentPageFor(root, s.settings)
		v, err := p.Eval(context.Background(), expr)
		if err != nil {
			t.Fatal(err)
		}
		var x any
		_ = json.Unmarshal(v, &x)
		return fmt.Sprint(x)
	}

	if r := run(`{"action":"navigate","url":"` + srv.URL + `"}`); !strings.Contains(r, "Bancada") {
		t.Fatal(r)
	}
	if js(`!!document.getElementById('__infuse_cursor') || 'sem'`) != "true" {
		run(`{"action":"hover","selector":"h1"}`)
	}
	// typing replaces the field's text, accents included
	run(`{"action":"type","selector":"label=Nome","text":"Jônathan Teste"}`)
	if v := js(`document.getElementById('nome').value`); v != "Jônathan Teste" {
		t.Fatalf("typed %q", v)
	}
	// a recording around something that happens over time
	run(`{"action":"record_start"}`)
	run(`{"action":"click","selector":"role=button[name=\"Salvar\"]"}`)
	run(`{"action":"wait_for","text":"Salvo: Jônathan Teste"}`)
	run(`{"action":"scroll","dy":300}`)
	rec := run(`{"action":"record_stop"}`)
	for _, want := range []string{"keyframes", M("browser.ev_console", "type", "error", "text", "falha simulada").String(), "/faltando → 404", M("browser.ev_action", "action", "click").String()} {
		if !strings.Contains(rec, want) {
			t.Fatalf("recording without %q:\n%s", want, rec)
		}
	}
	if !strings.Contains(rec, "Image 2 ·") {
		t.Fatalf("one keyframe only:\n%s", rec)
	}
	run(`{"action":"scroll","dy":-2000}`)
	// select, double and right click, keyboard combinations
	if r := run(`{"action":"select","selector":"label=Cor","value":"Verde"}`); !strings.Contains(r, M("browser.selected", "option", "Verde").String()) || js(`document.getElementById('cor').value`) != "g" {
		t.Fatal(r)
	}
	run(`{"action":"click","selector":"#duplo","count":2}`)
	run(`{"action":"click","selector":"text=direito","button":"right"}`)
	if js(`document.getElementById('duplo').dataset.n + '/' + document.getElementById('direito').dataset.n`) != "1/1" {
		t.Fatal("double or right click not received")
	}
	run(`{"action":"click","selector":"#nota"}`)
	run(`{"action":"press","key":"Control+A"}`)
	run(`{"action":"press","key":"Backspace"}`)
	if v := js(`document.getElementById('nota').value`); v != "" {
		t.Fatalf("Control+A Backspace left %q", v)
	}
	run(`{"action":"hover","selector":"#salvar"}`)
	if js(`document.getElementById('salvar').matches(':hover')`) != "true" {
		t.Fatal("hover")
	}
	if r := run(`{"action":"text","selector":"#toast"}`); !strings.Contains(r, "Salvo: Jônathan Teste") {
		t.Fatal(r)
	}
	// captures: the whole screen at density 2, and one element far down the page
	if r := run(`{"action":"screenshot"}`); !strings.Contains(r, "2880x1800") {
		t.Fatal(r)
	}
	r := run(`{"action":"screenshot","selector":"#rodape"}`)
	jpg, _ := base64.StdEncoding.DecodeString(s.pendingImages[len(s.pendingImages)-1].Data)
	im, err := jpeg.Decode(bytes.NewReader(jpg))
	if err != nil {
		t.Fatal(err)
	}
	b := im.Bounds()
	cr, cg, _, _ := im.At(b.Dx()/2, b.Dy()/2).RGBA()
	if cr>>8 < 180 || cg>>8 > 80 {
		t.Fatalf("element capture missed the element (centre %d,%d): %s", cr>>8, cg>>8, r)
	}
	// upload and diagnostics
	run(`{"action":"upload","selector":"#arq","path":"anexo.txt"}`)
	if js(`document.getElementById('arq').files[0]?.name`) != "anexo.txt" {
		t.Fatal("upload")
	}
	for in, want := range map[string]string{
		`{"action":"network","only_issues":true}`: "404",
		`{"action":"console"}`:                    "falha simulada",
		`{"action":"performance"}`:                "timings (ms)",
		`{"action":"a11y"}`:                       `button "Salvar"`,
		`{"action":"storage"}`:                    "escuro",
		`{"action":"dom","selector":"#cor"}`:      "Verde",
	} {
		if r := run(in); !strings.Contains(r, want) {
			t.Fatalf("%s without %q: %s", in, want, r)
		}
	}
	// devices and network conditions
	if r := run(`{"action":"emulate","device":"iphone"}`); !strings.Contains(r, "980 px") || !strings.Contains(r, "viewport") {
		t.Fatalf("missing viewport meta not reported: %s", r)
	}
	if js(`screen.width + 'x' + devicePixelRatio`) != "390x3" {
		t.Fatal("iphone emulation: " + js(`innerWidth + 'x' + devicePixelRatio + ' ' + navigator.userAgent`))
	}
	run(`{"action":"emulate","device":"desktop"}`)
	if js(`String(innerWidth)`) != "1440" {
		t.Fatal("back to desktop")
	}
	run(`{"action":"throttle","network":"offline"}`)
	if js(`fetch('/').then(() => 'online', () => 'offline')`) != "offline" {
		t.Fatal("offline")
	}
	run(`{"action":"throttle","network":"none"}`)
	if r := run(`{"action":"pdf"}`); !strings.Contains(r, ".pdf") {
		t.Fatal(r)
	}
	run(`{"action":"back"}`)
}
