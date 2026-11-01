package knowledge

// The meaning of the project's instructions — in any language, any wording — is compiled once into
// structured rules by a model, and every rule must carry the exact quote it comes from. Code then checks
// each quote against the instruction text word for word (spaces normalized): a rule whose quote is not
// there was invented and is dropped. The rules are stored in .infuse/project-rules.json.

import (
	"crypto/sha1"
	"encoding/hex"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
)

type CompiledRule struct {
	Kind     string `json:"kind"`               // reference | read | verify
	Path     string `json:"path,omitempty"`     // reference, read
	Command  string `json:"command,omitempty"`  // verify
	When     string `json:"when,omitempty"`     // verify: after-change | before-commit | on-request
	Quote    string `json:"quote"`              // the exact words of the instructions
	Disabled bool   `json:"disabled,omitempty"` // switched off by the user
}

type CompiledRules struct {
	Hash     string         `json:"hash"` // of the instruction text compiled
	Compiled int64          `json:"compiled"`
	Model    string         `json:"model"`
	Rules    []CompiledRule `json:"rules"`
	Dropped  []core.Msg     `json:"rejected,omitempty"` // proposals rejected by the quote check, with why (as messages)
	Stale    bool           `json:"stale,omitempty"`    // a later compilation failed: these rules are from an older instruction text
}

func CompiledRulesFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "project-rules.json")
}

func InstructionsHash(text string) string {
	h := sha1.Sum([]byte(text))
	return hex.EncodeToString(h[:])
}

func LoadCompiledRules(root string) *CompiledRules {
	var c CompiledRules
	if core.ReadJSON(CompiledRulesFile(root), &c) != nil {
		return nil
	}
	return &c
}

const CompilePrompt = `You receive the instructions of a software project (they may be written in any language). Extract ONLY what they state explicitly, in these three kinds:
- "reference": a path (folder or file) that the instructions say not to copy, not to write to, or to use only as a reference for reading.
- "read": a document that the instructions say to read, consult or follow.
- "verify": a command that the instructions declare for verifying, testing or measuring the work; "when" is "after-change" when it must run after changes, "before-commit" when it must run before a commit, and "on-request" in every other case.
For each rule, "quote" is a passage copied EXACTLY from the text, in the original language of the instructions (the same words, neither translated nor summarized), that contains the path or the command and the instruction itself. A rule without a verbatim quote will be discarded. Do not invent anything the text does not say.
Answer by calling ProjectRules with every rule found (an empty list when the text states none).`

// ProjectRulesSpec is the shape of the compiled instructions.
var ProjectRulesSpec = core.ToolSpec{Name: "ProjectRules", Description: "The rules stated in the project's instructions, each with its verbatim quote.",
	InputSchema: core.RawJSON(`{"type":"object","properties":{"rules":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","enum":["reference","read","verify"]},"path":{"type":"string"},"command":{"type":"string"},"when":{"type":"string","enum":["after-change","before-commit","on-request"]},"quote":{"type":"string"}},"required":["kind","quote"]}}},"required":["rules"]}`)}

// VerifyRule is the mechanical check of a proposed rule (empty when it holds).
func VerifyRule(root, normText string, r CompiledRule) core.Msg {
	q := core.NormSpace(r.Quote)
	if len(q) < 8 || !strings.Contains(normText, q) {
		return core.M("instr.rule_no_quote")
	}
	switch r.Kind {
	case "reference", "read":
		p := strings.Trim(strings.TrimPrefix(filepath.ToSlash(r.Path), "./"), "/")
		if p == "" || !strings.Contains(q, core.NormSpace(p)) {
			return core.M("instr.rule_no_path")
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil && r.Kind == "read" {
			return core.M("instr.rule_no_doc")
		}
	case "verify":
		if r.Command == "" || !strings.Contains(q, core.NormSpace(r.Command)) {
			return core.M("instr.rule_no_command")
		}
	default:
		return core.M("instr.rule_bad_kind")
	}
	return core.Msg{}
}
