package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestPtProtocolMessages(t *testing.T) {
	// a size refusal
	en := fmt.Sprintf("[infuse] Blocked: %s would have %d lines (project limit: %d). Split the responsibility into smaller files within the existing structure.", "src/a.ts", 515, 400)
	if got := localize("pt-BR", en); got != "[infuse] Bloqueado: src/a.ts ficaria com 515 linhas (limite do projeto: 400). Divida a responsabilidade em arquivos menores dentro da estrutura existente." {
		t.Fatal(got)
	}

	// an architecture message: the frame and, as its own paragraph, the reason
	why := fmt.Sprintf("the function %s (%s:%d) would have %d lines (limit %d). A function does one thing: extract the steps into functions whose names say what they do.", "calcular", "src/b.ts", 1, 72, 60)
	arch := fmt.Sprintf("[infuse] Architecture: %s Project rules: .infuse/architecture.json.", why)
	if got := localize("pt-BR", arch); !strings.HasPrefix(got, "[infuse] Arquitetura: ") || !strings.HasSuffix(got, " Regras do projeto: .infuse/architecture.json.") || !strings.Contains(got, "calcular (src/b.ts:1)") {
		t.Fatal(got)
	}
	if got := localize("pt-BR", why); got != "a função calcular (src/b.ts:1) ficaria com 72 linhas (limite 60). Uma função faz uma coisa: extraia as etapas em funções com nomes que digam o que fazem." {
		t.Fatal(got)
	}

	// a refusal with an inner reason (a convention), and the reason on its own
	reason := fmt.Sprintf("the project indents with %s (%.0f%% of the files) and this code uses %s.", "2 spaces", 95.0, "tabs")
	conv := fmt.Sprintf("[infuse] Blocked: %s Rewrite it following the project's convention (.infuse/conventions.md).", reason)
	if got := localize("pt-BR", conv); got != "[infuse] Bloqueado: o projeto indenta com 2 spaces (95% dos arquivos) e este código usa tabs. Reescreva seguindo a convenção do projeto (.infuse/conventions.md)." { // the inner reason is translated too
		t.Fatal(got)
	}
	if got := localize("pt-BR", reason); got != "o projeto indenta com 2 spaces (95% dos arquivos) e este código usa tabs." {
		t.Fatal(got)
	}

	// a multi-line gate message, and a Browser result
	gate := fmt.Sprintf("[infuse] Blocked: %d attempts failed in a row. Before another attempt, read the code involved (Read, Grep) or consult a source (WebSearch, WebFetch, pinned documentation); then explain the cause, and only then fix it.\nRecent errors:\n- %s", 2, "module not found\n- exit 1")
	if got := localize("pt-BR", gate); !strings.HasPrefix(got, "[infuse] Bloqueado: 2 tentativas falharam seguidas.") || !strings.HasSuffix(got, "Erros recentes:\n- module not found\n- exit 1") {
		t.Fatal(got)
	}
	click := fmt.Sprintf("clicked <%s> %q · now at %s", "button", "Salvar", "http://localhost:5173/ — App")
	if got := localize("pt-BR", click); got != `clicou em <button> "Salvar" · agora em http://localhost:5173/ — App` {
		t.Fatal(got)
	}
}

// Every translation keeps the verbs of its English format, and no two formats claim the same message
// (localize takes the first that matches, in no fixed order).
func TestPtProtocolMessagesAreUnambiguous(t *testing.T) {
	compiled := map[string]func(string) bool{}
	for en := range ptProtocolMessages {
		re := templateRegex(en)
		if re == nil {
			t.Fatalf("no pattern for %q", en)
		}
		compiled[en] = re.MatchString
	}
	for en, pt := range ptProtocolMessages {
		if a, b := strings.Join(reVerb.FindAllString(en, -1), " "), strings.Join(reVerb.FindAllString(pt, -1), " "); a != b {
			t.Errorf("verbs differ: %q has %q, %q has %q", en, a, pt, b)
		}
		i := 0
		sample := reVerb.ReplaceAllStringFunc(en, func(v string) string {
			if v == "%%" {
				return "%"
			}
			i++
			return fmt.Sprintf("«%d»", i)
		})
		for other, match := range compiled {
			if other != en && match(sample) {
				t.Errorf("%q also matches the message of %q", other, en)
			}
		}
	}
}
