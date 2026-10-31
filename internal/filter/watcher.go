package filter

// Instant write watcher — the always-available half of the filesystem interposition.
// Every write by any tool (native Write/Edit, apply_patch, write_file, or a shell command) is
// journaled within milliseconds and the map is refreshed. Runs as a detached process per project,
// started by the interposed shell on first use, stopping after a period with no writes.

import (
	"infuse/internal/core"
	"infuse/internal/fsx"
	"infuse/internal/journal"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcherState struct {
	Pid     int   `json:"pid"`
	Started int64 `json:"started"`
}

func watcherFile(root string) string { return filepath.Join(root, core.InfuseDirName, "watcher.json") }

func WatcherAlive(root string) (int, bool) {
	var st watcherState
	if core.ReadJSON(watcherFile(root), &st) != nil || st.Pid == 0 {
		return 0, false
	}
	return st.Pid, process.ProcessAlive(st.Pid)
}

// EnsureWatcher starts a detached watcher for this project if none is alive.
func EnsureWatcher(root string) {
	if _, alive := WatcherAlive(root); alive {
		return
	}
	cmd := exec.Command(core.SelfPath(), "watch", root)
	cmd.Env = append(os.Environ(), "INFUSE_ROOT="+root)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	process.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return
	}
	_ = core.WriteJSON(watcherFile(root), watcherState{Pid: cmd.Process.Pid, Started: time.Now().UnixMilli()})
	_ = cmd.Process.Release()
}

func StopWatcher(root string) bool {
	pid, alive := WatcherAlive(root)
	if !alive {
		_ = os.Remove(watcherFile(root))
		return false
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
		if err := p.Kill(); err == nil || true {
			_ = os.Remove(watcherFile(root))
		}
	}
	return true
}

func IgnoredPath(root, p string) bool { return fsx.IgnoredPath(root, p) }

type writeWatcher struct {
	root    string
	w       *fsnotify.Watcher
	refresh *time.Timer
	last    map[string]int64 // dedupe Create+Write bursts for the same path
}

// RunWatcher is the body of the detached process.
func RunWatcher(root string, idle time.Duration) {
	_ = core.WriteJSON(watcherFile(root), watcherState{Pid: os.Getpid(), Started: time.Now().UnixMilli()})
	defer os.Remove(watcherFile(root))
	projectmap.WriteMap(root, projectmap.BuildMap(root))

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer w.Close()
	wt := &writeWatcher{root: root, w: w, last: map[string]int64{}}
	wt.addTree(root)

	idleTimer := time.NewTimer(idle)
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if !wt.handleEvent(ev) {
				continue
			}
			wt.scheduleRefresh()
			idleTimer.Reset(idle)
		case <-w.Errors:
		case <-idleTimer.C:
			return
		}
	}
}

// addTree watches every project directory under dir.
func (wt *writeWatcher) addTree(dir string) {
	_ = fsx.Walk(dir, fsx.Project, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = wt.w.Add(p)
		}
		return nil
	})
}

// handleEvent journals an event; it reports whether the map should be refreshed afterwards.
func (wt *writeWatcher) handleEvent(ev fsnotify.Event) bool {
	if IgnoredPath(wt.root, ev.Name) {
		return false
	}
	rel, _ := filepath.Rel(wt.root, ev.Name)
	rel = core.ToPosix(rel)
	switch {
	case ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write):
		return wt.recordWrite(ev, rel)
	case ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename):
		journal.Journal(wt.root, journal.JournalEntry{Kind: "unlink", Path: rel})
		return true
	}
	return false
}

// recordWrite journals a file write (a created directory is added to the tree instead); it reports
// whether to refresh.
func (wt *writeWatcher) recordWrite(ev fsnotify.Event, rel string) bool {
	st, err := os.Stat(ev.Name)
	if err != nil {
		return true
	}
	if st.IsDir() {
		if ev.Has(fsnotify.Create) {
			wt.addTree(ev.Name)
		}
		return false
	}
	now := time.Now().UnixMilli()
	if now-wt.last[rel] > 200 {
		journal.Journal(wt.root, journal.JournalEntry{Kind: "write", Path: rel, Bytes: int(st.Size())})
	}
	wt.last[rel] = now
	return true
}

// scheduleRefresh rebuilds the map shortly after the last event.
func (wt *writeWatcher) scheduleRefresh() {
	if wt.refresh != nil {
		wt.refresh.Stop()
	}
	wt.refresh = time.AfterFunc(300*time.Millisecond, func() { projectmap.WriteMap(wt.root, projectmap.BuildMap(wt.root)) })
}
