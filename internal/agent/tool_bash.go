package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Bash: the filtered shell, background processes and output trailers.

// ---------------------------------------------------------------- Bash

type bashTool struct{}

func (bashTool) Category() string { return "exec" }
func (bashTool) Spec() ToolSpec {
	return ToolSpec{Name: "Bash", Description: "Runs a command in the project's shell (bash). Use it to run builds, tests, git and utilities. The output comes back with project information when relevant. run_in_background=true for something that keeps running (server, game, watcher): returns at once with the id; read the output with BashOutput and stop it with KillShell. You are notified when the process ends.",
		InputSchema: schema(`{"type":"object","properties":{"command":{"type":"string","description":"Command to run"},"timeout":{"type":"integer","description":"Timeout in seconds (default 120, maximum 600)"},"description":{"type":"string","description":"Short description of what the command does"},"run_in_background":{"type":"boolean","description":"runs without waiting for it to finish"}},"required":["command"]}`)}
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
		return tc.Say(M("bash.empty")), true
	}
	if in.Timeout <= 0 {
		in.Timeout = 120
	}
	if in.Timeout > 600 {
		in.Timeout = 600
	}
	started := time.Now().UnixMilli()
	// The guard runs first: protected targets are refused or neutralized before the transform can write.
	guarded, guardNotes, gerr := newGuard(tc.Root, tc.S.settings.ProtectedPaths).GuardCommandMsgs(in.Command, tc.Cwd)
	if gerr != nil {
		return tc.SayErr(gerr), true
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
	command, ruleLines := ApplyCommandRulesMsgs(tc.Root, tc.Rules, tr.Command)
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
			return tc.Say(M("bash.not_started", "error", errArg(err))), true
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
		tc.S.Mu.Lock()
		p.read = off
		state := p.State
		tc.S.Mu.Unlock()
		m := M("bash.bg_running", "id", p.ID, "pid", p.PID)
		if state != "running" {
			m = M("bash.bg_finished", "id", p.ID, "pid", p.PID)
		}
		msg := tc.Say(m)
		if strings.TrimSpace(first) != "" {
			msg += "\n" + tc.Say(M("bash.bg_first")) + "\n" + tc.clip(first)
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
		out += "\n" + tc.Say(M("bash.timeout", "seconds", in.Timeout))
		isErr = true
	} else if err != nil {
		out += fmt.Sprintf("\n[exit %d]", exitCode(err))
		isErr = true
	}
	var trailer []string
	for _, m := range trailerMsgs(tc.Root, tc.Cwd, in.Command, tr.Actions, started, tc.Rules, append(guardNotes, ruleLines...)) {
		trailer = append(trailer, tc.Say(m)) // each note recorded: the user reads it in their language
	}
	if len(trailer) > 0 {
		out = strings.TrimRight(out, "\n") + "\n\n" + strings.Join(trailer, "\n")
	}
	return tc.clip(out), isErr
}
