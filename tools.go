package main

// Agent tools. Every tool that touches the project goes through the filter by construction:
// protected paths are refused here, writes are journaled and checkpointed, outputs come back infused.
// Adding a tool = registering an implementation.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ToolContext struct {
	S     *Session
	Root  string
	Cwd   string
	Rules *RuleSet
}

type Tool interface {
	Spec() ToolSpec
	Category() string // read | write | exec | net | ui
	Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool)
}

var (
	toolRegistry = map[string]Tool{}
	toolOrder    []string
)

func registerTool(t Tool) {
	toolRegistry[t.Spec().Name] = t
	toolOrder = append(toolOrder, t.Spec().Name)
}

func toolSpecs() []ToolSpec {
	out := make([]ToolSpec, 0, len(toolOrder))
	for _, n := range toolOrder {
		out = append(out, toolRegistry[n].Spec())
	}
	return out
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

const maxToolOutput = 40000

func clip(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	return s[:maxToolOutput/2] + fmt.Sprintf("\n\n[... %d caracteres omitidos ...]\n\n", len(s)-maxToolOutput) + s[len(s)-maxToolOutput/2:]
}

// resolvePath turns a tool path into an absolute path inside the project; refuses escapes and protected paths.
func (tc *ToolContext) resolvePath(p string, forWrite bool) (string, error) {
	p = FromMsysPath(p)
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(tc.Cwd, p)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(tc.Root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		if forWrite {
			return "", fmt.Errorf("caminho fora do projeto: %s", p)
		}
	}
	if !forWrite && err == nil && !strings.HasPrefix(rel, "..") && hiddenInternal(rel) {
		return "", fmt.Errorf("%s é um arquivo interno do infuse (sessões, jornal, histórico), não do projeto. O agente lê do .infuse/ só a documentação fixada (.infuse/docs), as convenções, o mapa (.infuse/map/MAP.md) e as próprias capturas", toPosix(rel))
	}
	if forWrite && tc.S.isProtected(abs) {
		Journal(tc.Root, JournalEntry{Kind: "blocked", Path: toPosix(rel), Detail: "tentativa de escrita em caminho protegido"})
		Lesson(tc.Root, "protected-path-write", "tentativa de escrever em "+toPosix(rel))
		return "", fmt.Errorf("caminho protegido, somente leitura para o agente: %s. O oráculo e os caminhos protegidos são definidos pelo usuário e não podem ser alterados pelo agente", toPosix(rel))
	}
	return abs, nil
}

func (tc *ToolContext) afterWrite(abs string) []string {
	rel := relTo(tc.Root, abs)
	st, _ := os.Stat(abs)
	size := 0
	if st != nil {
		size = int(st.Size())
	}
	Journal(tc.Root, JournalEntry{Kind: "write", Path: rel, Bytes: size, Detail: "agente"})
	tc.S.scheduleMapRefresh()
	var lines []string
	if m := LoadMap(tc.Root); m != nil {
		if d := DescribeDependents(m, rel); d != "" {
			lines = append(lines, d)
		}
	}
	lines = append(lines, ApplyWriteRules(tc.Root, tc.Rules, []string{abs})...)
	if tc.S.settings.Diagnostics == nil || *tc.S.settings.Diagnostics {
		if d := fileDiagnostics(tc.Root, abs); d != "" {
			lines = append(lines, "[diagnóstico] problemas encontrados pelo verificador do projeto neste arquivo:\n"+d)
		}
	}
	return lines
}

// ---------------------------------------------------------------- Bash

type bashTool struct{}

func (bashTool) Category() string { return "exec" }
func (bashTool) Spec() ToolSpec {
	return ToolSpec{Name: "Bash", Description: "Executa um comando no shell do projeto (bash). Use para rodar builds, testes, git e utilitários. A saída volta com informações do projeto quando relevante. run_in_background=true para o que continua rodando (servidor, jogo, watcher): volta na hora com o id; leia a saída com BashOutput e encerre com KillShell. Você é avisado quando o processo termina.",
		InputSchema: schema(`{"type":"object","properties":{"command":{"type":"string","description":"Comando a executar"},"timeout":{"type":"integer","description":"Timeout em segundos (padrão 120, máximo 600)"},"description":{"type":"string","description":"Descrição curta do que o comando faz"},"run_in_background":{"type":"boolean","description":"roda sem esperar terminar"}},"required":["command"]}`)}
}

func (bashTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Command     string `json:"command"`
		Timeout     int    `json:"timeout"`
		Description string `json:"description"`
		Background  bool   `json:"run_in_background"`
	}
	_ = json.Unmarshal(input, &in)
	if strings.TrimSpace(in.Command) == "" {
		return "comando vazio", true
	}
	if in.Timeout <= 0 {
		in.Timeout = 120
	}
	if in.Timeout > 600 {
		in.Timeout = 600
	}
	started := time.Now().UnixMilli()
	// The guard runs first: protected targets are refused or neutralized before the transform can write.
	guarded, guardNotes, gerr := newGuard(tc.Root, tc.S.settings.ProtectedPaths).GuardCommand(in.Command, tc.Cwd)
	if gerr != nil {
		return gerr.Error(), true
	}
	in.Command = guarded
	// Heredoc file writes happen inside TransformCommand: checkpoint their targets first.
	for _, h := range FindHeredocs(in.Command) {
		_, _, line := CommandLineOf(in.Command, h)
		if intent := DetectFileWrite(line); intent != nil {
			tc.S.checkpointBeforeWrite(resolveIn(tc.Cwd, FromMsysPath(intent.Path)))
		}
	}
	tr := TransformCommand(in.Command, tc.Cwd)
	if tc.Rules.Disabled["heredoc"] {
		tr = TransformResult{Command: in.Command}
	}
	command, ruleLines := ApplyCommandRules(tc.Root, tc.Rules, tr.Command)
	Journal(tc.Root, JournalEntry{Kind: "command", Command: in.Command, Detail: "agente"})
	for _, a := range tr.Actions {
		if a.Kind == ActLesson && a.Vice != "" {
			Lesson(tc.Root, a.Vice, a.Detail)
		} else {
			Journal(tc.Root, JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}
	if in.Background {
		p, err := tc.S.startBackground(command, in.Description, tc.Cwd)
		if err != nil {
			return "não iniciou: " + err.Error(), true
		}
		// the first moments of output: a command that fails at once says so now
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			if tc.S.proc(p.ID).State != "running" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		first, off := p.out.since(0)
		tc.S.mu.Lock()
		p.read = off
		state := p.State
		tc.S.mu.Unlock()
		msg := fmt.Sprintf("processo %s em segundo plano (pid %d), %s. Leia a saída nova com BashOutput(id=%q); encerre com KillShell. O usuário acompanha no painel Processos.", p.ID, p.PID, map[bool]string{true: "rodando", false: "já terminou"}[state == "running"], p.ID)
		if strings.TrimSpace(first) != "" {
			msg += "\nPrimeiros 2 s de saída:\n" + clip(first)
		}
		return msg, state != "running" && p.Exit != nil && *p.Exit != 0
	}
	// Track the working directory across commands, as a human shell would.
	wrapped := command + "\nprintf '\\n__INFUSE_CWD__%s' \"$PWD\""
	cctx, cancel := context.WithTimeout(ctx, time.Duration(in.Timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, FindRealBash(), "-c", wrapped)
	noConsoleWindow(cmd)
	cmd.Dir = tc.Cwd
	cmd.Env = append(os.Environ(), "INFUSE_AGENT=1", "GIT_TERMINAL_PROMPT=0", "CI=1")
	buf := newCapWriter(1 << 20) // head and tail kept: unbounded output cannot exhaust memory
	cmd.Stdout, cmd.Stderr = buf, buf
	err := cmd.Run()
	out := buf.String()
	if i := strings.LastIndex(out, "__INFUSE_CWD__"); i >= 0 {
		if nc := strings.TrimSpace(out[i+len("__INFUSE_CWD__"):]); nc != "" {
			if abs := FromMsysPath(nc); within(tc.Root, abs) {
				tc.S.setCwd(abs)
			}
		}
		out = strings.TrimRight(out[:i], "\n")
	}
	isErr := false
	if cctx.Err() == context.DeadlineExceeded {
		out += fmt.Sprintf("\n[tempo esgotado após %ds]", in.Timeout)
		isErr = true
	} else if err != nil {
		out += fmt.Sprintf("\n[exit %d]", exitCode(err))
		isErr = true
	}
	trailer := buildTrailer(tc.Root, tc.Cwd, in.Command, tr.Actions, started, tc.Rules, append(guardNotes, ruleLines...))
	if trailer != "" {
		out = strings.TrimRight(out, "\n") + "\n\n" + trailer
	}
	return clip(out), isErr
}

// ---------------------------------------------------------------- Read

type readTool struct{}

func (readTool) Category() string { return "read" }
func (readTool) Spec() ToolSpec {
	return ToolSpec{Name: "Read", Description: "Lê um arquivo do projeto com números de linha. Use offset/limit para arquivos grandes.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","description":"Linha inicial (1-based)"},"limit":{"type":"integer","description":"Quantidade de linhas"}},"required":["path"]}`)}
}

func (readTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path          string `json:"path"`
		Offset, Limit int
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, false)
	if err != nil {
		return err.Error(), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if mt := imageType(abs); mt != "" { // the model sees the image itself
		if len(b) > 5<<20 {
			return fmt.Sprintf("imagem grande demais para enviar ao modelo (%d bytes; máximo 5 MB)", len(b)), true
		}
		tc.S.mu.Lock()
		tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: mt, Data: base64.StdEncoding.EncodeToString(b)})
		tc.S.mu.Unlock()
		tc.S.markRead(abs)
		return fmt.Sprintf("imagem %s (%d bytes) anexada a esta resposta para você ver", relTo(tc.Root, abs), len(b)), false
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return fmt.Sprintf("arquivo binário (%d bytes)", len(b)), true
	}
	lines := strings.Split(string(b), "\n")
	start := in.Offset
	if start < 1 {
		start = 1
	}
	end := len(lines)
	if in.Limit > 0 && start-1+in.Limit < end {
		end = start - 1 + in.Limit
	}
	if start > len(lines) {
		return "", false
	}
	var sb strings.Builder
	for i := start - 1; i < end; i++ {
		fmt.Fprintf(&sb, "%6d\t%s\n", i+1, lines[i])
	}
	tc.S.markRead(abs)
	out := sb.String()
	if m := LoadMap(tc.Root); m != nil {
		if d := DescribeDependents(m, relTo(tc.Root, abs)); d != "" {
			out += "\n" + d
		}
	}
	return clip(out), false
}

// ---------------------------------------------------------------- Write

type writeTool struct{}

func (writeTool) Category() string { return "write" }
func (writeTool) Spec() ToolSpec {
	return ToolSpec{Name: "Write", Description: "Cria ou sobrescreve um arquivo com o conteúdo completo. Para mudanças pontuais em arquivo existente, prefira Edit.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)}
}

func (writeTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Path, Content string }
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return err.Error(), true
	}
	tc.S.checkpointBeforeWrite(abs)
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{fmt.Sprintf("Arquivo escrito: %s (%d bytes)", relTo(tc.Root, abs), len(in.Content))}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// ---------------------------------------------------------------- Edit

type editTool struct{}

func (editTool) Category() string { return "write" }
func (editTool) Spec() ToolSpec {
	return ToolSpec{Name: "Edit", Description: "Substitui um trecho exato de um arquivo. old_string precisa ocorrer exatamente uma vez (ou use replace_all). Leia o arquivo antes de editar.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"]}`)}
}

func (editTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return err.Error(), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if !tc.S.wasRead(abs) {
		return "leia o arquivo com Read antes de editá-lo", true
	}
	content := string(b)
	n := strings.Count(content, in.OldString)
	if in.OldString == "" || n == 0 {
		return "old_string não encontrado no arquivo", true
	}
	if n > 1 && !in.ReplaceAll {
		return fmt.Sprintf("old_string ocorre %d vezes; torne-o único ou use replace_all", n), true
	}
	tc.S.checkpointBeforeWrite(abs)
	next := strings.Replace(content, in.OldString, in.NewString, map[bool]int{true: -1, false: 1}[in.ReplaceAll])
	if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{fmt.Sprintf("Editado: %s (%d substituição(ões))", relTo(tc.Root, abs), map[bool]int{true: n, false: 1}[in.ReplaceAll])}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// ---------------------------------------------------------------- MultiEdit

type multiEditTool struct{}

func (multiEditTool) Category() string { return "write" }
func (multiEditTool) Spec() ToolSpec {
	return ToolSpec{Name: "MultiEdit", Description: "Aplica várias substituições exatas num mesmo arquivo, em ordem, de uma vez: ou todas valem, ou nenhuma é gravada. Cada old_string precisa ocorrer exatamente uma vez no texto já com as edições anteriores (ou use replace_all). Leia o arquivo antes.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old_string","new_string"]}}},"required":["path","edits"]}`)}
}

func (multiEditTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path  string `json:"path"`
		Edits []struct {
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		} `json:"edits"`
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return err.Error(), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if !tc.S.wasRead(abs) {
		return "leia o arquivo com Read antes de editá-lo", true
	}
	if len(in.Edits) == 0 {
		return "nenhuma edição", true
	}
	content := string(b)
	total := 0
	for i, e := range in.Edits {
		n := strings.Count(content, e.OldString)
		if e.OldString == "" || n == 0 {
			return fmt.Sprintf("edição %d: old_string não encontrado (nada foi gravado)", i+1), true
		}
		if n > 1 && !e.ReplaceAll {
			return fmt.Sprintf("edição %d: old_string ocorre %d vezes; torne-o único ou use replace_all (nada foi gravado)", i+1, n), true
		}
		if e.ReplaceAll {
			content = strings.ReplaceAll(content, e.OldString, e.NewString)
			total += n
		} else {
			content = strings.Replace(content, e.OldString, e.NewString, 1)
			total++
		}
	}
	tc.S.checkpointBeforeWrite(abs)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{fmt.Sprintf("Editado: %s (%d edições, %d substituição(ões))", relTo(tc.Root, abs), len(in.Edits), total)}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// imageType is the media type of an image the model can see, by extension.
func imageType(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// ---------------------------------------------------------------- Glob

type globTool struct{}

func (globTool) Category() string { return "read" }
func (globTool) Spec() ToolSpec {
	return ToolSpec{Name: "Glob", Description: "Encontra arquivos por padrão (ex.: src/**/*.ts). Ordenado por modificação, mais recentes primeiro.",
		InputSchema: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string","description":"Pasta base (padrão: raiz do projeto)"}},"required":["pattern"]}`)}
}

func globToRegex(pat string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch {
		case c == '*' && i+1 < len(pat) && pat[i+1] == '*':
			b.WriteString(".*")
			i++
			if i+1 < len(pat) && pat[i+1] == '/' {
				i++
				b.WriteString("(?:/|)")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			j := strings.IndexByte(pat[i:], '}')
			if j > 0 {
				alts := strings.Split(pat[i+1:i+j], ",")
				for k := range alts {
					alts[k] = regexp.QuoteMeta(alts[k])
				}
				b.WriteString("(?:" + strings.Join(alts, "|") + ")")
				i += j
				continue
			}
			b.WriteString(`\{`)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return regexp.MustCompile("^$")
	}
	return re
}

func (globTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Pattern, Path string }
	_ = json.Unmarshal(input, &in)
	base := tc.Root
	if in.Path != "" {
		if abs, err := tc.resolvePath(in.Path, false); err == nil {
			base = abs
		}
	}
	re := globToRegex(strings.TrimPrefix(in.Pattern, "./"))
	type hit struct {
		p string
		t time.Time
	}
	var hits []hit
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != base && ignoredPath(tc.Root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if re.MatchString(toPosix(rel)) {
			info, _ := d.Info()
			t := time.Time{}
			if info != nil {
				t = info.ModTime()
			}
			hits = append(hits, hit{toPosix(rel), t})
		}
		if len(hits) >= 2000 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Slice(hits, func(i, j int) bool { return hits[i].t.After(hits[j].t) })
	var sb strings.Builder
	for _, h := range hits {
		sb.WriteString(h.p + "\n")
	}
	if len(hits) == 0 {
		return "nenhum arquivo encontrado", false
	}
	return clip(sb.String()), false
}

// ---------------------------------------------------------------- Grep

type grepTool struct{}

func (grepTool) Category() string { return "read" }
func (grepTool) Spec() ToolSpec {
	return ToolSpec{Name: "Grep", Description: "Busca por expressão regular no conteúdo dos arquivos. Devolve arquivo:linha:texto.",
		InputSchema: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string","description":"Filtrar arquivos, ex.: *.ts"},"ignore_case":{"type":"boolean"},"context":{"type":"integer","description":"Linhas de contexto"},"files_only":{"type":"boolean"}},"required":["pattern"]}`)}
}

func (grepTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		IgnoreCase bool   `json:"ignore_case"`
		Context    int    `json:"context"`
		FilesOnly  bool   `json:"files_only"`
	}
	_ = json.Unmarshal(input, &in)
	pat := in.Pattern
	if in.IgnoreCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return "regex inválida: " + err.Error(), true
	}
	base := tc.Root
	if in.Path != "" {
		if abs, err := tc.resolvePath(in.Path, false); err == nil {
			base = abs
		}
	}
	var fileRe *regexp.Regexp
	if in.Glob != "" {
		fileRe = globToRegex("**/" + in.Glob)
	}
	var sb strings.Builder
	matches := 0
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || matches > 500 {
			return nil
		}
		if d.IsDir() {
			if p != base && ignoredPath(tc.Root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(tc.Root, p)
		rel = toPosix(rel)
		if fileRe != nil && !fileRe.MatchString(rel) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 2_000_000 || bytes.IndexByte(b, 0) >= 0 {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		fileHit := false
		for i, l := range lines {
			if !re.MatchString(l) {
				continue
			}
			matches++
			if in.FilesOnly {
				if !fileHit {
					sb.WriteString(rel + "\n")
				}
				fileHit = true
				continue
			}
			from, to := i-in.Context, i+in.Context
			if from < 0 {
				from = 0
			}
			if to >= len(lines) {
				to = len(lines) - 1
			}
			for j := from; j <= to; j++ {
				sep := "-"
				if j == i {
					sep = ":"
				}
				fmt.Fprintf(&sb, "%s:%d%s%s\n", rel, j+1, sep, lines[j])
			}
			if in.Context > 0 {
				sb.WriteString("--\n")
			}
		}
		return nil
	})
	if matches == 0 {
		return "nenhuma ocorrência", false
	}
	return clip(sb.String()), false
}

// ---------------------------------------------------------------- LS

type lsTool struct{}

func (lsTool) Category() string { return "read" }
func (lsTool) Spec() ToolSpec {
	return ToolSpec{Name: "LS", Description: "Lista uma pasta do projeto. A resposta inclui o mapa da pasta: o que cada arquivo exporta, quem depende dele, arquivos grandes demais.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"}},"required":[]}`)}
}

func (lsTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Path string }
	_ = json.Unmarshal(input, &in)
	if in.Path == "" {
		in.Path = "."
	}
	abs, err := tc.resolvePath(in.Path, false)
	if err != nil {
		return err.Error(), true
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return err.Error(), true
	}
	var sb strings.Builder
	here := relTo(tc.Root, abs)
	for _, e := range entries {
		if hiddenInternal(filepath.ToSlash(filepath.Join(here, e.Name()))) {
			continue // infuse's own records are not listed as part of the project
		}
		if e.IsDir() {
			sb.WriteString(e.Name() + "/\n")
		} else {
			sb.WriteString(e.Name() + "\n")
		}
	}
	rel := relTo(tc.Root, abs)
	if rel == "" {
		rel = "."
	}
	m := LoadOrBuildMap(tc.Root)
	if d := DescribeDir(m, rel); d != "" {
		sb.WriteString("\n" + d + "\n")
	}
	for _, l := range ApplyListRules(tc.Root, tc.Rules, rel) {
		sb.WriteString(l + "\n")
	}
	return clip(sb.String()), false
}

// ---------------------------------------------------------------- WebFetch

type webFetchTool struct{}

func (webFetchTool) Category() string { return "net" }
func (webFetchTool) Spec() ToolSpec {
	return ToolSpec{Name: "WebFetch", Description: "Baixa uma página ou documento da web e devolve o texto (HTML convertido para texto), em partes de 30000 caracteres. Uma página em partes precisa ser lida até o fim (offset) antes de você executar comandos ou alterar arquivos.",
		InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string"},"offset":{"type":"integer","description":"onde continuar a leitura (indicado no fim de cada parte)"}},"required":["url"]}`)}
}

var reTag = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]+>`)
var reWs = regexp.MustCompile(`\n\s*\n+`)

func (webFetchTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		URL    string
		Offset int
	}
	_ = json.Unmarshal(input, &in)
	if bad, why := privateTarget(in.URL); bad { // no requests into this machine or the private network
		Journal(tc.Root, JournalEntry{Kind: "blocked", Command: in.URL, Detail: "WebFetch recusado: " + why})
		return "WebFetch recusado: " + why + ". O agente só busca endereços públicos.", true
	}
	Journal(tc.Root, JournalEntry{Kind: "fetch", Command: in.URL, Detail: "pesquisa"})
	req, err := http.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return err.Error(), true
	}
	req.Header.Set("User-Agent", "infuse/"+version)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err.Error(), true
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	text := string(b)
	if strings.Contains(resp.Header.Get("content-type"), "html") {
		text = reTag.ReplaceAllString(text, " ")
		text = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(text)
		text = reWs.ReplaceAllString(text, "\n\n")
	}
	text = strings.TrimSpace(text)
	total := max(1, (len(text)+docPageSize-1)/docPageSize)
	part := min(max(in.Offset, 0)/docPageSize, total-1)
	body := text[part*docPageSize : min(len(text), (part+1)*docPageSize)]
	head := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if total > 1 {
		head += fmt.Sprintf(" · parte %d de %d", part+1, total)
		if part+1 < total {
			body += fmt.Sprintf("\n\n[continua: WebFetch com offset=%d — leia até o fim antes de agir]", (part+1)*docPageSize)
		}
	}
	if tc.S != nil && resp.StatusCode < 400 {
		tc.S.docs.markPart(in.URL, part, total)
	}
	return externalContent(in.URL, head+"\n\n"+body), resp.StatusCode >= 400
}

// ---------------------------------------------------------------- TodoWrite

type TaskItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`          // pending | in_progress | done | blocked
	Proof  string `json:"proof,omitempty"` // how the task will be shown to work (a command, a page, a file)
	Note   string `json:"note,omitempty"`  // set by infuse, never by the model (why a task was closed as blocked)
}

type todoTool struct{}

func (todoTool) Category() string { return "ui" }
func (todoTool) Spec() ToolSpec {
	return ToolSpec{Name: "TodoWrite", Description: "O plano do pedido, visível ao usuário e marcado em tempo real. Obrigatório antes de alterar arquivos. Envie a lista completa a cada atualização; cada tarefa aberta com proof (como será comprovada). Status: pending, in_progress, done, blocked (com o motivo no Report). O turno só termina com todas done ou blocked.",
		InputSchema: schema(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"text":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","done","blocked"]},"proof":{"type":"string","description":"como esta tarefa será comprovada: o comando, a página ou o arquivo que mostra que funciona"}},"required":["id","text","status"]}}},"required":["tasks"]}`)}
}

func (todoTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Tasks []TaskItem `json:"tasks"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return err.Error(), true
	}
	if len(in.Tasks) == 0 {
		return "o plano precisa de pelo menos uma tarefa", true
	}
	var noProof []string
	for _, t := range in.Tasks {
		if t.Status != "done" && t.Status != "blocked" && strings.TrimSpace(t.Proof) == "" {
			noProof = append(noProof, t.Text)
		}
	}
	if len(noProof) > 0 { // the protocol: every open task says how it will be proven
		return "plano recusado: cada tarefa aberta precisa de proof (o comando, a página ou o arquivo que comprovará que funciona). Sem proof: " + strings.Join(noProof, "; "), true
	}
	for i := range in.Tasks {
		in.Tasks[i].Note = ""
	}
	tc.S.setTasks(in.Tasks)
	tc.S.mu.Lock()
	tc.S.gate.planned = true
	tc.S.mu.Unlock()
	return fmt.Sprintf("plano registrado (%d tarefas); o usuário vê cada uma marcada conforme você atualiza", len(in.Tasks)), false
}

// ---------------------------------------------------------------- AskUserQuestion

type askTool struct{}

func (askTool) Category() string { return "ui" }
func (askTool) Spec() ToolSpec {
	return ToolSpec{Name: "AskUserQuestion", Description: "Faz uma pergunta ao usuário na interface, com opções. Use quando uma decisão é do usuário e não pode ser inferida.",
		InputSchema: schema(`{"type":"object","properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"}}},"required":["question"]}`)}
}

func (askTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	_ = json.Unmarshal(input, &in)
	answer, ok := tc.S.askUser(ctx, in.Question, in.Options)
	if !ok {
		return "o usuário não respondeu", true
	}
	return "Resposta do usuário: " + answer, false
}

func init() {
	for _, t := range []Tool{bashTool{}, readTool{}, writeTool{}, editTool{}, multiEditTool{}, globTool{}, grepTool{}, lsTool{}, webFetchTool{}, todoTool{}, askTool{}} {
		registerTool(t)
	}
}
