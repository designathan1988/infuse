package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCall(id, path, content string) Block {
	in, _ := json.Marshal(map[string]string{"path": path, "content": content})
	return Block{Type: "tool_use", ID: id, Name: "Write", Input: in}
}

func TestProtocolDecidesNotTheModel(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass" // the protocol holds even when the user accepts everything
	off := false
	s.settings.RequireReview = &off
	run := func(b Block) (string, bool) { return s.runTool(context.Background(), &RuleSet{}, b) }

	// 1. no plan, no write
	if out, isErr := run(writeCall("1", "soma.js", "module.exports = (a, b) => a + b;\n")); !isErr || !strings.Contains(out, "registre o plano") {
		t.Fatalf("write without a plan: %s", out)
	}
	// a plan whose open task has no proof is refused
	if out, isErr := run(Block{Type: "tool_use", ID: "2", Name: "TodoWrite", Input: json.RawMessage(`{"tasks":[{"id":"1","text":"criar soma","status":"pending"}]}`)}); !isErr || !strings.Contains(out, "precisa de proof") {
		t.Fatalf("plan without proof accepted: %s", out)
	}
	if out, isErr := run(Block{Type: "tool_use", ID: "3", Name: "TodoWrite", Input: json.RawMessage(`{"tasks":[{"id":"1","text":"criar soma","status":"in_progress","proof":"node --test"}]}`)}); isErr {
		t.Fatal(out)
	}
	if out, isErr := run(writeCall("4", "soma.js", "module.exports = (a, b) => a + b;\n")); isErr {
		t.Fatalf("write with a plan refused: %s", out)
	}

	// 2. a library or external API not researched in this turn: refused before the first error
	code := "const axios = require('axios');\nmodule.exports = () => axios.get('https://api.frankfurter.app/latest');\n"
	out, isErr := run(writeCall("5", "cotacao.js", code))
	if !isErr || !strings.Contains(out, "axios") || !strings.Contains(out, "api.frankfurter.app") {
		t.Fatalf("unresearched library and API accepted: %s", out)
	}
	s.noteResearch("WebSearch", json.RawMessage(`{"query":"axios get request documentation"}`))
	s.noteResearch("WebFetch", json.RawMessage(`{"url":"https://frankfurter.dev/"}`))
	if out, isErr := run(writeCall("6", "cotacao.js", code)); isErr {
		t.Fatalf("refused after research: %s", out)
	}
	// built-ins and relative imports need no research
	if out, isErr := run(writeCall("7", "ler.js", "const fs = require('fs');\nconst soma = require('./soma');\nconst { test } = require('node:test');\n")); isErr {
		t.Fatal(out)
	}

	// 3. size limit
	big := strings.Repeat("const x = 1;\n", 450)
	if out, isErr := run(writeCall("8", "grande.js", big)); !isErr || !strings.Contains(out, "limite do projeto: 400") {
		t.Fatalf("oversized file accepted: %s", out)
	}

	// 4. the turn cannot end with the plan open
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "crie soma"}}}}
	if why := s.turnObligation(context.Background()); !strings.Contains(why, "tarefas abertas") {
		t.Fatalf("turn ended with an open plan: %q", why)
	}
	s.setTasks([]TaskItem{{ID: "1", Text: "criar soma", Status: "done"}})
	s.Checkpoints[1] = []CheckpointFile{{Path: "quebrado.json", New: true}}
	_ = os.WriteFile(filepath.Join(root, "quebrado.json"), []byte(`{"a": }`), 0o644)
	if why := s.turnObligation(context.Background()); !strings.Contains(why, "verificador do projeto") {
		t.Fatalf("turn ended with a broken file: %q", why)
	}
	_ = os.WriteFile(filepath.Join(root, "quebrado.json"), []byte(`{"a": 1}`), 0o644)
	if why := s.turnObligation(context.Background()); !strings.Contains(why, "sem Report") {
		t.Fatalf("turn ended without a report: %q", why)
	}
}

func TestConventionsAreEnforcedOnWrites(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	for _, n := range []string{"user-list", "order-form", "api-client", "cart-total", "price-tag"} {
		_ = os.WriteFile(filepath.Join(root, "src", n+".js"), []byte("const a = require('./a');\nfunction f() {\n  if (a) {\n    return 1;\n  }\n  return 2;\n}\nmodule.exports = f;\n"), 0o644)
	}
	LearnConventions(root)
	tabs := "const a = require('./a');\nfunction g() {\n\tif (a) {\n\t\treturn 1;\n\t}\n\tconst b = 2;\n\tconst c = 3;\n\treturn b + c;\n}\nmodule.exports = g;\n"
	if why := conventionViolation(root, "src/novo-arquivo.js", "", tabs); !strings.Contains(why, "2 espaços") {
		t.Fatalf("tabs in a 2-space project: %q", why)
	}
	esm := "import a from './a';\nexport function g() {\n  if (a) {\n    return 1;\n  }\n  return 2;\n}\n"
	if why := conventionViolation(root, "src/outro-arquivo.js", "", esm); !strings.Contains(why, "CommonJS") {
		t.Fatalf("ESM in a CommonJS project: %q", why)
	}
	good := "const a = require('./a');\nfunction g() {\n  if (a) {\n    return 1;\n  }\n  return 2;\n}\nmodule.exports = g;\n"
	if why := conventionViolation(root, "src/mais-um.js", "", good); why != "" {
		t.Fatalf("a file that follows the conventions was refused: %q", why)
	}
	if why := conventionViolation(root, "src/maisUm.js", "", good); !strings.Contains(why, "kebab-case") {
		t.Fatalf("camelCase name in a kebab-case project: %q", why)
	}
}

func TestExternalUsesFound(t *testing.T) {
	u := externalUses("a.js", "import x from 'react';\nimport y from '@tanstack/query/core';\nconst z = require('lodash/fp');\nconst p = require('path');\nfetch('https://api.github.com/x');\nfetch('http://localhost:3000')")
	for _, want := range []string{"pkg:react", "pkg:@tanstack/query", "pkg:lodash", "host:api.github.com"} {
		if !u[want] {
			t.Fatalf("missing %s in %v", want, u)
		}
	}
	if u["pkg:path"] || u["host:localhost"] {
		t.Fatal(u)
	}
	if py := externalUses("a.py", "import os\nfrom requests import get\nimport numpy as np"); !py["pkg:requests"] || !py["pkg:numpy"] || py["pkg:os"] {
		t.Fatal(py)
	}
	if researchName("host:api.frankfurter.app") != "frankfurter" || researchName("pkg:github.com/spf13/cobra") != "cobra" {
		t.Fatal("research names")
	}
}
