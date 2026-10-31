package main

// The agent's report and the turn contract. Report: at the end of a turn the agent states what it did as
// claims, each with the evidence it relies on (a command it ran, a file it changed); infuse checks every
// claim against what actually happened in the turn — the tool calls and their results, the checkpoints —
// and shows the outcome beside the agent's words. Contract: the agent turns the order into verifiable
// criteria and the user confirms them; confirmed criteria become session memory of the highest priority.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReportClaim struct {
	Text     string `json:"text"`
	Kind     string `json:"kind"` // command | file | other
	Ref      string `json:"ref,omitempty"`
	Status   string `json:"status"` // confirmado | parcial | nao-confirmado | sem-evidencia
	Evidence string `json:"evidence"`
}

type TurnReport struct {
	Turn    int           `json:"turn"`
	Summary string        `json:"summary"`
	Claims  []ReportClaim `json:"claims"`
	Next    string        `json:"next,omitempty"`
}

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// verifyClaim compares one claim with the turn's tool calls and checkpoints.
func (s *Session) verifyClaim(c *ReportClaim) {
	s.mu.Lock()
	start := 0
	if s.Turn-1 < len(s.TurnStarts) && s.Turn > 0 {
		start = s.TurnStarts[s.Turn-1]
	}
	msgs := append([]Message{}, s.Messages[min(start, len(s.Messages)):]...)
	msgs = append(msgs, Message{Role: "user", Content: append([]Block{}, s.inflight...)})
	written := map[string]bool{}
	for _, f := range s.Checkpoints[s.Turn] {
		written[f.Path] = true
	}
	root := s.workRoot()
	s.mu.Unlock()

	switch c.Kind {
	case "command":
		ref := squash(c.Ref)
		if ref == "" {
			c.Status, c.Evidence = "sem-evidencia", "nenhum comando indicado"
			return
		}
		results := map[string]Block{}
		for _, m := range msgs {
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					results[b.ToolUseID] = b
				}
			}
		}
		found, failed := false, ""
		for _, m := range msgs {
			for _, b := range m.Content {
				if b.Type != "tool_use" || b.Name != "Bash" {
					continue
				}
				var in struct{ Command string }
				_ = json.Unmarshal(b.Input, &in)
				if !strings.Contains(squash(in.Command), ref) {
					continue
				}
				r, ok := results[b.ID]
				if ok && !r.IsError {
					found = true
				} else if ok {
					failed = firstLine(r.Content)
				}
			}
		}
		switch {
		case found:
			c.Status, c.Evidence = "confirmado", "executado neste turno sem erro"
		case failed != "":
			c.Status, c.Evidence = "nao-confirmado", "executado neste turno, mas falhou: "+failed
		default:
			c.Status, c.Evidence = "nao-confirmado", "nenhuma execução deste comando neste turno"
		}
	case "file":
		rel := filepath.ToSlash(strings.TrimPrefix(filepath.Clean(c.Ref), filepath.Clean(root)+string(filepath.Separator)))
		rel = strings.TrimPrefix(rel, "./")
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		switch {
		case written[rel] && err == nil:
			c.Status, c.Evidence = "confirmado", "alterado neste turno"
		case written[rel]:
			c.Status, c.Evidence = "confirmado", "removido neste turno"
		case err == nil:
			c.Status, c.Evidence = "parcial", "o arquivo existe, mas não foi alterado neste turno"
		default:
			c.Status, c.Evidence = "nao-confirmado", "o arquivo não existe"
		}
	default:
		c.Kind, c.Status, c.Evidence = "other", "sem-evidencia", "afirmação sem evidência verificável"
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// ---------------------------------------------------------------- tools

type reportTool struct{}

func (reportTool) Category() string { return "ui" }
func (reportTool) Spec() ToolSpec {
	return ToolSpec{Name: "Report", Description: "Ao terminar um trabalho, relate o que foi feito como afirmações, cada uma com a evidência em que se apoia: kind=command com ref = o comando que você executou (build, testes), kind=file com ref = o arquivo alterado, ou kind=other. O infuse confere cada afirmação com o que de fato aconteceu no turno e mostra o resultado ao usuário. Inclua em next o próximo passo sugerido, se houver.",
		InputSchema: schema(`{"type":"object","properties":{"summary":{"type":"string"},"claims":{"type":"array","items":{"type":"object","properties":{"text":{"type":"string"},"kind":{"type":"string","enum":["command","file","other"]},"ref":{"type":"string"}},"required":["text","kind"]}},"next":{"type":"string"}},"required":["summary","claims"]}`)}
}

func (reportTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var r TurnReport
	if err := json.Unmarshal(input, &r); err != nil {
		return err.Error(), true
	}
	s := tc.S
	r.Turn = s.Turn
	var b strings.Builder
	bad := 0
	for i := range r.Claims {
		s.verifyClaim(&r.Claims[i])
		c := r.Claims[i]
		fmt.Fprintf(&b, "- [%s] %s — %s\n", c.Status, c.Text, c.Evidence)
		if c.Status == "nao-confirmado" {
			bad++
		}
	}
	s.emit("report", r)
	out := "Relatório conferido pelo infuse:\n" + b.String()
	if bad > 0 {
		out += fmt.Sprintf("%d afirmação(ões) não confirmada(s) pela execução: corrija o trabalho ou a afirmação; o usuário vê esta conferência.", bad)
	}
	return out, false
}

type contractTool struct{}

func (contractTool) Category() string { return "ui" }
func (contractTool) Spec() ToolSpec {
	return ToolSpec{Name: "Contract", Description: "Antes de um trabalho com mais de um passo, transforme o pedido em critérios verificáveis (o que precisa ser verdade ao final, cada um conferível por um comando, um arquivo ou o uso do programa) e peça a confirmação do usuário. Critérios confirmados entram na memória da sessão com prioridade máxima.",
		InputSchema: schema(`{"type":"object","properties":{"goal":{"type":"string"},"criteria":{"type":"array","items":{"type":"string"}}},"required":["goal","criteria"]}`)}
}

func (contractTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Goal     string   `json:"goal"`
		Criteria []string `json:"criteria"`
	}
	_ = json.Unmarshal(input, &in)
	if len(in.Criteria) == 0 {
		return "informe ao menos um critério", true
	}
	var q strings.Builder
	fmt.Fprintf(&q, "Objetivo: %s\n\nVou considerar pronto quando:\n", in.Goal)
	for i, c := range in.Criteria {
		fmt.Fprintf(&q, "%d. %s\n", i+1, c)
	}
	q.WriteString("\nConfirma? (ou escreva o que ajustar)")
	ans, ok := tc.S.askUser(ctx, q.String(), []string{"Confirmar"})
	if !ok {
		return "o usuário não respondeu", true
	}
	if strings.TrimSpace(ans) != "Confirmar" {
		return "O usuário pediu ajustes nos critérios: " + ans + "\nReformule e chame Contract de novo.", false
	}
	for _, c := range in.Criteria {
		it := MemoryItem{Type: "criterio", Text: c, Priority: 3, Origin: "usuario", Session: tc.S.ID}
		if normalizeMemory(&it) == nil {
			_ = tc.S.editMemory("session", func(items []MemoryItem) []MemoryItem { return append(items, it) })
		}
	}
	tc.S.emit("memory", map[string]any{"action": "contract"})
	return "Critérios confirmados pelo usuário e guardados na memória da sessão. Trabalhe até cumpri-los e relate com Report, com evidência de cada um.", false
}

func init() {
	registerTool(contractTool{})
	registerTool(reportTool{})
}
