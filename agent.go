package main

// Subagents: the Agent tool runs a task in a fresh conversation (no history of the session) and returns
// only its conclusion. Every tool call of a subagent goes through the session's permissions, the filter
// and the guard, like the agent's own. Kinds:
//   explore — read-only search and reading across the project;
//   review  — independent reviewer: sees the diff, the plan and the criteria, never the agent's reasoning;
//   general — any tool except Agent itself.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var readOnlyTools = map[string]bool{"Read": true, "Glob": true, "Grep": true, "LS": true, "WebFetch": true, "WebSearch": true}

func subagentAllows(kind, tool string) bool {
	switch kind {
	case "explore", "review":
		return readOnlyTools[tool]
	default:
		return tool != "Agent"
	}
}

func subagentSystem(s *Session, kind string) string {
	var b strings.Builder
	switch kind {
	case "review":
		b.WriteString("Você é um revisor independente de código. Você não escreveu este trabalho e não viu o raciocínio de quem escreveu. Confira as alterações contra o pedido, o plano e os critérios: procure erros de lógica, casos de borda sem tratamento, validações ou verificações removidas, testes alterados para passar, código que não é chamado, responsabilidades espalhadas. Leia os arquivos que precisar. Responda com uma lista de achados, do mais grave ao menos grave, cada um com arquivo:linha, o problema e a consequência concreta; e por fim um veredito: aprovar ou corrigir. Se não houver achados, diga isso.\n")
	case "explore":
		b.WriteString("Você é um subagente de pesquisa num projeto de software. Leia e procure o que for preciso para responder à tarefa e devolva uma conclusão objetiva, com caminhos de arquivos e linhas. Você só tem ferramentas de leitura.\n")
	default:
		b.WriteString("Você é um subagente executando uma tarefa delimitada num projeto de software. Faça só o que a tarefa pede e devolva um relato curto do que fez, com a evidência (comandos e resultados).\n")
	}
	fmt.Fprintf(&b, "Diretório do projeto: %s. Responda em português.\n", s.workRoot())
	return b.String()
}

// RunSubagent runs task in a fresh conversation and returns the final text.
func (s *Session) RunSubagent(ctx context.Context, kind, title, task string) (string, error) {
	provider, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return "", err
	}
	if kind == "" {
		kind = "general"
	}
	rules := LoadRules(s.Root)
	mcpFor(s.Root).ensure(ctx)
	allowed := func(name string) bool {
		if subagentAllows(kind, name) {
			return true
		}
		t, ok := s.lookupTool(name) // read-only connector tools serve the read-only kinds too
		return ok && strings.HasPrefix(name, "mcp__") && (kind == "general" || t.Category() == "read")
	}
	var specs []ToolSpec
	for _, sp := range s.toolSpecs() {
		if allowed(sp.Name) {
			specs = append(specs, sp)
		}
	}
	id := "a" + randomHex(6)
	s.emit("subagent", map[string]any{"id": id, "kind": kind, "title": title, "state": "start"})
	msgs := []Message{{Role: "user", Content: []Block{{Type: "text", Text: task}}, TS: time.Now().UnixMilli()}}
	var breaker loopBreaker
	for iter := 0; iter < 60; iter++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		res, err := provider.Stream(ctx, ModelRequest{Model: s.Model, System: subagentSystem(s, kind), Messages: msgs, Tools: specs, MaxTokens: s.settings.MaxTokens}, func(StreamEvent) {})
		if err != nil {
			return "", err
		}
		cost := s.settings.costOf(s.Model, res.Usage)
		s.mu.Lock()
		s.Cost += cost
		s.Usage.Input += res.Usage.Input
		s.Usage.Output += res.Usage.Output
		s.mu.Unlock()
		recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.Model, Input: res.Usage.Input, Output: res.Usage.Output, Cost: cost})
		msgs = append(msgs, Message{Role: "assistant", Content: res.Content})
		if res.StopReason != "tool_use" {
			out := strings.TrimSpace(msgText(Message{Content: res.Content}))
			s.emit("subagent", map[string]any{"id": id, "state": "done", "steps": iter})
			return out, nil
		}
		var results []Block
		for _, b := range res.Content {
			if b.Type != "tool_use" {
				continue
			}
			var out string
			var isErr bool
			if !allowed(b.Name) {
				out, isErr = "ferramenta indisponível para este subagente: "+b.Name, true
			} else {
				out, isErr = s.runTool(ctx, rules, b)
				out = redactSecrets(out)
			}
			results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: out, IsError: isErr})
			s.emit("subagent", map[string]any{"id": id, "state": "tool", "tool": b.Name, "summary": firstLine(string(b.Input))})
			if err := breaker.record(b.Name, b.Input, isErr); err != nil {
				return "", fmt.Errorf("subagente interrompido: %w", err)
			}
		}
		msgs = append(msgs, Message{Role: "user", Content: results})
	}
	return "", errors.New("subagente atingiu o limite de passos")
}

// reviewMaterial is what the reviewer sees: the request, the plan, the criteria and the diff.
func (s *Session) reviewMaterial(focus string) string {
	var b strings.Builder
	if req := s.turnRequest(); req != "" {
		b.WriteString("## Pedido do usuário\n" + req + "\n\n")
	}
	s.mu.Lock()
	tasks := append([]TaskItem{}, s.Tasks...)
	s.mu.Unlock()
	if len(tasks) > 0 {
		b.WriteString("## Plano\n")
		for _, t := range tasks {
			fmt.Fprintf(&b, "- [%s] %s", t.Status, t.Text)
			if t.Proof != "" {
				fmt.Fprintf(&b, " (comprovação: %s)", t.Proof)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if o := readOracle(s.Root); o != "" {
		b.WriteString("## Critérios de pronto\n" + o + "\n\n")
	}
	if m := memoryPrompt(s); m != "" {
		b.WriteString("## Memória (correções e critérios combinados)\n" + m + "\n")
	}
	if focus != "" {
		b.WriteString("## Foco pedido\n" + focus + "\n\n")
	}
	root := s.workRoot()
	const budget = 60000
	diff := ""
	if exists(filepath.Join(root, ".git")) {
		diff, _ = gitRun(root, "diff", "HEAD")
		if untracked, _ := gitRun(root, "ls-files", "--others", "--exclude-standard"); strings.TrimSpace(untracked) != "" {
			for _, f := range strings.Split(strings.TrimSpace(untracked), "\n") {
				if len(diff) > budget {
					break
				}
				if c, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil && len(c) < 40000 {
					diff += "\n--- /dev/null\n+++ b/" + f + " (arquivo novo)\n" + string(c)
				}
			}
		}
	} else {
		s.mu.Lock()
		seen := map[string]bool{}
		var files []string
		for _, cps := range s.Checkpoints {
			for _, c := range cps {
				if !seen[c.Path] {
					seen[c.Path] = true
					files = append(files, c.Path)
				}
			}
		}
		s.mu.Unlock()
		for _, f := range files {
			if c, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil && len(diff) < budget {
				diff += "\n=== " + f + " (alterado nesta sessão)\n" + string(c)
			}
		}
	}
	if len(diff) > budget {
		diff = diff[:budget] + "\n[… diff cortado]"
	}
	if strings.TrimSpace(diff) == "" {
		diff = "(nenhuma alteração encontrada)"
	}
	b.WriteString("## Alterações\n```diff\n" + diff + "\n```\n")
	return b.String()
}

// ---------------------------------------------------------------- the agent's tool

type agentTool struct{}

func (agentTool) Category() string { return "ui" }
func (agentTool) Spec() ToolSpec {
	return ToolSpec{Name: "Agent", Description: "Delega uma tarefa a um subagente com contexto limpo, que devolve só a conclusão. kind=explore: pesquisa e leitura no projeto (só leitura). kind=review: revisor independente das alterações atuais (vê o diff, o plano e os critérios, não o seu raciocínio) — use antes de declarar um trabalho pronto. kind=general: tarefa delimitada com qualquer ferramenta. As ações do subagente passam pelas mesmas permissões e pelo filtro.",
		InputSchema: schema(`{"type":"object","properties":{"kind":{"type":"string","enum":["explore","review","general"]},"description":{"type":"string","description":"título curto"},"prompt":{"type":"string","description":"a tarefa, completa: o subagente não vê esta conversa"}},"required":["kind","prompt"]}`)}
}

func (agentTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Kind        string `json:"kind"`
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	_ = json.Unmarshal(input, &in)
	task := in.Prompt
	if in.Kind == "review" {
		task = tc.S.reviewMaterial(in.Prompt) + "\nRevise as alterações acima."
	}
	out, err := tc.S.RunSubagent(ctx, in.Kind, in.Description, task)
	if err != nil {
		return "subagente falhou: " + err.Error(), true
	}
	return out, false
}

func init() { registerTool(agentTool{}) }

const reviewMarker = "[Revisão independente]"
const reviewAck = "Revisão recebida."

// IndependentReview runs the reviewer on the current changes (asked by the user) and records its
// findings in the conversation, where the agent sees them on the next turn.
func (s *Session) IndependentReview(ctx context.Context, focus string) (string, error) {
	s.mu.Lock()
	busy := s.State != "idle" && s.State != "error"
	s.mu.Unlock()
	if busy {
		return "", errors.New("espere o agente terminar para revisar")
	}
	out, err := s.RunSubagent(ctx, "review", "Revisão independente", s.reviewMaterial(focus)+"\nRevise as alterações acima.")
	if err != nil {
		return "", err
	}
	now := time.Now().UnixMilli()
	s.mu.Lock()
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: reviewMarker + "\n\n" + out}}, TS: now}, Message{Role: "assistant", Content: []Block{{Type: "text", Text: reviewAck}}, TS: now})
	s.mu.Unlock()
	s.save()
	s.emit("session", s.snapshot())
	return out, nil
}
