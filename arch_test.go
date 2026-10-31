package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func archProject(t *testing.T, files map[string]string) string {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	for p, c := range files {
		abs := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		if err := os.WriteFile(abs, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// wantArch checks the refusal of a write: want is the message expected (its id and the arguments it
// names), or empty when the write must pass.
func wantArch(t *testing.T, root, rel, before, after string, want Msg) {
	t.Helper()
	got := archCheck(root, root, rel, before, after)
	if want.Empty() && !got.Empty() {
		t.Fatalf("%s refused: %s", rel, got)
	}
	if !want.Empty() && !msgMatches(got, want) {
		t.Fatalf("%s: want %s %v, got %s %v (%s)", rel, want.ID, want.Args, got.ID, got.Args, got)
	}
}

// msgMatches reports whether got is the message want names: the same id, and the same value for each
// argument want gives (nested messages compared by what they say).
func msgMatches(got, want Msg) bool {
	if got.ID != want.ID {
		return false
	}
	for k, v := range want.Args {
		if fmt.Sprint(got.Args[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func longFunc(name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "export function %s(x) {\n", name)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  const v%d = x + %d;\n", i, i)
	}
	b.WriteString("  return x;\n}\n")
	return b.String()
}

func TestArchPlacement(t *testing.T) {
	empty := archProject(t, nil)
	wantArch(t, empty, "cli.js", "", "console.log(1)\n", Msg{}) // a project being born starts where it wants
	root := archProject(t, map[string]string{"src/app.ts": "export const a = 1;\n", "README.md": "# x\n"})
	wantArch(t, root, "util.ts", "", "export const b = 2;\n", M("arch.loose_root", "file", "util.ts"))
	wantArch(t, root, "src/util.ts", "", "export const b = 2;\n", Msg{})
	wantArch(t, root, "misc/util.ts", "", "export const b = 2;\n", M("arch.new_folder", "dir", "misc"))
	wantArch(t, root, "tests/util.test.ts", "", "export const b = 2;\n", Msg{})
	wantArch(t, root, "vite.config.md", "", "x\n", Msg{})
}

func TestArchFunctions(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	wantArch(t, root, "src/b.ts", "", longFunc("calcular", 70), M("arch.func_lines", "func", "calcular", "file", "src/b.ts"))
	wantArch(t, root, "src/b.ts", "", longFunc("calcular", 30), Msg{})
	legacy := longFunc("antiga", 80)
	wantArch(t, root, "src/c.ts", legacy, strings.Replace(legacy, "x + 3", "x + 33", 1), Msg{})           // existing debt does not block
	wantArch(t, root, "src/c.ts", legacy, longFunc("antiga", 81), M("arch.func_lines", "func", "antiga")) // but may not grow
	var ifs strings.Builder
	ifs.WriteString("function decide(a, b) {\n")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&ifs, "  if (a > %d && b) { a++; }\n", i)
	}
	ifs.WriteString("  return a;\n}\n")
	wantArch(t, root, "src/d.js", "", ifs.String(), M("arch.func_complexity", "func", "decide"))
	deep := "function fundo(a) {\n if (a) {\n  for (;;) {\n   while (a) {\n    if (a) {\n     if (a) {\n      if (a) { a--; }\n     }\n    }\n   }\n  }\n }\n}\n"
	wantArch(t, root, "src/e.js", "", deep, M("arch.func_nesting", "func", "fundo"))
	goLong := "package main\n\nfunc grande() {\n" + strings.Repeat("\tx := 1\n\t_ = x\n", 40) + "}\n"
	wantArch(t, root, "src/f.go", "", goLong, M("arch.func_lines", "func", "grande"))
	// braces inside strings and comments do not count
	wantArch(t, root, "src/g.js", "", "function s() {\n  const a = '{{{{';\n  // }}}\n  return a;\n}\n", Msg{})
}

func TestArchTables(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	var sw strings.Builder
	sw.WriteString("function cor(k) {\n  switch (k) {\n")
	for i, c := range []string{"vermelho", "verde", "azul", "preto", "branco", "cinza", "rosa"} {
		fmt.Fprintf(&sw, "    case '%s': return %d;\n", c, i)
	}
	sw.WriteString("  }\n}\n")
	wantArch(t, root, "src/cor.js", "", sw.String(), M("arch.table", "count", 7))
	wantArch(t, root, "src/cor.js", sw.String(), sw.String()+"\nexport const z = 1;\n", Msg{}) // the chain was already there
	ifChain := "function f(t) {\n  if (t === 'a') return 1;\n  else if (t === 'b') return 2;\n  else if (t === 'c') return 3;\n  else if (t === 'd') return 4;\n  else if (t === 'e') return 5;\n  else if (t === 'f') return 6;\n}\n"
	wantArch(t, root, "src/f.js", "", ifChain, M("arch.table"))
}

func TestArchStyles(t *testing.T) {
	root := archProject(t, map[string]string{"src/tokens.css": ":root {\n  --brand: #3366ff;\n  --bg: #ffffff;\n  --text: #111111;\n  --muted: #777777;\n  --gap: 8px;\n}\n"})
	wantArch(t, root, "src/card.css", "", ".card { color: #3366ff; }\n", M("arch.color", "color", "#3366ff", "hint", M("arch.color_hint", "vars", "var(--brand)")))
	wantArch(t, root, "src/card.css", "", ".card { color: var(--brand); padding: var(--gap); }\n", Msg{})
	wantArch(t, root, "src/tokens.css", ":root{--a:#000}", ":root{--a:#000;--b:#123456}", Msg{}) // the tokens file holds the literals
	wantArch(t, root, "src/Card.tsx", "", "export const C = () => <div style={{ color: '#ff0000' }} />;\n", M("arch.inline_color"))
	wantArch(t, root, "src/Card.tsx", "", "export const C = (x) => <div style={{ left: x + 'px' }} />;\n", Msg{}) // computed values are fine
}

func TestArchStateAndConstants(t *testing.T) {
	root := archProject(t, map[string]string{
		"src/store.ts":  "import { create } from 'zustand';\nexport const useStore = create((set) => ({ n: 0 }));\n",
		"src/config.ts": "export const API_URL = 'https://api.exemplo.com';\n",
	})
	wantArch(t, root, "src/other.ts", "", "import { create } from 'zustand';\nexport const useOther = create((set) => ({ m: 0 }));\n", M("arch.new_store", "stores", "src/store.ts"))
	wantArch(t, root, "src/ui.ts", "", "window.contador = 0;\n", M("arch.global_state_store", "stores", "src/store.ts"))
	wantArch(t, root, "src/ui.ts", "", "const largura = window.innerWidth;\n", Msg{})
	wantArch(t, root, "src/api.ts", "", "export const API_URL = 'https://api.exemplo.com/v2';\n", M("arch.constant", "name", "API_URL", "file", "src/config.ts"))
	wantArch(t, root, "src/api.ts", "", "import { API_URL } from './config';\n", Msg{})
}

func TestArchDuplicatesAndCycles(t *testing.T) {
	block := "export function total(itens) {\n  const soma = itens.reduce((acc, item) => acc + item.preco * item.quantidade, 0);\n  const desconto = soma > 1000 ? soma * 0.1 : soma > 500 ? soma * 0.05 : 0;\n  const imposto = (soma - desconto) * 0.18;\n  const frete = soma - desconto > 300 ? 0 : 25;\n  const final = soma - desconto + imposto + frete;\n  return Math.round(final * 100) / 100;\n}\n"
	root := archProject(t, map[string]string{"src/carrinho.ts": block, "src/a.ts": "import { b } from './b';\nexport const a = () => b;\n", "src/b.ts": "export const b = 1;\n"})
	wantArch(t, root, "src/pedido.ts", "", "// cópia\n"+block, M("arch.duplicate", "file", "src/carrinho.ts"))
	wantArch(t, root, "src/pedido.ts", "", "import { total } from './carrinho';\nexport const pedido = (i) => total(i);\n", Msg{})
	wantArch(t, root, "src/b.ts", "export const b = 1;\n", "import { a } from './a';\nexport const b = 1;\nexport const c = a;\n", M("arch.cycle", "loop", "src/b.ts → src/a.ts → src/b.ts"))
}

func TestArchJSON(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	wantArch(t, root, "data/cidades.json", "", `[{"nome":"Recife"}]`, M("arch.no_schema", "file", "data/cidades.json"))
	wantArch(t, root, "package.json", "", `{"name":"x"}`, Msg{})
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"cidade","type":"object","required":["nome","uf"],"properties":{"nome":{"type":"string"},"uf":{"type":"string","minLength":2,"maxLength":2}},"additionalProperties":false}`
	wantArch(t, root, "schemas/cidade.schema.json", "", `{"type": 12}`, M("arch.bad_schema", "file", "schemas/cidade.schema.json"))
	_ = os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "schemas", "cidade.schema.json"), []byte(schema), 0o644)
	wantArch(t, root, "data/recife.json", "", `{"$schema":"../schemas/cidade.schema.json","nome":"Recife","uf":"PE"}`, Msg{})
	wantArch(t, root, "data/recife.json", "", `{"$schema":"../schemas/cidade.schema.json","nome":"Recife","uf":"Pernambuco"}`, M("arch.schema_invalid", "file", "data/recife.json"))
	wantArch(t, root, "data/recife.json", "", `{"$schema":"../schemas/cidade.schema.json","nome":"Recife"}`, M("arch.schema_invalid", "file", "data/recife.json"))
}

// The rules live in .infuse/architecture.json: the agent reads them, never writes them.
func TestArchRulesOutOfTheAgentsReach(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	_ = loadArchRules(root)
	s := newSession(root)
	if !s.isProtected(filepath.Join(root, ".infuse", "architecture.json")) {
		t.Fatal("the agent could rewrite its own rules")
	}
	if hiddenInternal(".infuse/architecture.json") {
		t.Fatal("the agent cannot read the rules it must follow")
	}
}
