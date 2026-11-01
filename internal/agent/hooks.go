package agent

// Hooks, in Claude Code's format: commands that run at fixed points of the agent's work.
//   PreToolUse   before a tool runs (matcher = tool name regex); exit 2 or {"decision":"block"} refuses it
//   PostToolUse  after a tool ran; what it prints to stderr with exit 2 goes back to the agent
//   UserPromptSubmit  before a message reaches the agent; a block stops the message
//   Stop         when the agent finishes; a block makes it continue with the reason
// The hook gets the event as JSON on stdin. Sources: .infuse/hooks.json and .claude/settings.json of the
// project, ~/.infuse/hooks.json for every project. Every hook command runs only after the user approved
// that exact command (the agent can write the project's files).

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/platform/process"
	"infuse/internal/sandbox"
	"infuse/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var hookEvents = []string{"PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop"}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hooksFile struct {
	store.Versioned
	Hooks map[string][]hookGroup `json:"hooks"`
}

// HookEntry is one hook command with where it came from.
type HookEntry struct {
	Source   string `json:"source"` // file path, as shown to the user
	Scope    string `json:"scope"`  // project | global
	Event    string `json:"event"`
	Matcher  string `json:"matcher"`
	Command  string `json:"command"`
	Timeout  int    `json:"timeout"`
	Approved bool   `json:"approved"`
	Key      string `json:"key"`
}

func hooksSources(root string) [][2]string {
	return [][2]string{
		{filepath.Join(root, infuseDirName, "hooks.json"), "project"},
		{filepath.Join(root, ".claude", "settings.json"), "project"},
		{filepath.Join(InfuseHome(), "hooks.json"), "global"},
	}
}

func hookKey(root, scope, event, matcher, command string) string {
	if scope == "global" {
		root = ""
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{filepath.Clean(root), scope, event, matcher, command}, "\x00")))
	return hex.EncodeToString(sum[:])
}

var hookApproveMu sync.Mutex

func hookApprovalsFile() string { return filepath.Join(InfuseHome(), "approved-hooks.json") }

func approvedHooks() map[string]bool {
	hookApproveMu.Lock()
	defer hookApproveMu.Unlock()
	m := map[string]bool{}
	// The approval file is signed by the user's key: an unsigned or edited file approves nothing.
	if err := config.OpenJSON(hookApprovalsFile(), &m); err != nil {
		return map[string]bool{}
	}
	return m
}

func approveHook(key string) error {
	hookApproveMu.Lock()
	defer hookApproveMu.Unlock()
	m := map[string]bool{}
	_ = config.OpenJSON(hookApprovalsFile(), &m)
	m[key] = true
	return config.SealJSON(hookApprovalsFile(), m)
}

// LoadHooks lists every hook command of the project and the global file.
func LoadHooks(root string) []HookEntry {
	approved := approvedHooks()
	var out []HookEntry
	for _, src := range hooksSources(root) {
		var f hooksFile
		if readJSON(src[0], &f) != nil {
			continue
		}
		_, _ = f.Upgrade(nil)
		for _, ev := range hookEvents {
			for _, g := range f.Hooks[ev] {
				for _, h := range g.Hooks {
					if h.Command == "" || (h.Type != "" && h.Type != "command") {
						continue
					}
					k := hookKey(root, src[1], ev, g.Matcher, h.Command)
					out = append(out, HookEntry{Source: src[0], Scope: src[1], Event: ev, Matcher: g.Matcher, Command: h.Command, Timeout: h.Timeout, Approved: approved[k], Key: k})
				}
			}
		}
	}
	return out
}

// addHook appends a hook to .infuse/hooks.json (project) or ~/.infuse/hooks.json (global).
func addHook(root, scope, event, matcher, command string) error {
	p := filepath.Join(root, infuseDirName, "hooks.json")
	if scope == "global" {
		p = filepath.Join(InfuseHome(), "hooks.json")
	}
	var f hooksFile
	_ = readJSON(p, &f)
	_, _ = f.Upgrade(nil)
	if f.Hooks == nil {
		f.Hooks = map[string][]hookGroup{}
	}
	f.Hooks[event] = append(f.Hooks[event], hookGroup{Matcher: matcher, Hooks: []hookCommand{{Type: "command", Command: command}}})
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	f.SchemaVersion = store.Version
	return writeJSON(p, f)
}

// removeHook deletes one hook command from its file.
func removeHook(root, key string) error {
	for _, src := range hooksSources(root) {
		var raw map[string]json.RawMessage
		if readJSON(src[0], &raw) != nil {
			continue
		}
		var f hooksFile
		_ = readJSON(src[0], &f)
		changed := false
		for ev, groups := range f.Hooks {
			for gi := range groups {
				kept := groups[gi].Hooks[:0]
				for _, h := range groups[gi].Hooks {
					if hookKey(root, src[1], ev, groups[gi].Matcher, h.Command) == key {
						changed = true
						continue
					}
					kept = append(kept, h)
				}
				groups[gi].Hooks = kept
			}
		}
		if changed {
			b, _ := json.Marshal(f.Hooks)
			raw["hooks"] = b // other keys of the file (e.g. .claude/settings.json) stay as they were
			return writeJSON(src[0], raw)
		}
	}
	return fmt.Errorf("hook not found")
}

type hookOutcome struct {
	Block   bool
	Message string // the hook's own words (its reason, or the context it adds)
	Command string // the hook that blocked
}

// blockMsg is why a hook blocked, for the model and the user: the hook's reason as it wrote it, or the
// hook's command when it gave none.
func (o hookOutcome) blockMsg() Msg {
	if o.Message != "" {
		return M("hook.blocked", "message", o.Message)
	}
	return M("hook.blocked_cmd", "command", o.Command)
}

// guardedHook judges a hook as the shell command it is: a hook cannot rewrite the oracle, the
// settings or the evidence on the agent's behalf.
func (s *Session) guardedHook(h HookEntry) (string, bool) {
	guarded, _, err := newGuard(s.Root, s.settings.ProtectedPaths).GuardCommand(h.Command, s.workRoot())
	if err != nil {
		Journal(s.Root, JournalEntry{Kind: "blocked", Command: h.Command, Detail: "hook refused: " + err.Error()})
		return "", false
	}
	return guarded, true
}

// runHookCommand executes one approved hook in the sandbox and returns its exit code and output.
func (s *Session) runHookCommand(ctx context.Context, h HookEntry, payload map[string]any) (code int, stdout, stderr string) {
	in, _ := json.Marshal(payload)
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := sandbox.Wrap([]string{FindRealBash(), "-c", h.Command}, s.workRoot(), s.settings.ProtectedPaths)
	cmd := exec.CommandContext(hctx, argv[0], argv[1:]...)
	noConsoleWindow(cmd)
	cmd.Dir = s.workRoot()
	cmd.Env = append(os.Environ(), "INFUSE_PROJECT_DIR="+s.Root, "CLAUDE_PROJECT_DIR="+s.Root)
	cmd.Stdin = bytes.NewReader(in)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Start()
	if err == nil {
		process.AttachJob(cmd)
		err = cmd.Wait()
		process.ReleaseJob(cmd)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return code, out.String(), errb.String()
}

// runHooks runs the approved hooks of an event whose matcher fits target (a tool name; "" for others).
func (s *Session) runHooks(ctx context.Context, event, target string, payload map[string]any) hookOutcome {
	var res hookOutcome
	for _, h := range LoadHooks(s.Root) {
		if h.Event != event || !h.Approved {
			continue
		}
		if h.Matcher != "" && h.Matcher != "*" && target != "" {
			if re, err := regexp.Compile("^(?:" + h.Matcher + ")$"); err != nil || !re.MatchString(target) {
				continue
			}
		}
		guarded, ok := s.guardedHook(h)
		if !ok {
			continue
		}
		h.Command = guarded
		payload["hook_event_name"] = event
		payload["session_id"] = s.ID
		payload["cwd"] = s.workRoot()
		code, stdout, stderr := s.runHookCommand(ctx, h, payload)
		Journal(s.Root, JournalEntry{Kind: "hook", Command: h.Command, Detail: fmt.Sprintf("%s · output %d", event, code)})
		var dec struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		_ = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &dec)
		switch {
		case code == 2:
			res.Block = true
			res.Message = strings.TrimSpace(stderr)
		case dec.Decision == "block":
			res.Block = true
			res.Message = dec.Reason
		case event == "UserPromptSubmit" && code == 0 && strings.TrimSpace(stdout) != "":
			res.Message = strings.TrimSpace(stdout) // context added to the user's message
		}
		if res.Block {
			res.Command = h.Command
			return res
		}
	}
	return res
}
