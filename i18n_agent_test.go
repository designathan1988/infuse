package main

import "testing"

func TestLocalizeByTemplate(t *testing.T) {
	registerTranslations("pt-BR", map[string]string{
		"[infuse] Blocked: %s would have %d lines (project limit: %d).": "[infuse] Bloqueado: %s ficaria com %d linhas (limite do projeto: %d).",
	})
	en := "[infuse] Blocked: src/a.ts would have 515 lines (project limit: 400)."
	if got := localize("pt-BR", en); got != "[infuse] Bloqueado: src/a.ts ficaria com 515 linhas (limite do projeto: 400)." {
		t.Fatal(got)
	}
	if got := localize("en", en); got != en {
		t.Fatal(got)
	}
	mixed := "Arquivo escrito: x\n" + en // tool output followed by a note: only the note is translated
	if got := localize("pt-BR", mixed); got != "Arquivo escrito: x\n[infuse] Bloqueado: src/a.ts ficaria com 515 linhas (limite do projeto: 400)." {
		t.Fatal(got)
	}
	if localize("pt-BR", "untranslated text") != "untranslated text" {
		t.Fatal("changed a text with no translation")
	}
}
