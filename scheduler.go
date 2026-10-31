package main

// Scheduled tasks: a prompt that starts a new session in a folder on a schedule (manual, hourly, daily,
// weekdays, weekly), while the app is open. A run missed while the app was closed is caught up once,
// if it is less than seven days old. Tasks live in ~/.infuse/scheduled/<name>.json.

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type ScheduledTask struct {
	Name      string    `json:"name"`
	Prompt    string    `json:"prompt"`
	Root      string    `json:"root"`
	Preset    string    `json:"preset"` // manual | hourly | daily | weekdays | weekly
	Hour      int       `json:"hour"`
	Minute    int       `json:"minute"`
	Weekday   int       `json:"weekday"` // 0 = Sunday (weekly)
	Mode      string    `json:"mode,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Model     string    `json:"model,omitempty"`
	Worktree  bool      `json:"worktree,omitempty"`
	Active    bool      `json:"active"`
	NextRun   int64     `json:"nextRun"`
	LastRun   int64     `json:"lastRun"`
	History   []TaskRun `json:"history"`
	AlwaysOK  []string  `json:"alwaysAllowed"`
	CreatedAt int64     `json:"createdAt"`
}

type TaskRun struct {
	Ts      int64  `json:"ts"`
	Session string `json:"session,omitempty"`
	Status  string `json:"status"` // started | skipped | failed
	Note    string `json:"note,omitempty"`
}

var (
	reTaskName = regexp.MustCompile(`[^a-z0-9-]+`)
	schedMu    sync.Mutex
)

func scheduledDir() string { return filepath.Join(InfuseHome(), "scheduled") }

func taskSlug(name string) string {
	return strings.Trim(reTaskName.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
}

func ListTasks() []ScheduledTask {
	entries, _ := os.ReadDir(scheduledDir())
	out := []ScheduledTask{}
	for _, e := range entries {
		var t ScheduledTask
		if strings.HasSuffix(e.Name(), ".json") && readJSON(filepath.Join(scheduledDir(), e.Name()), &t) == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func LoadTask(name string) (ScheduledTask, error) {
	var t ScheduledTask
	err := readJSON(filepath.Join(scheduledDir(), taskSlug(name)+".json"), &t)
	return t, err
}

func SaveTask(t ScheduledTask) (ScheduledTask, error) {
	t.Name = taskSlug(t.Name)
	if t.Name == "" {
		return t, errors.New("dê um nome à tarefa")
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return t, errors.New("escreva as instruções da tarefa")
	}
	if st, err := os.Stat(t.Root); err != nil || !st.IsDir() {
		return t, errors.New("escolha a pasta do projeto")
	}
	if t.Preset == "" {
		t.Preset = "daily"
	}
	if t.CreatedAt == 0 {
		t.CreatedAt = time.Now().UnixMilli()
	}
	if t.History == nil {
		t.History = []TaskRun{}
	}
	if t.AlwaysOK == nil {
		t.AlwaysOK = []string{}
	}
	t.NextRun = nextRun(t, time.Now())
	return t, writeJSON(filepath.Join(scheduledDir(), t.Name+".json"), t)
}

func DeleteTask(name string) error { return os.Remove(filepath.Join(scheduledDir(), taskSlug(name)+".json")) }

// nextRun computes the next time the task fires after `from` (0 for manual or paused tasks).
func nextRun(t ScheduledTask, from time.Time) int64 {
	if !t.Active || t.Preset == "manual" {
		return 0
	}
	at := func(d time.Time) time.Time {
		return time.Date(d.Year(), d.Month(), d.Day(), t.Hour, t.Minute, 0, 0, d.Location())
	}
	switch t.Preset {
	case "hourly":
		n := time.Date(from.Year(), from.Month(), from.Day(), from.Hour(), t.Minute, 0, 0, from.Location())
		if !n.After(from) {
			n = n.Add(time.Hour)
		}
		return n.UnixMilli()
	case "daily":
		n := at(from)
		if !n.After(from) {
			n = at(from.AddDate(0, 0, 1))
		}
		return n.UnixMilli()
	case "weekdays":
		for i := 0; i < 8; i++ {
			n := at(from.AddDate(0, 0, i))
			if n.After(from) && n.Weekday() != time.Saturday && n.Weekday() != time.Sunday {
				return n.UnixMilli()
			}
		}
	case "weekly":
		for i := 0; i < 8; i++ {
			n := at(from.AddDate(0, 0, i))
			if n.After(from) && int(n.Weekday()) == t.Weekday {
				return n.UnixMilli()
			}
		}
	}
	return 0
}

// RunTask starts the task's session now and records the run.
func RunTask(name string) (string, error) {
	schedMu.Lock()
	defer schedMu.Unlock()
	t, err := LoadTask(name)
	if err != nil {
		return "", err
	}
	s := newSession(t.Root)
	s.mu.Lock()
	s.Title = "⏱ " + t.Name
	if t.Mode != "" {
		s.Mode = t.Mode
	}
	if t.Provider != "" {
		s.Provider = t.Provider
	}
	if t.Model != "" {
		s.Model = t.Model
	}
	for _, tool := range t.AlwaysOK {
		s.allowAlways[tool] = true
	}
	s.mu.Unlock()
	if t.Worktree {
		if dir, branch, err := CreateWorktree(t.Root, t.Name); err == nil {
			s.mu.Lock()
			s.Worktree, s.Branch, s.Cwd = dir, branch, dir
			s.mu.Unlock()
		}
	}
	run := TaskRun{Ts: time.Now().UnixMilli(), Session: s.ID, Status: "started"}
	if err := s.Send(t.Prompt, nil, nil); err != nil {
		run.Status, run.Note = "failed", err.Error()
	}
	t.LastRun = run.Ts
	t.History = append([]TaskRun{run}, t.History...)
	if len(t.History) > 50 {
		t.History = t.History[:50]
	}
	t.NextRun = nextRun(t, time.Now())
	_ = writeJSON(filepath.Join(scheduledDir(), t.Name+".json"), t)
	return s.ID, nil
}

// StartScheduler checks due tasks every 30 seconds while the app runs.
func StartScheduler() {
	go func() {
		for {
			now := time.Now()
			for _, t := range ListTasks() {
				if !t.Active || t.NextRun == 0 || t.NextRun > now.UnixMilli() {
					continue
				}
				if now.Sub(time.UnixMilli(t.NextRun)) > 7*24*time.Hour { // too old to catch up
					t.History = append([]TaskRun{{Ts: now.UnixMilli(), Status: "skipped", Note: "execução perdida há mais de 7 dias"}}, t.History...)
					t.NextRun = nextRun(t, now)
					_ = writeJSON(filepath.Join(scheduledDir(), t.Name+".json"), t)
					continue
				}
				_, _ = RunTask(t.Name)
			}
			time.Sleep(30 * time.Second)
		}
	}()
}
