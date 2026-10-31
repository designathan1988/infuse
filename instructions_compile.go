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
	"encoding/json"
	"fmt"
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
	Dropped  []string       `json:"dropped,omitempty"` // proposals rejected by the quote check, with why
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

func normSpace(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

const compilePrompt = `Você recebe as instruções de um projeto de software (podem estar em qualquer idioma). Extraia SOMENTE o que elas afirmam explicitamente, nestes três tipos:
- "reference": um caminho (pasta ou arquivo) que as instruções dizem para não copiar, não escrever ou usar só como referência de leitura.
- "read": um documento que as instruções mandam ler, consultar ou seguir.
- "verify": um comando que as instruções declaram para verificar, testar ou medir o trabalho; "when" é "after-change" quando deve rodar depois de alterações, "before-commit" quando antes de commit, "on-request" nos outros casos.
Para cada regra, "quote" é um trecho copiado EXATAMENTE do texto (mesmas palavras, sem traduzir nem resumir) que contém o caminho ou o comando e a ordem. Regra sem trecho literal será descartada. Não invente nada que o texto não diga.
Responda só com JSON: {"rules":[{"kind":"reference|read|verify","path":"…","command":"…","when":"…","quote":"…"}]}`

// compileInstructions asks the model for the rules and keeps those whose quote is in the text.
func (s *Session) compileInstructions(ctx context.Context, p Provider, text string) (*compiledRules, error) {
	res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), System: compilePrompt, Messages: []Message{{Role: "user", Content: []Block{{Type: "text", Text: text}}}}, MaxTokens: 4000}, func(StreamEvent) {})
	if err != nil {
		return nil, err
	}
	raw := msgText(Message{Content: res.Content})
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var out struct {
		Rules []compiledRule `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("a compilação das instruções não devolveu JSON: %w", err)
	}
	c := &compiledRules{Hash: instructionsHash(text), Compiled: time.Now().UnixMilli(), Model: s.fastModel()}
	norm := normSpace(text)
	for _, r := range out.Rules {
		why := verifyRule(s.Root, norm, r)
		if why != "" {
			c.Dropped = append(c.Dropped, fmt.Sprintf("%s %s%s: %s", r.Kind, r.Path, r.Command, why))
			continue
		}
		r.Disabled = false
		c.Rules = append(c.Rules, r)
	}
	s.mu.Lock()
	s.Cost += s.settings.costOf(s.fastModel(), res.Usage)
	s.mu.Unlock()
	return c, nil
}

// verifyRule is the mechanical check of a proposed rule ("" when it holds).
func verifyRule(root, normText string, r compiledRule) string {
	q := normSpace(r.Quote)
	if len(q) < 8 || !strings.Contains(normText, q) {
		return "o trecho citado não está nas instruções"
	}
	switch r.Kind {
	case "reference", "read":
		p := strings.Trim(strings.TrimPrefix(filepath.ToSlash(r.Path), "./"), "/")
		if p == "" || !strings.Contains(q, normSpace(p)) {
			return "o caminho não aparece no trecho citado"
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil && r.Kind == "read" {
			return "o documento não existe no projeto"
		}
	case "verify":
		if r.Command == "" || !strings.Contains(q, normSpace(r.Command)) {
			return "o comando não aparece no trecho citado"
		}
	default:
		return "tipo desconhecido"
	}
	return ""
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
	s.emit("gate", map[string]any{"message": "[infuse] Lendo as instruções do projeto para extrair as regras (cada uma conferida contra o texto)…"})
	c, err := s.compileInstructions(ctx, p, pr.text)
	if err != nil {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "compilação das instruções: " + err.Error()})
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
