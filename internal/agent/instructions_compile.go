package agent

// Compiling the instructions into quote-checked rules: the model call and the session orchestration.
// The rule types, verification and storage live in internal/knowledge.

import (
	"context"
	"infuse/internal/knowledge"
	"time"
)

type CompiledRule = knowledge.CompiledRule
type CompiledRules = knowledge.CompiledRules

func compiledRulesFile(root string) string         { return knowledge.CompiledRulesFile(root) }
func instructionsHash(text string) string          { return knowledge.InstructionsHash(text) }
func loadCompiledRules(root string) *CompiledRules { return knowledge.LoadCompiledRules(root) }

// compileInstructions asks the model for the rules and keeps those whose quote is in the text.
func (s *Session) compileInstructions(ctx context.Context, p Provider, text string) (*CompiledRules, error) {
	var out struct {
		Rules []CompiledRule `json:"rules"`
	}
	usage, err := askJSON(ctx, p, s.fastModel(), knowledge.CompilePrompt, text, knowledge.ProjectRulesSpec, 8000, &out)
	s.Mu.Lock()
	s.Cost += s.settings.CostOf(s.fastModel(), usage)
	s.Mu.Unlock()
	if err != nil {
		return nil, err
	}
	c := &CompiledRules{Hash: knowledge.InstructionsHash(text), Compiled: time.Now().UnixMilli(), Model: s.fastModel()}
	norm := normSpace(text)
	for _, r := range out.Rules {
		why := knowledge.VerifyRule(s.Root, norm, r)
		if !why.Empty() {
			c.Dropped = append(c.Dropped, M("instr.rule_dropped", "kind", r.Kind, "target", r.Path+r.Command, "why", why))
			continue
		}
		r.Disabled = false
		c.Rules = append(c.Rules, r)
	}
	return c, nil
}

// ensureCompiledRules compiles the instructions when they changed since the last compilation.
func (s *Session) ensureCompiledRules(ctx context.Context, p Provider) {
	pr := knowledge.LoadProjectRules(s.Root)
	if pr.Text == "" {
		return
	}
	h := knowledge.InstructionsHash(pr.Text)
	old := knowledge.LoadCompiledRules(s.Root)
	if old != nil && old.Hash == h {
		return
	}
	s.emitMsg("gate", M("instr.compiling"))
	c, err := s.compileInstructions(ctx, p, pr.Text)
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
	_ = writeJSON(knowledge.CompiledRulesFile(s.Root), c)
	knowledge.InvalidateRules(s.Root)
	s.emit("projectRules", c)
}
