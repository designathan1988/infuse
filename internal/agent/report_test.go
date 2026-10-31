package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportClaimsCheckedAgainstTheTurn(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "old.js"), []byte("x\n"), 0o644)
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.Checkpoints[1] = []CheckpointFile{{Path: "a.js", New: true}}
	s.Messages = []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "faça"}}},
		{Role: "assistant", Content: []Block{
			{Type: "tool_use", ID: "1", Name: "Bash", Input: json.RawMessage(`{"command":"npm  test"}`)},
			{Type: "tool_use", ID: "2", Name: "Bash", Input: json.RawMessage(`{"command":"npm run build"}`)},
			{Type: "tool_use", ID: "3", Name: "WebSearch", Input: json.RawMessage(`{"query":"vite config"}`)},
		}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "1", Content: "ok"}, {Type: "tool_result", ToolUseID: "2", Content: "exit 1\nerro TS2304", IsError: true}, {Type: "tool_result", ToolUseID: "3", Content: "…"}}},
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "R", Name: "Report"}}},
	}
	s.inflight = []Block{} // a command whose result is still in flight counts too
	in := `{"summary":"feito","claims":[
		{"text":"testes passam","kind":"command","ref":"npm test"},
		{"text":"build ok","kind":"command","ref":"npm run build"},
		{"text":"lint ok","kind":"command","ref":"npm run lint"},
		{"text":"criei a.js","kind":"file","ref":"a.js"},
		{"text":"mexi em old.js","kind":"file","ref":"old.js"},
		{"text":"ficou mais rápido","kind":"other"}]}`
	out, isErr := reportTool{}.Run(context.Background(), &ToolContext{CallID: "R", S: s, Root: root, Cwd: root}, json.RawMessage(in))
	if isErr {
		t.Fatal(out)
	}
	claim := func(status, text string, ev Msg) string {
		return M("report.claim", "status", claimStatus(status), "text", text, "evidence", ev).String()
	}
	for _, want := range []string{
		claim("confirmado", "testes passam", M("report.ev_ran")),
		claim("nao-confirmado", "build ok", M("report.ev_failed", "output", "exit 1")),
		claim("nao-confirmado", "lint ok", M("report.ev_not_run")),
		claim("confirmado", "criei a.js", M("report.ev_changed")),
		claim("parcial", "mexi em old.js", M("report.ev_unchanged")),
		claim("sem-evidencia", "ficou mais rápido", M("report.ev_none")),
		M("report.source", "source", M("report.src_search", "query", "vite config")).String(),
		M("report.unconfirmed", "count", 2).String(),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	// the screen reads the check from data attached to the call, never from the text
	var r TurnReport
	if err := json.Unmarshal(s.Messages[3].Content[0].Meta, &r); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range r.Claims {
		got = append(got, c.Status)
	}
	if strings.Join(got, ",") != "confirmado,nao-confirmado,nao-confirmado,confirmado,parcial,sem-evidencia" || r.Unconfirmed != 2 || r.CallID != "R" {
		t.Fatalf("%+v", r)
	}
	if r.Claims[1].Display != M("report.ev_failed", "output", "exit 1").Render(s.uiLang()) || len(r.Sources) != 1 || r.Sources[0] != (ReportSource{"search", "vite config"}) {
		t.Fatalf("%+v", r)
	}
	// the user reads the same check in their language: the messages recorded with the call replace the English
	if d := displayFor("pt-BR", out, s.takeCallMsgs("R")); !strings.Contains(d, "- [não confirmado] build ok — executado neste turno, mas falhou: exit 1") || strings.Contains(d, "run in this turn") {
		t.Fatal(d)
	}
}
