package main

// The meaning of the project's instructions — in any language, any wording — is compiled once into
// structured rules by a model, and every rule must carry the exact quote it comes from. Code then checks
// each quote against the instruction text word for word (spaces normalized): a rule whose quote is not
// there was invented and is dropped. The rules are stored in .infuse/project-rules.json (the agent reads
// it, never writes it), shown to the user, who can switch any off; enforcement over them is mechanical.
// They are compiled again whenever the instruction files change.

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type compiledRule struct {
	Kind     string `json:"kind"`               // reference | read | verify
	Path     string `json:"path,omitempty"`     // reference, read
	Command  string `json:"command,omitempty"`  // verify
	When     string `json:"when,omitempty"`     // verify: after-change | before-commit | on-request
	Quote    string `json:"quote"`              // the exact words of the instructions
	Disabled bool   `json:"disabled,omitempty"` // switched off by the user
}

type compiledRules struct {
	Hash     string         `json:"hash"` // of the instruction text compiled
	Compiled int64          `json:"compiled"`
	Model    string         `json:"model"`
	Rules    []compiledRule `json:"rules"`
	Dropped  []Msg          `json:"rejected,omitempty"` // proposals rejected by the quote check, with why (as messages)
}

func compiledRulesFile(root string) string {
	return filepath.Join(root, infuseDirName, "project-rules.json")
}

func instructionsHash(text string) string {
	h := sha1.Sum([]byte(text))
	return hex.EncodeToString(h[:])
}

func loadCompiledRules(root string) *compiledRules {
	var c compiledRules
	if readJSON(compiledRulesFile(root), &c) != nil {
		return nil
	}
	return &c
}

const compilePrompt = `You receive the instructions of a software project (they may be written in any language). Extract ONLY what they state explicitly, in these three kinds:
- "reference": a path (folder or file) that the instructions say not to copy, not to write to, or to use only as a reference for reading.
- "read": a document that the instructions say to read, consult or follow.
- "verify": a command that the instructions declare for verifying, testing or measuring the work; "when" is "after-change" when it must run after changes, "before-commit" when it must run before a commit, and "on-request" in every other case.
For each rule, "quote" is a passage copied EXACTLY from the text, in the original language of the instructions (the same words, neither translated nor summarized), that contains the path or the command and the instruction itself. A rule without a verbatim quote will be discarded. Do not invent anything the text does not say.
Answer by calling ProjectRules with every rule found (an empty list when the text states none).`

// projectRulesSpec is the shape of the compiled instructions.
var projectRulesSpec = ToolSpec{Name: "ProjectRules", Description: "The rules stated in the project's instructions, each with its verbatim quote.",
	InputSchema: schema(`{"type":"object","properties":{"rules":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","enum":["reference","read","verify"]},"path":{"type":"string"},"command":{"type":"string"},"when":{"type":"string","enum":["after-change","before-commit","on-request"]},"quote":{"type":"string"}},"required":["kind","quote"]}}},"required":["rules"]}`)}

// compileInstructions asks the model for the rules and keeps those whose quote is in the text.
func (s *Session) compileInstructions(ctx context.Context, p Provider, text string) (*compiledRules, error) {
	var out struct {
		Rules []compiledRule `json:"rules"`
	}
	usage, err := askJSON(ctx, p, s.fastModel(), compilePrompt, text, projectRulesSpec, 8000, &out)
	s.mu.Lock()
	s.Cost += s.settings.CostOf(s.fastModel(), usage)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	c := &compiledRules{Hash: instructionsHash(text), Compiled: time.Now().UnixMilli(), Model: s.fastModel()}
	norm := normSpace(text)
	for _, r := range out.Rules {
		why := verifyRule(s.Root, norm, r)
		if !why.Empty() {
			c.Dropped = append(c.Dropped, M("instr.rule_dropped", "kind", r.Kind, "target", r.Path+r.Command, "why", why))
			continue
		}
		r.Disabled = false
		c.Rules = append(c.Rules, r)
	}
	return c, nil
}

// verifyRule is the mechanical check of a proposed rule (empty when it holds).
func verifyRule(root, normText string, r compiledRule) Msg {
	q := normSpace(r.Quote)
	if len(q) < 8 || !strings.Contains(normText, q) {
		return M("instr.rule_no_quote")
	}
	switch r.Kind {
	case "reference", "read":
		p := strings.Trim(strings.TrimPrefix(filepath.ToSlash(r.Path), "./"), "/")
		if p == "" || !strings.Contains(q, normSpace(p)) {
			return M("instr.rule_no_path")
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil && r.Kind == "read" {
			return M("instr.rule_no_doc")
		}
	case "verify":
		if r.Command == "" || !strings.Contains(q, normSpace(r.Command)) {
			return M("instr.rule_no_command")
		}
	default:
		return M("instr.rule_bad_kind")
	}
	return Msg{}
}

// ensureCompiledRules compiles the instructions when they changed since the last compilation.
func (s *Session) ensureCompiledRules(ctx context.Context, p Provider) {
	pr := loadProjectRules(s.Root)
	if pr.text == "" {
		return
	}
	h := instructionsHash(pr.text)
	old := loadCompiledRules(s.Root)
	if old != nil && old.Hash == h {
		return
	}
	s.emitMsg("gate", M("instr.compiling"))
	c, err := s.compileInstructions(ctx, p, pr.text)
	if err != nil { // the user sees it: without the compiled rules their locks are not in force
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "compiling the instructions: " + err.Error()})
		s.emitMsg("gate", M("instr.compile_failed", "error", firstLine(err.Error())))
		return
	}
	if old != nil { // the user's switches survive a recompilation
		off := map[string]bool{}
		for _, r := range old.Rules {
			if r.Disabled {
				off[r.Kind+"|"+r.Path+"|"+r.Command] = true
			}
		}
		for i := range c.Rules {
			c.Rules[i].Disabled = off[c.Rules[i].Kind+"|"+c.Rules[i].Path+"|"+c.Rules[i].Command]
		}
	}
	_ = writeJSON(compiledRulesFile(s.Root), c)
	rulesCache.Delete(s.Root)
	s.emit("projectRules", c)
}
