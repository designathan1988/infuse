package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryToolAndPrompt(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	tool := memoryTool{}
	tc := &ToolContext{S: s, Root: root, Cwd: root}
	run := func(in string) string {
		out, isErr := tool.Run(context.Background(), tc, json.RawMessage(in))
		if isErr {
			t.Fatalf("%s → %s", in, out)
		}
		return out
	}
	run(`{"action":"add","type":"fato","text":"o projeto usa Node 22"}`)
	run(`{"action":"add","type":"correcao","text":"use pnpm, não npm","priority":3}`)
	run(`{"action":"add","type":"decisao","text":"CommonJS","scope":"session"}`)
	run(`{"action":"add","type":"comando","text":"go test ./...","scope":"global"}`)
	p := memoryPrompt(s)
	if !strings.HasPrefix(p, "- [user correction · project] use pnpm") {
		t.Fatalf("highest priority first:\n%s", p)
	}
	for _, want := range []string{"[decision · session] CommonJS", "[command that worked · global] go test", "[fact · project] o projeto usa Node 22"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in\n%s", want, p)
		}
	}
	if !strings.Contains(buildSystemPrompt(s, &RuleSet{}), "use pnpm, não npm") {
		t.Fatal("memory not in the system prompt")
	}
	id := loadMemory(projectMemoryFile(root))[0].ID
	run(`{"action":"remove","id":"` + id + `"}`)
	if strings.Contains(memoryPrompt(s), "Node 22") {
		t.Fatal("not removed")
	}

	// the budget: low-priority items that do not fit are counted, not silently dropped
	for i := 0; i < 40; i++ {
		run(`{"action":"add","type":"fato","text":"` + strings.Repeat("x", 300) + `"}`)
	}
	if p := memoryPrompt(s); len(p) > memoryBudget+200 || !strings.Contains(p, "did not fit") || !strings.Contains(p, "use pnpm") {
		t.Fatalf("budget: %d chars", len(p))
	}
}

func TestAgentCannotWriteMemoryFolderDirectly(t *testing.T) {
	root, g := bypassRoot(t)
	for _, cmd := range []string{"echo '[]' > .infuse/memory/items.json", "rm -rf .infuse/memory"} {
		out, _, err := g.GuardCommand(cmd, root)
		if err == nil && !strings.Contains(out, "/dev/null") {
			t.Fatalf("not refused: %q → %q", cmd, out)
		}
	}
}

func TestCompactionKeepsUserWordsVerbatim(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "[Contexto compactado — resumo]\n\nresumo\n\n## Pedidos do usuário, literais\n> primeiro pedido\n> em duas linhas"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "ok"}}},
		{Role: "user", Content: []Block{{Type: "text", Text: "NÃO use npm, use pnpm"}}},
		{Role: "user", Content: []Block{{Type: "tool_result", Content: "saída"}}},
	}
	v := verbatimRequests(msgs)
	if v != "> primeiro pedido\n> em duas linhas\n\n> NÃO use npm, use pnpm" {
		t.Fatalf("%q", v)
	}
}
