package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tools' messages reach the model in English and the user in pt-BR: each message is recorded with the
// call that produced it, and the user's version replaces infuse's exact text, leaving the project's data
// (file contents, command output) as it is.
func TestToolMessagesShownInTheUsersLanguage(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	run := func(tool Tool, id, in string) (string, string) {
		t.Helper()
		out, _ := tool.Run(context.Background(), &ToolContext{CallID: id, S: s, Root: root, Cwd: root, Rules: &RuleSet{}}, json.RawMessage(in))
		return out, displayFor("pt-BR", out, s.takeCallMsgs(id))
	}

	out, pt := run(todoTool{}, "1", `{"tasks":[{"id":"1","text":"write the parser","status":"pending"},{"id":"2","text":"test it","status":"pending"}]}`)
	if out != M("plan.no_proof", "tasks", "write the parser; test it").String() || !strings.HasPrefix(out, "plan refused:") {
		t.Fatal(out)
	}
	if pt != M("plan.no_proof", "tasks", "write the parser; test it").Render("pt-BR") {
		t.Fatal(pt)
	}

	// a message inside a message is written in the same language (the list of processes: none)
	out, pt = run(bashOutputTool{}, "2", `{"id":"p9"}`)
	if out != "process not found: p9. Processes in this session: none" || pt != M("proc.not_found", "id", "p9", "list", M("proc.none")).Render("pt-BR") {
		t.Fatalf("%q / %q", out, pt)
	}

	// infuse's line changes, the project's data does not
	long := strings.Repeat("export const a = '"+strings.Repeat("x", 100)+"'\n", 500) // read in parts
	_ = os.WriteFile(filepath.Join(root, "a.ts"), []byte(long), 0o644)
	out, pt = run(readTool{}, "3", `{"path":"a.ts"}`)
	if !strings.Contains(out, "export const a = 'xxx") || !strings.Contains(out, "[continues: the file has 501 lines; read on from here with Read offset=") {
		t.Fatal(out[len(out)-300:])
	}
	idx := strings.Index(out, "Read offset=")
	if idx < 0 {
		t.Fatal(out[len(out)-300:])
	}
	i := idx + len("Read offset=")
	j := i
	for j < len(out) && out[j] >= '0' && out[j] <= '9' {
		j++
	}
	if !strings.Contains(pt, "export const a = 'xxx") || !strings.Contains(pt, M("read.continues", "lines", 501, "offset", out[i:j]).Render("pt-BR")) || strings.Contains(pt, "read on from here") {
		t.Fatal(pt[len(pt)-300:])
	}

	// a refusal that travels as an error keeps its message: the path refusals
	out, pt = run(writeTool{}, "4", `{"path":"../outside.txt","content":"x"}`)
	if out != M("path.outside", "path", "../outside.txt").String() || pt != M("path.outside", "path", "../outside.txt").Render("pt-BR") {
		t.Fatalf("%q / %q", out, pt)
	}

	// an English reader gets the model's text and no separate version
	if displayFor("en", out, []Msg{M("path.outside", "path", "../outside.txt")}) != "" {
		t.Fatal("English has no display")
	}
}

func TestMessageErrorsCarryTheirMessage(t *testing.T) {
	inner := errMsg(M("mcp.timeout"))
	m, ok := msgOf(inner)
	if !ok || m.ID != "mcp.timeout" || inner.Error() != "the server did not answer within 120 s" {
		t.Fatal(m, inner)
	}
	outer := M("mcp.call_failed", "name", "github", "error", errArg(inner))
	if outer.Render("pt-BR") != M("mcp.call_failed", "name", "github", "error", M("mcp.timeout")).Render("pt-BR") {
		t.Fatal(outer.Render("pt-BR"))
	}
	if userError("pt-BR", M("fork.no_message")).Error() != M("fork.no_message").Render("pt-BR") {
		t.Fatal("an error answered to the screen is in the user's language")
	}
	if errArg(os.ErrNotExist) != os.ErrNotExist.Error() {
		t.Fatal("an error that is not infuse's is kept as it is")
	}
}
