package api

// Integrated terminal and slash commands for the window.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type terminalResult struct {
	Output string   `json:"output"`
	Cwd    string   `json:"cwd"`
	Exit   int      `json:"exit"`
	Notes  []string `json:"notes"`
}

// runTerminal runs one command through the filter (heredoc transform, rules, journal) and returns what the
// agent would have seen, plus the new working directory.
func runTerminal(root, cwd, command string) terminalResult {
	if cwd == "" || !within(root, cwd) {
		cwd = root
	}
	if strings.TrimSpace(command) == "" {
		return terminalResult{Cwd: cwd, Notes: []string{}}
	}
	rules := LoadRules(root)
	tr := TransformCommand(command, cwd)
	if rules.Disabled["heredoc"] {
		tr = TransformResult{Command: command}
	}
	final, ruleLines := ApplyCommandRules(root, rules, tr.Command)
	Journal(root, JournalEntry{Kind: "command", Command: command, Detail: "terminal"})
	for _, a := range tr.Actions {
		if a.Kind == ActLesson && a.Vice != "" {
			Lesson(root, a.Vice, a.Detail)
		} else {
			Journal(root, JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}
	started := time.Now().UnixMilli()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, FindRealBash(), "-c", final+"\nprintf '\\n__INFUSE_CWD__%s' \"$PWD\"")
	noConsoleWindow(cmd)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	buf := newCapWriter(1 << 20)
	cmd.Stdout, cmd.Stderr = buf, buf
	err := cmd.Run()
	out := buf.String()
	res := terminalResult{Cwd: cwd, Exit: exitCode(err), Notes: []string{}}
	if i := strings.LastIndex(out, "__INFUSE_CWD__"); i >= 0 {
		if nc := FromMsysPath(strings.TrimSpace(out[i+len("__INFUSE_CWD__"):])); nc != "" && within(root, nc) {
			res.Cwd = nc
		}
		out = strings.TrimRight(out[:i], "\n")
	}
	res.Output = out
	if trailer := buildTrailer(root, cwd, command, tr.Actions, started, rules, ruleLines); trailer != "" {
		res.Notes = strings.Split(strings.TrimSpace(trailer), "\n")
	}
	return res
}

type slashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt,omitempty"` // custom: the file body, with $ARGUMENTS
	Builtin     bool   `json:"builtin"`
}

// listCommands: built-ins the window handles, plus custom ones from <project>/.infuse/commands/*.md and
// ~/.infuse/commands/*.md (also read from .claude/commands for compatibility).
func listCommands(root string) []slashCommand {
	out := []slashCommand{
		{Name: "clear", Description: "Nova sessão neste projeto", Builtin: true},
		{Name: "compact", Description: "Compactar o contexto da sessão agora", Builtin: true},
		{Name: "model", Description: "Trocar o modelo desta sessão", Builtin: true},
		{Name: "mode", Description: "Alternar o modo de permissão", Builtin: true},
		{Name: "rewind", Description: "Voltar a um turno anterior", Builtin: true},
		{Name: "diff", Description: "Mostrar as alterações do projeto", Builtin: true},
		{Name: "cost", Description: "Tokens e custo desta sessão", Builtin: true},
		{Name: "vim", Description: "Ligar/desligar o modo vim na caixa de prompt", Builtin: true},
		{Name: "copy", Description: "Copiar a última resposta (/copy 2 = a penúltima)", Builtin: true},
		{Name: "export", Description: "Exportar a conversa como Markdown", Builtin: true},
		{Name: "btw", Description: "Pergunta paralela, fora do histórico", Builtin: true},
		{Name: "mission", Description: "Trabalhar sozinho até os critérios do Pronto? passarem, com orçamento", Builtin: true},
		{Name: "plan", Description: "Entrar no modo Plano", Builtin: true},
		{Name: "effort", Description: "Escolher o esforço de raciocínio", Builtin: true},
		{Name: "config", Description: "Abrir as Configurações", Builtin: true},
		{Name: "resume", Description: "Abrir outra sessão", Builtin: true},
		{Name: "rename", Description: "Renomear esta sessão", Builtin: true},
		{Name: "review", Description: "Revisão independente: um revisor com contexto limpo confere as alterações", Builtin: true},
		{Name: "init", Description: "Gerar INFUSE.md com as instruções do projeto", Builtin: true},
		{Name: "memory", Description: "Editar as instruções do projeto", Builtin: true},
		{Name: "terminal", Description: "Abrir o terminal", Builtin: true},
		{Name: "help", Description: "Atalhos de teclado", Builtin: true},
	}
	seen := map[string]bool{}
	for _, dir := range []string{
		filepath.Join(root, infuseDirName, "commands"), filepath.Join(root, ".claude", "commands"),
		filepath.Join(InfuseHome(), "commands"), filepath.Join(homeDir(), ".claude", "commands"),
	} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			if seen[name] {
				continue
			}
			seen[name] = true
			body := readIfExists(filepath.Join(dir, e.Name()))
			desc := ""
			if strings.HasPrefix(body, "---") { // frontmatter: description
				if end := strings.Index(body[3:], "---"); end > 0 {
					for _, l := range strings.Split(body[3:3+end], "\n") {
						if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "description" {
							desc = strings.TrimSpace(v)
						}
					}
					body = strings.TrimSpace(body[3+end+3:])
				}
			}
			if desc == "" {
				desc = truncate(strings.SplitN(strings.TrimSpace(body), "\n", 2)[0], 80)
			}
			out = append(out, slashCommand{Name: name, Description: desc, Prompt: body})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Builtin && !out[j].Builtin })
	return out
}
