package api

// Scheduled tasks: the storage and timing live in internal/scheduler; this file is the runner that
// starts a session for a due task.

import (
	"infuse/internal/scheduler"
	"time"
)

type ScheduledTask = scheduler.Task
type TaskRun = scheduler.TaskRun

func ListTasks() []ScheduledTask { return scheduler.List() }

func LoadTask(name string) (ScheduledTask, error) { return scheduler.Load(name) }

func SaveTask(t ScheduledTask) (ScheduledTask, error) { return scheduler.Save(t) }

func DeleteTask(name string) error { return scheduler.Delete(name) }

func nextRun(t ScheduledTask, from time.Time) int64 { return scheduler.NextRun(t, from) }

func taskSlug(name string) string { return scheduler.Slug(name) }

type scheduledRunner struct{}

func (scheduledRunner) RunTask(t scheduler.Task) (string, error) {
	s := newSession(t.Root)
	s.Mu.Lock()
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
	s.Mu.Unlock()
	s.AllowAlways(t.AlwaysOK)
	if t.Worktree {
		if dir, branch, err := CreateWorktree(t.Root, t.Name); err == nil {
			s.Mu.Lock()
			s.Worktree, s.Branch, s.Cwd = dir, branch, dir
			s.Mu.Unlock()
		}
	}
	if err := s.Send(t.Prompt, nil, nil); err != nil {
		return s.ID, err
	}
	return s.ID, nil
}

var taskService = &scheduler.Service{Runner: scheduledRunner{}}

// RunTask starts the task's session now and records the run.
func RunTask(name string) (string, error) { return taskService.Run(name) }

// StartScheduler checks due tasks every 30 seconds while the app runs.
func StartScheduler() { taskService.Start() }
