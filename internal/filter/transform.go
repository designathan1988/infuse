package filter

import (
	"fmt"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
)

type ActionKind string

const (
	ActHeredocToFile  ActionKind = "heredoc-to-file"
	ActHeredocToStdin ActionKind = "heredoc-to-stdin"
	ActLesson         ActionKind = "lesson"
)

type TransformAction struct {
	Kind   ActionKind
	Detail string
	Path   string
	Bytes  int
	Vice   string
}

type TransformResult struct {
	Command string
	Actions []TransformAction
}

// TransformCommand rewrites a shell command so that no heredoc ever reaches bash.
//
//   - `cat > file <<EOF ... EOF` and `tee file <<EOF ...` become direct file writes
//     performed here (exact bytes) when the delimiter is quoted or the body is code; the command
//     line is replaced by a no-op.
//   - Unquoted template heredocs become a double-quoted printf in the same shell, so expansion keeps
//     bash semantics with the same variables.
//   - Any other heredoc (`python3 <<EOF`...) has its body routed through a temp file or a process
//     substitution.
func TransformCommand(cmd, cwd string) TransformResult {
	hs := FindHeredocs(cmd)
	if len(hs) == 0 {
		return TransformResult{Command: cmd}
	}
	var actions []TransformAction
	out := cmd
	for i := len(hs) - 1; i >= 0; i-- { // last to first so earlier offsets stay valid
		h := hs[i]
		lineStart, lineEnd, lineText := CommandLineOf(out, h)
		intent := DetectFileWrite(lineText)
		literal := h.Quoted

		if !h.Quoted && intent != nil && LooksLikeLiteralIntent(h.Body) {
			literal = true
			actions = append(actions, TransformAction{Kind: ActLesson, Vice: "unquoted-heredoc-with-code",
				Detail: fmt.Sprintf("heredoc for %s had an unquoted delimiter but contains `/$(/${; treated as literal", intent.Path)})
		}
		if !h.TerminatorFound {
			actions = append(actions, TransformAction{Kind: ActLesson, Vice: "heredoc-missing-terminator",
				Detail: fmt.Sprintf("heredoc %q never terminated; end of command used as terminator", h.Delimiter)})
		}

		switch {
		case intent != nil && literal:
			action, line := heredocDirectWrite(cwd, intent, h.Body)
			actions = append(actions, action)
			out = spliceHeredoc(out, h, lineStart, line)
		case intent != nil:
			action, line := heredocPrintf(cwd, intent, h.Body)
			actions = append(actions, action)
			out = spliceHeredoc(out, h, lineStart, line)
		case literal:
			if action, line, ok := heredocTempFile(h, out, lineEnd); ok {
				actions = append(actions, action)
				out = spliceHeredoc(out, h, h.OpStart, line)
			}
		default:
			action, line := heredocProcessSubst(h, out, lineEnd)
			actions = append(actions, action)
			out = spliceHeredoc(out, h, h.OpStart, line)
		}
	}
	return TransformResult{Command: out, Actions: actions}
}

// spliceHeredoc replaces the command text from from through the heredoc block's end with line.
func spliceHeredoc(out string, h Heredoc, from int, line string) string {
	return out[:from] + line + "\n" + out[h.BlockEnd:]
}

// heredocDirectWrite writes a literal body to the intent's file and returns its action and the no-op
// line that replaces the command.
func heredocDirectWrite(cwd string, intent *FileWriteIntent, body string) (TransformAction, string) {
	target := ResolveIn(cwd, core.FromMsysPath(intent.Path))
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if intent.Append {
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	if f, err := os.OpenFile(target, flag, 0o644); err == nil {
		_, _ = f.WriteString(body)
		_ = f.Close()
	}
	verb := "wrote"
	if intent.Append {
		verb = "appended"
	}
	action := TransformAction{Kind: ActHeredocToFile, Path: target, Bytes: len(body),
		Detail: fmt.Sprintf("%s %s (%d bytes) without heredoc", verb, intent.Path, len(body))}
	return action, ":"
}

// heredocPrintf rewrites an expanded body as a double-quoted printf in the same shell, keeping
// expansion semantics.
func heredocPrintf(cwd string, intent *FileWriteIntent, body string) (TransformAction, string) {
	redir := ">"
	verb := "write"
	if intent.Append {
		redir, verb = ">>", "append to"
	}
	line := fmt.Sprintf("printf '%%s' %s %s %s", doubleQuote(body), redir, ShellQuote(intent.Path))
	action := TransformAction{Kind: ActHeredocToFile, Path: ResolveIn(cwd, core.FromMsysPath(intent.Path)), Bytes: len(body),
		Detail: fmt.Sprintf("%s %s via printf (shell expansion kept) without heredoc", verb, intent.Path)}
	return action, line
}

// heredocTempFile routes a literal body through a temp file fed to the command's stdin. ok is false
// when the temp file cannot be created.
func heredocTempFile(h Heredoc, out string, lineEnd int) (action TransformAction, line string, ok bool) {
	tmp, err := os.CreateTemp("", "infuse-hd-*")
	if err != nil {
		return TransformAction{}, "", false
	}
	_, _ = tmp.WriteString(h.Body)
	_ = tmp.Close()
	action = TransformAction{Kind: ActHeredocToStdin, Path: tmp.Name(), Bytes: len(h.Body),
		Detail: "heredoc body routed through " + tmp.Name()}
	afterOp := out[h.OpEnd:lineEnd]
	return action, "< " + ShellQuote(core.ToBashPath(tmp.Name())) + afterOp, true
}

// heredocProcessSubst routes an expanded body through process substitution.
func heredocProcessSubst(h Heredoc, out string, lineEnd int) (TransformAction, string) {
	action := TransformAction{Kind: ActHeredocToStdin, Bytes: len(h.Body),
		Detail: "heredoc body routed through process substitution (shell expansion kept)"}
	afterOp := out[h.OpEnd:lineEnd]
	return action, "< <(printf '%s' " + doubleQuote(h.Body) + ")" + afterOp
}

// doubleQuote wraps a heredoc body in double quotes preserving unquoted-heredoc semantics:
// `$`, backticks and `\` behave the same; only `"` needs escaping, and a pre-existing `\"`
// (literal in a heredoc) must survive as `\"`.
func doubleQuote(body string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) && body[i+1] == '"' {
			b.WriteString(`\\\"`)
			i++
			continue
		}
		if body[i] == '"' {
			b.WriteString(`\"`)
			continue
		}
		b.WriteByte(body[i])
	}
	b.WriteByte('"')
	return b.String()
}

func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func ResolveIn(cwd, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}
