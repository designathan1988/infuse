package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Tool dispatch and the policy pipeline around each call.

func (s *Session) addNote(kind string, m Msg) {
	s.Mu.Lock()
	s.Messages = append(s.Messages, Message{Role: "user", Kind: kind, Content: []Block{s.textBlockLocked(m)}, TS: time.Now().UnixMilli()})
	s.Mu.Unlock()
	s.emitMsg("gate", m)
}

// failMsg ends the turn with an error the user reads in their language.
func (s *Session) failMsg(m Msg) { s.fail(errors.New(m.Render(s.uiLang()))) }

// refuse is a gate's refusal of a call: journaled, shown to the user, recorded with the call.
func (s *Session) refuse(callID, command string, m Msg) (string, bool) {
	Journal(s.Root, JournalEntry{Kind: "gate", Command: command, Detail: firstLine(m.Render(s.uiLang()))})
	s.emitMsg("gate", m)
	return s.say(callID, m), true
}

func hasResult(results []Block, id string) bool {
	for _, r := range results {
		if r.ToolUseID == id {
			return true
		}
	}
	return false
}

// readsOutside reports whether a read tool's path (path / file_path) points outside the session's folder.
func (s *Session) readsOutside(input json.RawMessage) bool {
	var in struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	_ = json.Unmarshal(input, &in)
	p := in.Path
	if p == "" {
		p = in.FilePath
	}
	if p == "" {
		return false
	}
	p = FromMsysPath(p)
	s.Mu.Lock()
	cwd, root := s.Cwd, s.workRoot()
	s.Mu.Unlock()
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return !within(root, p)
}

func (s *Session) runTool(ctx context.Context, rules *RuleSet, b Block) (string, bool) {
	tool, ok := s.lookupTool(b.Name)
	if !ok {
		return s.say(b.ID, M("session.unknown_tool", "tool", b.Name)), true
	}
	var toolInput any
	_ = json.Unmarshal(b.Input, &toolInput)
	if o := s.runHooks(ctx, "PreToolUse", b.Name, map[string]any{"tool_name": b.Name, "tool_input": toolInput}); o.Block {
		return s.say(b.ID, o.blockMsg()), true
	}
	if m := s.banCheck(b.Name, b.Input); !m.Empty() { // what the user forbade in this session
		return s.refuse(b.ID, bashCommand(b.Input), m)
	}
	if inv, _ := isResearch(b.Name, b.Input); inv && b.Name != "Read" || b.Name == "Read" && isSourceRead(b.Input) {
		s.noteResearch(b.Name, b.Input)
	}
	if c := tool.Category(); (c == "write" || c == "exec") && !(b.Name == "Bash" && helpCommand(bashCommand(b.Input))) {
		if why := s.requiredReading(); !why.Empty() { // the documents the instructions point to, read in full
			return s.refuse(b.ID, "", why)
		}
	}
	if m := s.diagnosisGate(b.Name, tool.Category(), b.Input); !m.Empty() { // the cause, from the project's knowledge, before acting
		return s.refuse(b.ID, "", m)
	}
	if tool.Category() == "write" {
		s.noteFileStateBefore(toolPath(b.Input))
	}
	if tool.Category() == "write" { // the protocol: plan, docs before use, conventions, size
		if why := s.protocolCheckWrite(b.Name, b.Input); !why.Empty() {
			return s.refuse(b.ID, "", why)
		}
		if p, before, after, ok := editedContent(s.workRoot(), s.Cwd, b.Name, b.Input); ok {
			rel := relTo(s.workRoot(), p)
			if why := s.largeRemoval(rel, before, after); !why.Empty() {
				return s.refuse(b.ID, "", why)
			}
			if why := cheatWriteCheck(s.workRoot(), rel, before, after); !why.Empty() {
				if why.ID != "cheat.test_infra" {
					return s.refuse(b.ID, "", why)
				}
				// the test infrastructure may have to change: the user decides, every time
				if !s.askPermissionWhy(ctx, tool, b.Input, "cheat:"+rel, why) {
					return s.say(b.ID, M("perm.user_refused", "why", why)), true
				}
			}
		}
	}
	if b.Name == "Bash" { // docs before commands: options the agent has not read about are refused
		var in struct{ Command string }
		_ = json.Unmarshal(b.Input, &in)
		if why := s.commandDocCheck(in.Command); !why.Empty() {
			return s.refuse(b.ID, in.Command, why)
		}
		if why := swallowedTestCommand(in.Command); !why.Empty() { // a test run judged by its real exit code
			return s.refuse(b.ID, in.Command, why)
		}
		if why := unknownPackages(ctx, in.Command); !why.Empty() { // packages exist before they are installed
			return s.refuse(b.ID, in.Command, why)
		}
	} else if tool.Category() == "write" {
		if open := s.docs.unfinished(); len(open) > 0 {
			return s.refuse(b.ID, "", M("docs.write_partial", "pages", open))
		}
	}
	if why := s.gateCheck(b.Name, tool.Category(), b.Input); !why.Empty() { // research before another attempt
		return s.refuse(b.ID, "", why)
	}
	var shortcutWarn []Msg
	if tool.Category() == "write" && s.mode() != "plan" {
		if p, before, after, ok := editedContent(s.workRoot(), s.Cwd, b.Name, b.Input); ok {
			ask, warn := detectShortcuts(p, before, after)
			shortcutWarn = warn
			if len(ask) > 0 {
				why := M("shortcut.weakens_tests", "items", ask)
				key := "shortcut"
				for _, a := range ask {
					key += ":" + a.ID
				}
				s.Mu.Lock()
				silenced := s.allowAlways["sentinel:"+key]
				s.Mu.Unlock()
				if !silenced {
					Journal(s.Root, JournalEntry{Kind: "sentinel", Path: p, Detail: why.Render(s.uiLang())})
					if !s.askPermissionWhy(ctx, tool, b.Input, key, why) {
						Lesson(s.Root, "atalho-em-teste", why.String()+" in "+p)
						return s.say(b.ID, M("shortcut.user_refused", "why", why)), true
					}
				}
			}
		}
	}
	out, isErr := s.runToolChecked(ctx, rules, tool, b)
	s.gateRecord(b.ID, b.Name, tool.Category(), out, isErr)
	if b.Name == "Bash" {
		s.recordRun(bashCommand(b.Input), !isErr)
	} else if tool.Category() == "write" && !isErr {
		s.noteWrite()
		s.noteFileState(toolPath(b.Input))
		if p := toolPath(b.Input); p != "" {
			abs := p
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(s.Cwd, abs)
			}
			s.noteUIChange(relTo(s.workRoot(), abs))
			s.noteAuthored(abs, writtenTexts(b.Input)...)
		}
	}
	if !isErr {
		switch b.Name {
		case "Browser":
			var in struct{ Action string }
			_ = json.Unmarshal(b.Input, &in)
			if in.Action == "screenshot" || in.Action == "record_stop" {
				s.noteScreenshot()
			}
			s.noteFetched(string(b.Input) + "\n" + out)
		case "WebFetch", "WebSearch":
			s.noteFetched(string(b.Input) + "\n" + out)
		}
	}
	s.noteRead(b.Name, b.Input, out, isErr)
	if b.Name == "Bash" && !isErr {
		var in struct{ Command string }
		_ = json.Unmarshal(b.Input, &in)
		for _, seg := range commandSegments(in.Command) {
			s.docs.succeeded(seg.flags)
		}
	}
	if len(shortcutWarn) > 0 && !isErr {
		Lesson(s.Root, "atalho", strings.Join(msgStrings(shortcutWarn), ", "))
		out += "\n" + s.say(b.ID, M("shortcut.introduced", "items", shortcutWarn))
	}
	for _, m := range s.procNoteMsgs() {
		out += "\n" + s.say(b.ID, m)
	}
	if o := s.runHooks(ctx, "PostToolUse", b.Name, map[string]any{"tool_name": b.Name, "tool_input": toolInput, "tool_response": out}); o.Message != "" {
		out += "\n" + s.say(b.ID, M("hook.post", "message", o.Message))
	}
	return out, isErr
}

// runToolChecked applies sentinels and permissions, then runs the tool.
func (s *Session) runToolChecked(ctx context.Context, rules *RuleSet, tool Tool, b Block) (string, bool) {
	decision := s.permissionFor(tool, b.Input)
	// reading outside the session's folder (~/.ssh, browser profiles…) is never silent: it asks,
	// unless the user chose to accept everything
	if decision == "allow" && tool.Category() == "read" && s.mode() != "bypass" && s.readsOutside(b.Input) {
		decision = "ask"
	}
	// sentinels: deterministic checks that ask the user even when the mode would allow the action
	if decision != "deny" {
		if why := s.sentinelMsg(tool, b.Input); !why.Empty() {
			key := "sentinel:" + why.ID + ":" + toolPath(b.Input)
			s.Mu.Lock()
			silenced := s.allowAlways[key]
			s.Mu.Unlock()
			if !silenced {
				Journal(s.Root, JournalEntry{Kind: "sentinel", Detail: why.Render(s.uiLang())})
				if !s.askPermissionWhy(ctx, tool, b.Input, key, why) {
					Lesson(s.Root, "teste-alterado", why.String())
					return s.say(b.ID, M("perm.user_refused", "why", why)), true
				}
				decision = "allow"
			}
		}
	}
	switch decision {
	case "deny":
		if s.mode() == "plan" {
			return s.say(b.ID, M("perm.plan_mode")), true
		}
		return s.say(b.ID, M("perm.denied_settings")), true
	case "ask":
		if !s.askPermission(ctx, tool, b.Input) {
			return s.say(b.ID, M("perm.user_denied")), true
		}
	}
	s.setState("tool")
	s.emit("tool_run", map[string]any{"id": b.ID, "name": b.Name})
	s.Mu.Lock()
	tc := &ToolContext{CallID: b.ID, S: s, Root: s.workRoot(), Cwd: s.Cwd, Rules: rules}
	s.Mu.Unlock()
	return tool.Run(ctx, tc, b.Input)
}
