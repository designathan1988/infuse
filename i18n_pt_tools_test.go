package main

import (
	"fmt"
	"testing"
)

func TestToolMessagesShownInPortuguese(t *testing.T) {
	cases := []struct{ en, pt string }{
		{fmt.Sprintf("[infuse] the background process %s (%s) finished on its own, exit code %d. Read its output with BashOutput.", "p1", "npm run dev", 1),
			"[infuse] o processo em segundo plano p1 (npm run dev) terminou sozinho, código 1. Leia a saída com BashOutput."},
		{fmt.Sprintf("process %s running in the background (pid %d). Read the new output with BashOutput(id=%q); stop it with KillShell. The user follows it in the Processes pane.", "p2", 4242, "p2"),
			`processo p2 em segundo plano (pid 4242), rodando. Leia a saída nova com BashOutput(id="p2"); encerre com KillShell. O usuário acompanha no painel Processos.`},
		{fmt.Sprintf("plan refused: every open task needs a proof (the command, the page or the file that will prove it works). Without proof: %s", "write the parser; test it"),
			"plano recusado: cada tarefa aberta precisa de proof (o comando, a página ou o arquivo que comprovará que funciona). Sem proof: write the parser; test it"},
		{fmt.Sprintf("Edited %s: %d edits, %d replacement(s)", "a.go", 2, 3), "Editado a.go: 2 edições, 3 substituição(ões)"},
		{fmt.Sprintf("Edited: %s (%d replacement(s))", "a.go", 1), "Editado: a.go (1 substituição(ões))"},
	}
	for _, c := range cases {
		if got := localize("pt-BR", c.en); got != c.pt {
			t.Errorf("localize(%q)\n got %q\nwant %q", c.en, got, c.pt)
		}
		if got := localize("en", c.en); got != c.en {
			t.Errorf("English changed: %q", got)
		}
	}
	// a tool output whose first line is infuse's and the rest the project's: only infuse's line changes
	out := fmt.Sprintf("File written: %s (%d bytes)", "src/a.ts", 120) + "\nexport const a = 1"
	if got := localize("pt-BR", out); got != "Arquivo escrito: src/a.ts (120 bytes)\nexport const a = 1" {
		t.Fatal(got)
	}
}
