package main

// Background processes: a command the agent starts with run_in_background keeps running (a dev server,
// a game, a watcher) while the agent goes on working. infuse keeps each one's output, tells the agent
// when one ends, lets it read new output (BashOutput) and stop it (KillShell), and shows every process
// of the session, live, in the Processes pane. Stopping ends the whole process tree; closing infuse ends
// them all.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type bgProc struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Started     int64  `json:"started"`
	Ended       int64  `json:"ended,omitempty"`
	Exit        *int   `json:"exit,omitempty"`
	State       string `json:"state"` // running | exited | stopped
	PID         int    `json:"pid"`

	out  *procLog
	cmd  *exec.Cmd
	read int64 // output already handed to the agent
	told bool  // the agent was told it ended
}

// procLog keeps the last 2 MB of a process's output, addressed by absolute byte offsets.
type procLog struct {
	mu    sync.Mutex
	buf   []byte
	total int64
}

const procLogMax = 2 << 20

func (l *procLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.buf = append(l.buf, p...)
	if len(l.buf) > procLogMax {
		l.buf = append([]byte{}, l.buf[len(l.buf)-procLogMax:]...)
	}
	l.total += int64(len(p))
	l.mu.Unlock()
	return len(p), nil
}

// since returns the output after offset (what was dropped is announced) and the new offset.
func (l *procLog) since(off int64) (string, int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	first := l.total - int64(len(l.buf))
	prefix := ""
	if off < first {
		prefix = fmt.Sprintf("[… %d bytes mais antigos descartados …]\n", first-off)
		off = first
	}
	return prefix + string(l.buf[off-first:]), l.total
}

var allProcs sync.Map // every running process of every session, for shutdown

// startBackground runs command in the session's shell and returns at once.
func (s *Session) startBackground(command, description, cwd string) (*bgProc, error) {
	cmd := exec.Command(FindRealBash(), "-c", command)
	noConsoleWindow(cmd)
	procGroup(cmd)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "INFUSE_AGENT=1", "GIT_TERMINAL_PROMPT=0", "CI=1")
	p := &bgProc{Command: command, Description: description, Started: time.Now().UnixMilli(), State: "running", out: &procLog{}, cmd: cmd}
	cmd.Stdout, cmd.Stderr = p.out, p.out
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	attachJob(cmd)
	p.PID = cmd.Process.Pid
	s.mu.Lock()
	s.procSeq++
	p.ID = fmt.Sprintf("p%d", s.procSeq)
	s.procs = append(s.procs, p)
	s.mu.Unlock()
	allProcs.Store(p, true)
	Journal(s.Root, JournalEntry{Kind: "command", Command: command, Detail: "em segundo plano (" + p.ID + ")"})
	s.emitProcs()
	go func() {
		err := cmd.Wait()
		allProcs.Delete(p)
		s.mu.Lock()
		stopped := p.State == "stopped"
		s.mu.Unlock()
		if !stopped {
			releaseJob(cmd)
		}
		code := exitCode(err)
		s.mu.Lock()
		p.Ended, p.Exit = time.Now().UnixMilli(), &code
		if p.State == "running" {
			p.State = "exited"
		}
		s.mu.Unlock()
		s.emitProcs()
	}()
	return p, nil
}

func (s *Session) proc(id string) *bgProc {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.procs {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// stopProc ends a process and everything it started.
func (s *Session) stopProc(id string) error {
	p := s.proc(id)
	if p == nil {
		return fmt.Errorf("processo %s não existe nesta sessão", id)
	}
	s.mu.Lock()
	running := p.State == "running"
	if running {
		p.State = "stopped"
	}
	s.mu.Unlock()
	if running {
		killTree(p.cmd)
	}
	s.emitProcs()
	return nil
}

// procList is the session's processes for the interface.
func (s *Session) procList() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.procListLocked()
}

func (s *Session) procListLocked() []map[string]any {
	out := []map[string]any{}
	for _, p := range s.procs {
		tail, _ := p.out.since(p.out.total - 4000)
		out = append(out, map[string]any{"id": p.ID, "command": p.Command, "description": p.Description, "started": p.Started, "ended": p.Ended, "exit": p.Exit, "state": p.State, "pid": p.PID, "bytes": p.out.total, "tail": tail})
	}
	return out
}

func (s *Session) emitProcs() { s.emit("procs", s.procList()) }

// procNotes tells the agent, once, about processes that ended since it last looked.
func (s *Session) procNotes() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var notes []string
	for _, p := range s.procs {
		if p.State != "running" && !p.told {
			p.told = true
			code := 0
			if p.Exit != nil {
				code = *p.Exit
			}
			if p.State == "stopped" {
				notes = append(notes, fmt.Sprintf("[infuse] o processo em segundo plano %s (%s) foi encerrado.", p.ID, truncate(p.Command, 80)))
			} else {
				notes = append(notes, fmt.Sprintf("[infuse] o processo em segundo plano %s (%s) terminou sozinho, código %d. Leia a saída com BashOutput.", p.ID, truncate(p.Command, 80), code))
			}
		}
	}
	return strings.Join(notes, "\n")
}

// stopAllProcs ends every background process (infuse is closing).
func stopAllProcs() {
	allProcs.Range(func(k, _ any) bool {
		killTree(k.(*bgProc).cmd)
		return true
	})
}

// ---------------------------------------------------------------- tools

type bashOutputTool struct{}

func (bashOutputTool) Category() string { return "read" }
func (bashOutputTool) Spec() ToolSpec {
	return ToolSpec{Name: "BashOutput", Description: "Lê a saída nova de um processo em segundo plano (iniciado com Bash run_in_background), e diz se ainda está rodando. filter: regex para mostrar só as linhas que interessam.",
		InputSchema: schema(`{"type":"object","properties":{"id":{"type":"string"},"filter":{"type":"string"}},"required":["id"]}`)}
}

func (bashOutputTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ ID, Filter string }
	_ = json.Unmarshal(input, &in)
	p := tc.S.proc(in.ID)
	if p == nil {
		return "processo não encontrado: " + in.ID + ". Processos desta sessão: " + tc.S.procIDs(), true
	}
	tc.S.mu.Lock()
	from := p.read
	tc.S.mu.Unlock()
	text, to := p.out.since(from)
	tc.S.mu.Lock()
	p.read = to
	state, exit := p.State, p.Exit
	if state != "running" {
		p.told = true
	}
	tc.S.mu.Unlock()
	if in.Filter != "" {
		re, err := regexp.Compile(in.Filter)
		if err != nil {
			return "filter inválido: " + err.Error(), true
		}
		var keep []string
		for _, l := range strings.Split(text, "\n") {
			if re.MatchString(l) {
				keep = append(keep, l)
			}
		}
		text = strings.Join(keep, "\n")
	}
	head := "rodando"
	if state != "running" {
		head = state
		if exit != nil {
			head += fmt.Sprintf(" (código %d)", *exit)
		}
	}
	if strings.TrimSpace(text) == "" {
		text = "(nenhuma saída nova)"
	}
	return fmt.Sprintf("[%s · %s]\n%s", in.ID, head, clip(text)), false
}

type killShellTool struct{}

func (killShellTool) Category() string { return "exec" }
func (killShellTool) Spec() ToolSpec {
	return ToolSpec{Name: "KillShell", Description: "Encerra um processo em segundo plano e tudo o que ele iniciou.",
		InputSchema: schema(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`)}
}

func (killShellTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ ID string }
	_ = json.Unmarshal(input, &in)
	if err := tc.S.stopProc(in.ID); err != nil {
		return err.Error(), true
	}
	return "processo " + in.ID + " encerrado", false
}

func (s *Session) procIDs() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for _, p := range s.procs {
		ids = append(ids, p.ID+" ("+p.State+")")
	}
	if len(ids) == 0 {
		return "nenhum"
	}
	return strings.Join(ids, ", ")
}

func init() {
	registerTool(bashOutputTool{})
	registerTool(killShellTool{})
}
