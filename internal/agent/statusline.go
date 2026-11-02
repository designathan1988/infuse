package agent

// The status line: a template over the session's fields or a command of the user's settings, given the
// same fields as JSON on stdin. It lives in the user's and the machine's layers only: a project file
// cannot choose a command to run.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/filter"
	"infuse/internal/sandbox"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultStatusTemplate = "{model} · {mode} · {branch}"
	statusCommandTimeout  = 3 * time.Second
)

// statusFields are the values a status line can show.
func (s *Session) statusFields() map[string]string {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return map[string]string{
		"model": s.Model, "mode": s.mode(), "branch": s.Branch, "cwd": s.Cwd, "session": s.ID, "title": s.Title,
		"cost": fmt.Sprintf("%.2f", s.Cost), "tokens": fmt.Sprint(s.Usage.Input + s.Usage.Output), "context": fmt.Sprint(s.LastInput),
	}
}

// StatusLine renders the line; a failing command falls back to the template.
func (s *Session) StatusLine(ctx context.Context) string {
	fields := s.statusFields()
	template := defaultStatusTemplate
	if c := s.settings.StatusLine; c != nil {
		if c.Command != "" {
			if line, ok := s.statusCommand(ctx, c.Command, fields); ok {
				return line
			}
		}
		template = firstNonEmpty(c.Template, template)
	}
	return expandStatus(template, fields)
}

func expandStatus(template string, fields map[string]string) string {
	for k, v := range fields {
		template = strings.ReplaceAll(template, "{"+k+"}", v)
	}
	return template
}

func (s *Session) statusCommand(ctx context.Context, command string, fields map[string]string) (string, bool) {
	guarded, _, err := newGuard(s.Root, s.settings.ProtectedPaths).GuardCommand(command, s.workRoot())
	if err != nil {
		return "", false
	}
	cctx, cancel := context.WithTimeout(ctx, statusCommandTimeout)
	defer cancel()
	argv := sandbox.Wrap([]string{FindRealBash(), "-c", guarded}, s.workRoot(), filter.ContainedRecords(s.settings.ProtectedPaths), s.settings.ContainProcesses)
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	noConsoleWindow(cmd)
	cmd.Dir = s.workRoot()
	cmd.Env = append(os.Environ(), s.settingsEnv()...)
	in, _ := json.Marshal(fields)
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return s.maskSettingsEnv(firstLine(strings.TrimSpace(string(out)))), true
}
