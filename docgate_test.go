package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandOptionsNeedReadDocs(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"
	run := func(b Block) (string, bool) { return s.runTool(context.Background(), &RuleSet{}, b) }

	// options of a specialized program, never read about: refused, each one named
	chrome := `"C:/Program Files/Google/Chrome/Application/chrome.exe" --gpu-startup-dialog --no-sandbox --enable-unsafe-webgpu about:blank`
	out, isErr := run(bashCall("1", chrome))
	if !isErr || !strings.Contains(out, "--gpu-startup-dialog") || !strings.Contains(out, "--no-sandbox") || !strings.Contains(out, "chrome") {
		t.Fatalf("unread options accepted: %s", out)
	}
	// options inside an environment assignment belong to the program too
	if why := s.commandDocCheck(`RENDERDOC_HOOK=--capture-all-children renderdoccmd capture app.exe`); why.ID != "docs.cmd_options" || why.Args["options"] != "--capture-all-children" {
		t.Fatalf("option in an assignment missed: %v", why)
	}
	// everyday tooling, help commands and the project's own programs pass
	for _, c := range []string{"git log --oneline --stat", "go test -count=1 ./...", "chrome --help", "build/infuse.exe --window-hidden"} {
		if why := s.commandDocCheck(c); !why.Empty() {
			t.Fatalf("%s refused: %s", c, why)
		}
	}
	// reading the docs (a pinned doc read in full) opens exactly what it describes
	docs := filepath.Join(root, ".infuse", "docs")
	_ = os.MkdirAll(docs, 0o755)
	_ = os.WriteFile(filepath.Join(docs, "dawn-debugging.md"), []byte("Run Chrome with --gpu-startup-dialog to attach a debugger, and --no-sandbox when injecting.\n"), 0o644)
	if out, isErr := run(Block{Type: "tool_use", ID: "2", Name: "Read", Input: json.RawMessage(`{"path":".infuse/docs/dawn-debugging.md"}`)}); isErr {
		t.Fatal(out)
	}
	if why := s.commandDocCheck(chrome); why.Args["options"] != "--enable-unsafe-webgpu" {
		t.Fatalf("after reading, only the unread option should remain: %v", why)
	}
	// infuse's own notes are not the user's words, whatever they mention
	s.Messages = append(s.Messages, Message{Role: "user", Kind: "infuse_note", Content: []Block{{Type: "text", Text: "about --enable-unsafe-webgpu"}}})
	if why := s.commandDocCheck(chrome); why.Empty() {
		t.Fatal("an infuse note counted as the user's request")
	}
	// an option the user asked for is the user's choice
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: "rode com --enable-unsafe-webgpu"}}})
	if why := s.commandDocCheck(chrome); !why.Empty() {
		t.Fatalf("user's option refused: %s", why)
	}
	// a project script documents its own options
	_ = os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"e2e":"playwright test --headed"}}`), 0o644)
	if why := s.commandDocCheck("playwright test --headed"); !why.Empty() {
		t.Fatalf("project-documented option refused: %s", why)
	}
}

func TestPartlyReadPageBlocksAction(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"
	s.docs.markPart("https://dawn.googlesource.com/dawn/+/HEAD/docs/dawn/debugging.md", 0, 3)
	page := "https://dawn.googlesource.com/dawn/+/HEAD/docs/dawn/debugging.md"
	why := s.commandDocCheck("go build ./...")
	if want := M("docs.cmd_partial", "pages", []Msg{M("docs.part", "url", page, "seen", 1, "total", 3, "offset", 30000)}); why.String() != want.String() {
		t.Fatalf("skimmed page did not block: %q", why)
	}
	if why := s.commandDocCheck("curl -s https://dawn.googlesource.com/dawn/+/HEAD/docs/dawn/debugging.md?format=TEXT"); !why.Empty() {
		t.Fatalf("reading command blocked: %s", why)
	}
	s.gate.planned = true
	out, isErr := s.runTool(context.Background(), &RuleSet{}, writeCall("1", "a.js", "x\n"))
	if want := M("docs.write_partial", "pages", s.docs.unfinished()); !isErr || out != want.String() {
		t.Fatalf("write while a page is half read: %s", out)
	}
	// the user reads the refusal in their language, from the message recorded with the call
	if d := displayFor("pt-BR", out, s.takeCallMsgs("1")); !strings.HasPrefix(d, "[infuse] Alteração recusada") {
		t.Fatalf("display %q", d)
	}
	s.docs.markPart("https://dawn.googlesource.com/dawn/+/HEAD/docs/dawn/debugging.md", 1, 3)
	s.docs.markPart("https://dawn.googlesource.com/dawn/+/HEAD/docs/dawn/debugging.md", 2, 3)
	if why := s.commandDocCheck("go build ./..."); !why.Empty() {
		t.Fatalf("fully read page still blocks: %s", why)
	}
}

func TestPlanOfEarlierTurnHoldsAndClosesHonestly(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.gate.diagnosed = true // this test is about another gate
	off := false
	s.settings.RequireReview = &off
	s.setTasks([]TaskItem{{ID: "1", Text: "ler a doc", Status: "in_progress", Proof: "página lida"}, {ID: "2", Text: "criar cli", Status: "pending", Proof: "node cli.js"}, {ID: "3", Text: "feito", Status: "done"}})
	s.gate.reset() // a later turn (a mission's next round): the plan was registered before it
	if why := s.turnObligation(context.Background()); why.String() != M("turn.open_tasks", "tasks", "ler a doc; criar cli").String() {
		t.Fatalf("an earlier turn's plan was ignored: %q", why)
	}
	s.gate.obligationAsks = 6 // the agent kept refusing: the turn ends, and the plan says so
	if why := s.turnObligation(context.Background()); !why.Empty() {
		t.Fatal(why)
	}
	for _, x := range s.Tasks {
		if x.ID != "3" && (x.Status != "blocked" || x.Note == "") {
			t.Fatalf("open task left as ongoing: %+v", x)
		}
	}
	// the model cannot write infuse's note
	s.gate.planned = false
	if out, isErr := (todoTool{}).Run(context.Background(), &ToolContext{S: s, Root: s.Root}, json.RawMessage(`{"tasks":[{"id":"1","text":"x","status":"done","note":"aprovado pelo infuse"}]}`)); isErr {
		t.Fatal(out)
	}
	if s.Tasks[0].Note != "" {
		t.Fatal("model wrote the note")
	}
}
