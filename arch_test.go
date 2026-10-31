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

func wantArch(t *testing.T, root, rel, before, after, want string) {
	t.Helper()
	got := archCheck(root, root, rel, before, after)
	if want == "" && got != "" {
		t.Fatalf("%s refused: %s", rel, got)
	}
	if want != "" && !strings.Contains(got, want) {
		t.Fatalf("%s: want %q, got %q", rel, want, got)
	}
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
	wantArch(t, empty, "cli.js", "", "console.log(1)\n", "") // a project being born starts where it wants
	root := archProject(t, map[string]string{"src/app.ts": "export const a = 1;\n", "README.md": "# x\n"})
	wantArch(t, root, "util.ts", "", "export const b = 2;\n", "solto na raiz")
	wantArch(t, root, "src/util.ts", "", "export const b = 2;\n", "")
	wantArch(t, root, "misc/util.ts", "", "export const b = 2;\n", "pasta nova na raiz")
	wantArch(t, root, "tests/util.test.ts", "", "export const b = 2;\n", "")
	wantArch(t, root, "vite.config.md", "", "x\n", "")
}

func TestArchFunctions(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	wantArch(t, root, "src/b.ts", "", longFunc("calcular", 70), "função calcular")
	wantArch(t, root, "src/b.ts", "", longFunc("calcular", 30), "")
	legacy := longFunc("antiga", 80)
	wantArch(t, root, "src/c.ts", legacy, strings.Replace(legacy, "x + 3", "x + 33", 1), "") // existing debt does not block
	wantArch(t, root, "src/c.ts", legacy, longFunc("antiga", 81), "função antiga")         // but may not grow
	var ifs strings.Builder
	ifs.WriteString("function decide(a, b) {\n")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&ifs, "  if (a > %d && b) { a++; }\n", i)
	}
	ifs.WriteString("  return a;\n}\n")
	wantArch(t, root, "src/d.js", "", ifs.String(), "complexidade")
	deep := "function fundo(a) {\n if (a) {\n  for (;;) {\n   while (a) {\n    if (a) {\n     if (a) {\n      if (a) { a--; }\n     }\n    }\n   }\n  }\n }\n}\n"
	wantArch(t, root, "src/e.js", "", deep, "aninhamento")
	goLong := "package main\n\nfunc grande() {\n" + strings.Repeat("\tx := 1\n\t_ = x\n", 40) + "}\n"
	wantArch(t, root, "src/f.go", "", goLong, "função grande")
	// braces inside strings and comments do not count
	wantArch(t, root, "src/g.js", "", "function s() {\n  const a = '{{{{';\n  // }}}\n  return a;\n}\n", "")
}

func TestArchTables(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	var sw strings.Builder
	sw.WriteString("function cor(k) {\n  switch (k) {\n")
	for i, c := range []string{"vermelho", "verde", "azul", "preto", "branco", "cinza", "rosa"} {
		fmt.Fprintf(&sw, "    case '%s': return %d;\n", c, i)
	}
	sw.WriteString("  }\n}\n")
	wantArch(t, root, "src/cor.js", "", sw.String(), "tabela")
	wantArch(t, root, "src/cor.js", sw.String(), sw.String()+"\nexport const z = 1;\n", "") // the chain was already there
	ifChain := "function f(t) {\n  if (t === 'a') return 1;\n  else if (t === 'b') return 2;\n  else if (t === 'c') return 3;\n  else if (t === 'd') return 4;\n  else if (t === 'e') return 5;\n  else if (t === 'f') return 6;\n}\n"
	wantArch(t, root, "src/f.js", "", ifChain, "tabela")
}

func TestArchStyles(t *testing.T) {
	root := archProject(t, map[string]string{"src/tokens.css": ":root {\n  --brand: #3366ff;\n  --bg: #ffffff;\n  --text: #111111;\n  --muted: #777777;\n  --gap: 8px;\n}\n"})
	wantArch(t, root, "src/card.css", "", ".card { color: #3366ff; }\n", "var(--brand)")
	wantArch(t, root, "src/card.css", "", ".card { color: var(--brand); padding: var(--gap); }\n", "")
	wantArch(t, root, "src/tokens.css", ":root{--a:#000}", ":root{--a:#000;--b:#123456}", "") // the tokens file holds the literals
	wantArch(t, root, "src/Card.tsx", "", "export const C = () => <div style={{ color: '#ff0000' }} />;\n", "inline")
	wantArch(t, root, "src/Card.tsx", "", "export const C = (x) => <div style={{ left: x + 'px' }} />;\n", "") // computed values are fine
}

func TestArchStateAndConstants(t *testing.T) {
	root := archProject(t, map[string]string{
		"src/store.ts":  "import { create } from 'zustand';\nexport const useStore = create((set) => ({ n: 0 }));\n",
		"src/config.ts": "export const API_URL = 'https://api.exemplo.com';\n",
	})
	wantArch(t, root, "src/other.ts", "", "import { create } from 'zustand';\nexport const useOther = create((set) => ({ m: 0 }));\n", "nova store")
	wantArch(t, root, "src/ui.ts", "", "window.contador = 0;\n", "estado global")
	wantArch(t, root, "src/ui.ts", "", "const largura = window.innerWidth;\n", "")
	wantArch(t, root, "src/api.ts", "", "export const API_URL = 'https://api.exemplo.com/v2';\n", "API_URL já está definida em src/config.ts")
	wantArch(t, root, "src/api.ts", "", "import { API_URL } from './config';\n", "")
}

func TestArchDuplicatesAndCycles(t *testing.T) {
	block := "export function total(itens) {\n  const soma = itens.reduce((acc, item) => acc + item.preco * item.quantidade, 0);\n  const desconto = soma > 1000 ? soma * 0.1 : soma > 500 ? soma * 0.05 : 0;\n  const imposto = (soma - desconto) * 0.18;\n  const frete = soma - desconto > 300 ? 0 : 25;\n  const final = soma - desconto + imposto + frete;\n  return Math.round(final * 100) / 100;\n}\n"
	root := archProject(t, map[string]string{"src/carrinho.ts": block, "src/a.ts": "import { b } from './b';\nexport const a = () => b;\n", "src/b.ts": "export const b = 1;\n"})
	wantArch(t, root, "src/pedido.ts", "", "// cópia\n"+block, "repete")
	wantArch(t, root, "src/pedido.ts", "", "import { total } from './carrinho';\nexport const pedido = (i) => total(i);\n", "")
	wantArch(t, root, "src/b.ts", "export const b = 1;\n", "import { a } from './a';\nexport const b = 1;\nexport const c = a;\n", "ciclo: src/b.ts → src/a.ts → src/b.ts")
}

func TestArchJSON(t *testing.T) {
	root := archProject(t, map[string]string{"src/a.ts": "export const a = 1;\n"})
	wantArch(t, root, "data/cidades.json", "", `[{"nome":"Recife"}]`, "sem contrato")
	wantArch(t, root, "package.json", "", `{"name":"x"}`, "")
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"cidade","type":"object","required":["nome","uf"],"properties":{"nome":{"type":"string"},"uf":{"type":"string","minLength":2,"maxLength":2}},"additionalProperties":false}`
	wantArch(t, root, "schemas/cidade.schema.json", "", `{"type": 12}`, "schema inválido")
	_ = os.MkdirAll(filepath.Join(root, "schemas"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "schemas", "cidade.schema.json"), []byte(schema), 0o644)
	wantArch(t, root, "data/recife.json", "", `{"$schema":"../schemas/cidade.schema.json","nome":"Recife","uf":"PE"}`, "")
	wantArch(t, root, "data/recife.json", "", `{"$schema":"../schemas/cidade.schema.json","nome":"Recife","uf":"Pernambuco"}`, "não seguem o schema")
	wantArch(t, root, "data/recife.json", "", `{"$schema":"../schemas/cidade.schema.json","nome":"Recife"}`, "não seguem o schema")
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
