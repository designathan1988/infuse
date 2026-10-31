package main

// The user's prohibitions (docs/FALHAS-DE-AGENTES.md F4, F5). What the user forbids in a session —
// "no subagents", "don't run the game", "don't touch src/legacy", "don't commit" — binds the agent until
// the user lifts it. The meaning of the user's words, in any language, is compiled by a model into
// structured prohibitions, each with the exact words of the user's message it comes from; code keeps
// only those whose quote is in the user's messages. Enforcement is mechanical: a tool name, a fragment
// of a command, a path, an action that maps to commands (commit, push, install). The user sees them and
// can remove any.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Ban struct {
	Kind  string `json:"kind"`          // tool | command | path | action
	Value string `json:"value"`         // tool name, command fragment, path, or commit|push|install
	Quote string `json:"quote"`         // the user's own words
	Turn  int    `json:"turn"`          // when it was said
	Off   bool   `json:"off,omitempty"` // lifted (by the user on screen, or by a later message)
}

var banStore sync.Map // session id → *[]Ban (persisted in .infuse/sessions/<id>.bans.json)

func bansFile(s *Session) string { return filepath.Join(sessionsDir(s.Root), s.ID+".bans.json") }

// Bans returns the session's prohibitions.
func (s *Session) Bans() []Ban {
	if v, ok := banStore.Load(s.ID); ok {
		return append([]Ban{}, *v.(*[]Ban)...)
	}
	var list []Ban
	_ = readJSON(bansFile(s), &list)
	banStore.Store(s.ID, &list)
	return append([]Ban{}, list...)
}

func (s *Session) setBans(list []Ban) {
	banStore.Store(s.ID, &list)
	_ = writeJSON(bansFile(s), list)
	s.emit("bans", list)
}

var toolNames = map[string]bool{"Agent": true, "Browser": true, "WebSearch": true, "WebFetch": true, "Bash": true, "Write": true, "Edit": true, "MultiEdit": true, "Skill": true, "Memory": true}

const bansPrompt = `You read the messages a user wrote to a coding agent (any language). List every prohibition or limit the user stated for this session that is still in force, by calling Prohibitions:
- tool: a tool the agent must not use; value is one of: Agent (subagents), Browser, WebSearch, WebFetch, Bash, Write, Edit, Skill.
- command: a program or command the agent must not run; value is the exact command or program name as the user wrote it.
- path: a file or folder the agent must not change; value is the path as written.
- action: value is commit (no git commit), push (no git push) or install (no package installation).
quote: words copied EXACTLY from the user's message where the prohibition is stated, in the user's language, 3 to 30 words.
Only explicit prohibitions. A later message that allows something again removes it from the list. If there are none, call Prohibitions with an empty list.`

// bansSpec is the shape of the compiled prohibitions.
var bansSpec = ToolSpec{Name: "Prohibitions", Description: "The prohibitions the user stated that are still in force, each with the user's own words.",
	InputSchema: schema(`{"type":"object","properties":{"bans":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","enum":["tool","command","path","action"]},"value":{"type":"string"},"quote":{"type":"string"}},"required":["kind","value","quote"]}}},"required":["bans"]}`)}

// compileBans reads the user's messages and replaces the session's prohibitions with what is in force,
// keeping the ones the user removed on screen removed.
func (s *Session) compileBans(ctx context.Context, p Provider) {
	if s.settings.UserProhibitions != nil && !*s.settings.UserProhibitions {
		return
	}
	text := s.userWords()
	if strings.TrimSpace(text) == "" {
		return
	}
	var out struct {
		Bans []Ban `json:"bans"`
	}
	usage, err := askJSON(ctx, p, s.fastModel(), bansPrompt, text, bansSpec, 4000, &out)
	s.mu.Lock()
	s.Cost += s.settings.costOf(s.fastModel(), usage)
	turn := s.Turn
	s.mu.Unlock()
	if err != nil { // the prohibitions already in force stay; the user is told the new words were not read
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "compiling the prohibitions: " + err.Error()})
		s.emitMsg("gate", M("ban.compile_failed", "error", firstLine(err.Error())))
		return
	}
	norm := normSpace(text)
	old := s.Bans()
	removed := map[string]bool{}
	when := map[string]int{}
	for _, b := range old {
		key := b.Kind + "|" + strings.ToLower(b.Value)
		when[key] = b.Turn
		if b.Off {
			removed[key] = true
		}
	}
	var list []Ban
	for _, b := range out.Bans {
		b.Value = strings.TrimSpace(b.Value)
		q := normSpace(b.Quote)
		if b.Value == "" || len(q) < 6 || !strings.Contains(norm, q) { // a prohibition the user did not write
			continue
		}
		switch b.Kind {
		case "tool":
			if !toolNames[b.Value] {
				continue
			}
		case "action":
			if b.Value != "commit" && b.Value != "push" && b.Value != "install" {
				continue
			}
		case "command", "path":
		default:
			continue
		}
		key := b.Kind + "|" + strings.ToLower(b.Value)
		b.Turn = turn
		if t, ok := when[key]; ok {
			b.Turn = t
		}
		b.Off = removed[key]
		list = append(list, b)
	}
	s.setBans(list)
}

// banCheck refuses a tool call that a prohibition in force forbids.
func (s *Session) banCheck(name string, input json.RawMessage) Msg {
	cmd := ""
	if name == "Bash" {
		cmd = strings.ToLower(bashCommand(input))
	}
	target := filepath.ToSlash(toolPath(input))
	for _, b := range s.Bans() {
		if b.Off {
			continue
		}
		v := strings.ToLower(b.Value)
		hit := false
		switch b.Kind {
		case "tool":
			hit = name == b.Value || (b.Value == "Edit" && name == "MultiEdit") || (b.Value == "Write" && (name == "Edit" || name == "MultiEdit"))
		case "command":
			hit = cmd != "" && strings.Contains(cmd, v)
		case "path":
			if target != "" && (name == "Write" || name == "Edit" || name == "MultiEdit") {
				rel := strings.ToLower(filepath.ToSlash(relTo(s.Root, filepath.Join(s.Cwd, target))))
				if filepath.IsAbs(target) {
					rel = strings.ToLower(filepath.ToSlash(relTo(s.Root, target)))
				}
				p := strings.Trim(strings.TrimPrefix(v, "./"), "/")
				hit = rel == p || strings.HasPrefix(rel, p+"/")
			}
		case "action":
			switch v {
			case "commit":
				hit = strings.Contains(cmd, "git commit")
			case "push":
				hit = strings.Contains(cmd, "git push")
			case "install":
				hit = len(packagesInCommand(cmd)) > 0 || strings.Contains(cmd, "go get ")
			}
		}
		if hit {
			return M("ban.refused", "quote", b.Quote, "turn", b.Turn)
		}
	}
	return Msg{}
}

// liftBan is the user removing (or restoring) a prohibition on screen.
func (s *Session) liftBan(i int, off bool) []Ban {
	list := s.Bans()
	if i >= 0 && i < len(list) {
		list[i].Off = off
		s.setBans(list)
	}
	return list
}

var _ = time.Now
