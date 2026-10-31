package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The agent's browser against a real headless Edge/Chrome: open, read, type, click, console, screenshot.
func TestAgentBrowserDrivesARealPage(t *testing.T) {
	if findChromium() == "" {
		t.Skip("no Chromium-family browser installed")
	}
	testHome(t)
	root := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><title>Login</title><h1>Entrar</h1><input id="email"><button onclick="document.querySelector('h1').innerText='Olá, '+document.querySelector('#email').value; console.log('clicou')">Continuar</button>`)
	}))
	defer srv.Close()
	defer closeAgentBrowsers()
	s := newSession(root)
	tc := &ToolContext{S: s, Root: root, Cwd: root}
	run := func(in string) string {
		out, isErr := browserTool{}.Run(context.Background(), tc, json.RawMessage(in))
		if isErr {
			t.Fatalf("%s → %s", in, out)
		}
		return out
	}
	if out := run(`{"action":"navigate","url":"` + srv.URL + `"}`); !strings.Contains(out, "Login") {
		t.Fatal(out)
	}
	run(`{"action":"type","selector":"#email","text":"ana@x.com"}`)
	run(`{"action":"click","selector":"text=Continuar"}`)
	if out := run(`{"action":"text","selector":"h1"}`); !strings.Contains(out, "Olá, ana@x.com") {
		t.Fatal(out)
	}
	if out := run(`{"action":"console"}`); !strings.Contains(out, "clicou") {
		t.Fatal(out)
	}
	run(`{"action":"viewport","width":375,"height":812}`)
	out := run(`{"action":"screenshot"}`)
	name := out[strings.LastIndex(out, "/")+1:]
	if st, err := os.Stat(filepath.Join(root, ".infuse", "shots", name)); err != nil || st.Size() < 1000 {
		t.Fatal("screenshot not saved:", out)
	}
	if len(s.pendingImages) != 1 {
		t.Fatal("the model does not see the screenshot")
	}
	if _, isErr := (browserTool{}).Run(context.Background(), tc, json.RawMessage(`{"action":"click","selector":"#nao-existe"}`)); !isErr {
		t.Fatal("missing element not reported")
	}
}
