package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func diagProject(t *testing.T) (*Session, string) {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	files := map[string]string{
		"CLAUDE.md":               "# Projeto\nLeia docs/AUDITORIA.md antes de otimizar.\n",
		"docs/AUDITORIA.md":       "# Auditoria\nO quadro é preso na GPU porque o renderizador desenha tudo dentro de raios fixos, sem teste de oclusão.\nPasso P11: culling por oclusão na GPU e LOD por erro na tela.\n",
		"src/render/cellLayer.ts": "export function draw(cells) {\n  for (const c of cells) if (c.inFrustum) c.render();\n}\n",
	}
	for p, c := range files {
		abs := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		_ = os.WriteFile(abs, []byte(c), 0o644)
	}
	s := newSession(root)
	s.Mode = "bypass"
	return s, root
}

func readAll(t *testing.T, s *Session, rel string) {
	t.Helper()
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "r", Name: "Read", Input: json.RawMessage(`{"path":"` + rel + `"}`)}); isErr {
		t.Fatal(out)
	}
}

func diagnose(s *Session, d map[string]any) (string, bool) {
	in, _ := json.Marshal(d)
	return s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "d", Name: "Diagnosis", Input: in})
}

var goodDiag = map[string]any{
	"problem":    "a GPU chega a 100% em todas as vistas do jogo",
	"root_cause": "o renderizador desenha tudo dentro de raios fixos sem teste de oclusão",
	"approach":   "implementar culling por oclusão na GPU e LOD por erro na tela, o passo P11",
	"why_cause":  "o custo passa a acompanhar o que aparece na tela, não o raio desenhado",
	"evidence": []map[string]string{
		{"file": "docs/AUDITORIA.md", "quote": "desenha tudo dentro de raios fixos, sem teste de oclusão", "supports": "a causa já registrada"},
		{"file": "src/render/cellLayer.ts", "quote": "if (c.inFrustum) c.render();", "supports": "só o campo de visão decide"},
	},
}

func TestNoChangeOrMeasurementBeforeADiagnosis(t *testing.T) {
	s, _ := diagProject(t)
	s.gate.planned = true
	readAll(t, s, "docs/AUDITORIA.md") // the document the instructions point to (its own gate)
	// reading is free; measuring and changing are not
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, bashCall("1", "git log --oneline -3 && ls src")); strings.Contains(out, "no diagnosis") && isErr {
		t.Fatal("a read-only command was blocked")
	}
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, bashCall("2", "npm run medir-gpu")); !isErr || !strings.Contains(out, "no diagnosis") {
		t.Fatalf("a measurement ran before the diagnosis: %s", out)
	}
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, writeCall("3", "src/render/cellLayer.ts", "x\n")); !isErr || !strings.Contains(out, "no diagnosis") {
		t.Fatalf("a change before the diagnosis: %s", out)
	}
}

func TestDiagnosisEvidenceIsChecked(t *testing.T) {
	s, _ := diagProject(t)
	// quotes from files not read to the end are refused
	if out, isErr := diagnose(s, goodDiag); !isErr || !strings.Contains(out, "read it to the end") {
		t.Fatalf("unread evidence accepted: %s", out)
	}
	readAll(t, s, "docs/AUDITORIA.md")
	readAll(t, s, "src/render/cellLayer.ts")
	// an invented quote is refused
	bad := map[string]any{}
	for k, v := range goodDiag {
		bad[k] = v
	}
	bad["evidence"] = []map[string]string{
		{"file": "docs/AUDITORIA.md", "quote": "a sombra das nuvens é a causa principal do custo", "supports": "x"},
		{"file": "src/render/cellLayer.ts", "quote": "if (c.inFrustum) c.render();", "supports": "y"},
	}
	if out, isErr := diagnose(s, bad); !isErr || !strings.Contains(out, "not in docs/AUDITORIA.md") {
		t.Fatalf("invented quote accepted: %s", out)
	}
	// evidence only from code, when the project has documents about it, is refused
	codeOnly := map[string]any{}
	for k, v := range goodDiag {
		codeOnly[k] = v
	}
	codeOnly["evidence"] = []map[string]string{
		{"file": "src/render/cellLayer.ts", "quote": "if (c.inFrustum) c.render();", "supports": "y"},
		{"file": "src/render/cellLayer.ts", "quote": "export function draw(cells) {", "supports": "z"},
	}
	if out, isErr := diagnose(s, codeOnly); !isErr || !strings.Contains(out, "project's own documents") {
		t.Fatalf("diagnosis ignoring the project's documents accepted: %s", out)
	}
	if out, isErr := diagnose(s, goodDiag); isErr {
		t.Fatal(out)
	}
	s.gate.planned = true
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, bashCall("4", "npm run medir-gpu")); strings.Contains(out, "no diagnosis") {
		t.Fatalf("still blocked after a valid diagnosis: %s %v", out, isErr)
	}
}

func TestFactsAgainstTheApproachDemandANewDiagnosis(t *testing.T) {
	s, root := diagProject(t)
	readAll(t, s, "docs/AUDITORIA.md")
	readAll(t, s, "src/render/cellLayer.ts")
	if out, isErr := diagnose(s, goodDiag); isErr {
		t.Fatal(out)
	}
	s.gate.planned = true
	// the agent undoes its own change
	orig, _ := os.ReadFile(filepath.Join(root, "src/render/cellLayer.ts"))
	run := func(b Block) (string, bool) { return s.runTool(context.Background(), &RuleSet{}, b) }
	if out, isErr := run(writeCall("w1", "src/render/cellLayer.ts", "export function draw(cells) {\n  for (const c of cells) if (c.visible) c.render();\n}\n")); isErr {
		t.Fatal(out)
	}
	if out, isErr := run(writeCall("w2", "src/render/cellLayer.ts", string(orig))); isErr {
		t.Fatal(out)
	}
	out, isErr := run(writeCall("w3", "src/render/cellLayer.ts", "export function draw(cells) {}\n"))
	if !isErr || !strings.Contains(out, "new diagnosis") || !strings.Contains(out, "undoing your own change") {
		t.Fatalf("an undone change did not demand a new diagnosis: %s", out)
	}
	// the same diagnosis again is not accepted (the changed file is read again first)
	readAll(t, s, "src/render/cellLayer.ts")
	if out, isErr := diagnose(s, goodDiag); !isErr || !strings.Contains(out, "evidence not cited before") {
		t.Fatalf("the same diagnosis accepted again: %s", out)
	}
}

func TestMeasureAndTweakLoopDemandsANewDiagnosis(t *testing.T) {
	s, _ := diagProject(t)
	readAll(t, s, "docs/AUDITORIA.md")
	readAll(t, s, "src/render/cellLayer.ts")
	if out, isErr := diagnose(s, goodDiag); isErr {
		t.Fatal(out)
	}
	s.gate.planned = true
	run := func(b Block) (string, bool) { return s.runTool(context.Background(), &RuleSet{}, b) }
	var out string
	for i := 0; i < 4; i++ {
		run(bashCall("m", "echo medindo-gpu"))
		out, _ = run(writeCall("w", "src/render/cellLayer.ts", "export function draw(cells) { /* ajuste "+string(rune('a'+i))+" */ }\n"))
	}
	if !strings.Contains(out, "new diagnosis") || !strings.Contains(out, "echo medindo-gpu") {
		t.Fatalf("the measure-and-tweak loop went on: %s", out)
	}
}

func TestContinuationCarriesThePreviousDiagnosisOnly(t *testing.T) {
	s, _ := diagProject(t)
	if out, isErr := diagnose(s, map[string]any{"continues": true}); !isErr {
		t.Fatalf("continuation with no previous diagnosis: %s", out)
	}
	readAll(t, s, "docs/AUDITORIA.md")
	readAll(t, s, "src/render/cellLayer.ts")
	s.Turn = 1
	if out, isErr := diagnose(s, goodDiag); isErr {
		t.Fatal(out)
	}
	s.Turn = 2
	s.gate.reset()
	if out, isErr := diagnose(s, map[string]any{"continues": true}); isErr || !s.gate.diagnosed {
		t.Fatalf("continuation refused: %s", out)
	}
}

// infuse hands every part of a document to the model; facts whose quote is not on the given lines are
// dropped.
func TestKnowledgeDigestCoversTheWholeDocumentAndChecksQuotes(t *testing.T) {
	s, root := diagProject(t)
	var long strings.Builder
	for i := 1; i <= 1000; i++ {
		long.WriteString("linha de contexto número " + strings.Repeat("x", i%7) + "\n")
	}
	long.WriteString("Decisão D81: o budget da rua mede 33 M de triângulos desenhados.\n")
	_ = os.WriteFile(filepath.Join(root, "docs", "ESTADO.md"), []byte(long.String()), 0o644)
	parts := 0
	p := &scriptProvider{steps: []func(ModelRequest) (ModelResult, error){func(req ModelRequest) (ModelResult, error) {
		parts++
		text := req.Messages[0].Content[0].Text
		facts := `{"facts":[{"line":2,"kind":"state","fact":"invented","quote":"esta frase não existe no documento de forma alguma"}]}`
		if strings.Contains(text, "Decisão D81") {
			facts = `{"facts":[{"line":1001,"kind":"measurement","fact":"street budget draws 33M triangles","quote":"o budget da rua mede 33 M de triângulos desenhados"}]}`
		}
		return ModelResult{Content: []Block{{Type: "text", Text: facts}}, StopReason: "end_turn"}, nil
	}}}
	b, _ := os.ReadFile(filepath.Join(root, "docs", "ESTADO.md"))
	d, err := s.digestDocument(context.Background(), p, "docs/ESTADO.md", b)
	if err != nil {
		t.Fatal(err)
	}
	if d.Parts != 3 || parts != 3 {
		t.Fatalf("a 1001-line document must be handed in 3 parts, got %d", d.Parts)
	}
	if len(d.Facts) != 1 || d.Facts[0].Line != 1001 || d.Dropped != 2 {
		t.Fatalf("facts %+v dropped %d", d.Facts, d.Dropped)
	}
	inv := knowledgeInventory(root)
	if !contains(inv, "docs/ESTADO.md") || !contains(inv, "docs/AUDITORIA.md") || !contains(inv, "CLAUDE.md") {
		t.Fatalf("inventory %v", inv)
	}
}

func TestDiagnosisMessagesShownInPortuguese(t *testing.T) {
	en := "[infuse] Diagnosis refused:\n- evidence 1: you read 3 of 10 lines of docs/A.md; read it to the end first (Read offset=4)"
	want := "[infuse] Diagnóstico recusado:\n- evidência 1: você leu 3 de 10 linhas de docs/A.md; leia até o fim antes (Read offset=4)"
	if got := localize("pt-BR", en); got != want {
		t.Fatalf("%q", got)
	}
}
