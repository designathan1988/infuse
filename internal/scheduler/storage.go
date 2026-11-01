// Package scheduler stores the tasks a user schedules (a prompt that starts a session in a folder) and
// computes when each one fires. Executing a task is the caller's job, through its Runner.
package scheduler

import (
	"errors"
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Task struct {
	store.Versioned
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

type Runner interface {
	RunTask(t Task) (session string, err error)
}

var reTaskName = regexp.MustCompile(`[^a-z0-9-]+`)

func Dir() string { return filepath.Join(core.InfuseHome(), "scheduled") }

func Slug(name string) string {
	return strings.Trim(reTaskName.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
}

func List() []Task {
	entries, _ := os.ReadDir(Dir())
	out := []Task{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if t, err := Load(strings.TrimSuffix(e.Name(), ".json")); err == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Load(name string) (Task, error) {
	var t Task
	p := filepath.Join(Dir(), Slug(name)+".json")
	if err := core.ReadJSON(p, &t); err != nil {
		return t, err
	}
	if changed, _ := t.Upgrade(nil); changed {
		_ = core.WriteJSON(p, t)
	}
	return t, nil
}

func Save(t Task) (Task, error) {
	t.Name = Slug(t.Name)
	if t.Name == "" {
		return t, errors.New("give the task a name")
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return t, errors.New("write the task instructions")
	}
	if st, err := os.Stat(t.Root); err != nil || !st.IsDir() {
		return t, errors.New("choose the project folder")
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
	t.NextRun = NextRun(t, time.Now())
	t.SchemaVersion = store.Version
	return t, core.WriteJSON(filepath.Join(Dir(), t.Name+".json"), t)
}

func Delete(name string) error {
	return os.Remove(filepath.Join(Dir(), Slug(name)+".json"))
}

// NextRun computes the next time the task fires after `from` (0 for manual or paused tasks).
func NextRun(t Task, from time.Time) int64 {
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

// Service runs due tasks while the application is open.
type Service struct {
	Runner Runner
	Every  time.Duration
}

func (s *Service) Run(name string) (string, error) {
	t, err := Load(name)
	if err != nil {
		return "", err
	}
	run := TaskRun{Ts: time.Now().UnixMilli(), Status: "started"}
	session, err := s.Runner.RunTask(t)
	if err != nil {
		run.Status, run.Note = "failed", err.Error()
	} else {
		run.Session = session
	}
	t.LastRun = run.Ts
	t.History = append([]TaskRun{run}, t.History...)
	if len(t.History) > 50 {
		t.History = t.History[:50]
	}
	t.NextRun = NextRun(t, time.Now())
	_ = core.WriteJSON(filepath.Join(Dir(), t.Name+".json"), t)
	return session, err
}

func (s *Service) Start() {
	every := s.Every
	if every <= 0 {
		every = 30 * time.Second
	}
	go func() {
		for {
			now := time.Now()
			for _, t := range List() {
				if !t.Active || t.NextRun == 0 || t.NextRun > now.UnixMilli() {
					continue
				}
				if now.Sub(time.UnixMilli(t.NextRun)) > 7*24*time.Hour { // too old to catch up
					t.History = append([]TaskRun{{Ts: now.UnixMilli(), Status: "skipped", Note: "run missed more than 7 days ago"}}, t.History...)
					t.NextRun = NextRun(t, now)
					_ = core.WriteJSON(filepath.Join(Dir(), t.Name+".json"), t)
					continue
				}
				_, _ = s.Run(t.Name)
			}
			time.Sleep(every)
		}
	}()
}
