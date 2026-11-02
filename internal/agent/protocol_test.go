package agent

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
	s.gate.diagnosed = true // this test is about another gate
	noContract(s)
	s.Mode = "bypass" // the protocol holds even when the user accepts everything
	off := false
	s.settings.RequireReview = &off
	run := func(b Block) (string, bool) { return s.runTool(context.Background(), &RuleSet{}, b) }

	// 1. no plan, no write
	if out, isErr := run(writeCall("1", "sum.js", "module.exports = (a, b) => a + b;\n")); !isErr || out != M("protocol.no_plan").String() {
		t.Fatalf("write without a plan: %s", out)
	}
	// a plan whose open task has no proof is refused
	if out, isErr := run(Block{Type: "tool_use", ID: "2", Name: "TodoWrite", Input: json.RawMessage(`{"tasks":[{"id":"1","text":"create sum","status":"pending"}]}`)}); !isErr || !strings.Contains(out, "needs a proof") {
		t.Fatalf("plan without proof accepted: %s", out)
	}
	if out, isErr := run(Block{Type: "tool_use", ID: "3", Name: "TodoWrite", Input: json.RawMessage(`{"tasks":[{"id":"1","text":"create sum","status":"in_progress","proof":"node --test"}]}`)}); isErr {
		t.Fatal(out)
	}
	if out, isErr := run(writeCall("4", "sum.js", "module.exports = (a, b) => a + b;\n")); isErr {
		t.Fatalf("write with a plan refused: %s", out)
	}

	// 2. a library or external API not researched in this turn: refused before the first error
	code := "const axios = require('axios');\nmodule.exports = () => axios.get('https://api.frankfurter.app/latest');\n"
	out, isErr := run(writeCall("5", "exchange-rate.js", code))
	if !isErr || out != M("protocol.docs_first", "uses", "api.frankfurter.app, axios").String() {
		t.Fatalf("unresearched library and API accepted: %s", out)
	}
	s.noteResearch("WebSearch", json.RawMessage(`{"query":"axios get request documentation"}`))
	s.noteResearch("WebFetch", json.RawMessage(`{"url":"https://frankfurter.dev/"}`))
	if out, isErr := run(writeCall("6", "exchange-rate.js", code)); isErr {
		t.Fatalf("refused after research: %s", out)
	}
	// built-ins and relative imports need no research
	if out, isErr := run(writeCall("7", "read.js", "const fs = require('fs');\nconst sum = require('./sum');\nconst { test } = require('node:test');\n")); isErr {
		t.Fatal(out)
	}

	// 3. size limit
	big := strings.Repeat("const x = 1;\n", 450)
	if out, isErr := run(writeCall("8", "big.js", big)); !isErr || out != M("protocol.file_too_big", "file", "big.js", "lines", 451, "limit", 400).String() {
		t.Fatalf("oversized file accepted: %s", out)
	}

	// 4. the turn cannot end with the plan open
	checkTurnObligations(t, s, root)
}

// The project's instruction files are never edited by the agent, even though they are prose.
func TestInstructionFilesAreRefusedBeforeTheProseExemption(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	in, _ := json.Marshal(map[string]string{"path": "CLAUDE.md", "content": "# new rules\n"})
	if why := s.protocolCheckWrite("Write", in); why.ID != "instr.no_edit" {
		t.Fatalf("a written instruction file passed: %q", why.Render("en"))
	}
}

// Research counts only when the call succeeded and names the dependency as a whole name; a project
// file named after it is not its documentation.
func TestResearchMustNameTheDependency(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	call := func(name, field, value string) Block {
		in, _ := json.Marshal(map[string]string{field: value})
		return Block{Type: "tool_use", ID: "r", Name: name, Input: in}
	}
	research := func(b Block, ok bool) { s.finishToolOutput(context.Background(), b, nil, "result", !ok, nil) }

	research(call("WebSearch", "query", "regular expression syntax"), false)
	if s.researchedFor("express") {
		t.Fatal("a failed search for another subject counted as express documentation")
	}
	research(call("WebSearch", "query", "forms over data"), false)
	if s.researchedFor("ms") {
		t.Fatal("ms matched inside forms")
	}
	research(call("WebSearch", "query", "express middleware documentation"), true)
	if !s.researchedFor("express") {
		t.Fatal("a search that names express was not counted")
	}
	research(call("Read", "path", "src/lodash.js"), true)
	if s.researchedFor("lodash") {
		t.Fatal("a project file named after the package counted as its documentation")
	}
	research(call("Read", "path", "node_modules/lodash/index.js"), true)
	if !s.researchedFor("lodash") {
		t.Fatal("the package's own source was not counted")
	}
	research(call("WebSearch", "query", "github actions"), true)
	if s.researchedHost("api.github.com") {
		t.Fatal("an unrelated github search counted as the API documentation")
	}
	research(call("WebFetch", "url", "https://docs.github.com/en/rest"), false)
	if s.researchedHost("api.github.com") {
		t.Fatal("a failed fetch counted as documentation")
	}
	research(call("WebFetch", "url", "https://docs.github.com/en/rest"), true)
	if !s.researchedHost("api.github.com") {
		t.Fatal("the service documentation was not counted")
	}
}

func checkTurnObligations(t *testing.T, s *Session, root string) {
	t.Helper()
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "create sum"}}}}
	if why := s.turnObligation(context.Background()); why.ID != "turn.open_tasks" {
		t.Fatalf("turn ended with an open plan: %q", why)
	}
	s.setTasks([]TaskItem{{ID: "1", Text: "create sum", Status: "done"}})
	s.Checkpoints[1] = []CheckpointFile{{Path: "broken.json", New: true}}
	_ = os.WriteFile(filepath.Join(root, "broken.json"), []byte(`{"a": }`), 0o644)
	if why := s.turnObligation(context.Background()); why.ID != "turn.checker" {
		t.Fatalf("turn ended with a broken file: %q", why)
	}
	_ = os.WriteFile(filepath.Join(root, "broken.json"), []byte(`{"a": 1}`), 0o644)
	if why := s.turnObligation(context.Background()); why.ID != "gate.no_report" {
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
	if why := conventionViolation(root, "src/new-file.js", "", tabs); why.String() != M("conv.indent", "want", M("conv.indent_2"), "share", 100, "got", M("conv.indent_tabs")).String() {
		t.Fatalf("tabs in a 2-space project: %q", why)
	}
	esm := "import a from './a';\nexport function g() {\n  if (a) {\n    return 1;\n  }\n  return 2;\n}\n"
	if why := conventionViolation(root, "src/other-file.js", "", esm); why.String() != M("conv.modules", "system", "CommonJS (require/module.exports)").String() {
		t.Fatalf("ESM in a CommonJS project: %q", why)
	}
	good := "const a = require('./a');\nfunction g() {\n  if (a) {\n    return 1;\n  }\n  return 2;\n}\nmodule.exports = g;\n"
	if why := conventionViolation(root, "src/one-more.js", "", good); !why.Empty() {
		t.Fatalf("a file that follows the conventions was refused: %q", why)
	}
	if why := conventionViolation(root, "src/oneMore.js", "", good); why.String() != M("conv.naming", "ext", ".js", "want", "kebab-case", "file", "oneMore.js", "got", "camelCase").String() {
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
