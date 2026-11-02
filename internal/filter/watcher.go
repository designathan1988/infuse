package filter

// Instant write watcher — the always-available half of the filesystem interposition.
// Every write by any tool (native Write/Edit, apply_patch, write_file, or a shell command) is
// journaled within milliseconds and the map is updated for the files touched; a full build follows
// when the writes stop, for the levels that read every file. Runs as a detached process per project,
// started by the interposed shell or the launcher on first use, stopping after a period with no writes.

import (
	"infuse/internal/core"
	"infuse/internal/fsx"
	"infuse/internal/journal"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcherState struct {
	Pid     int   `json:"pid"`
	Started int64 `json:"started"`
}

func watcherFile(root string) string { return filepath.Join(root, core.InfuseDirName, "watcher.json") }

// watcherRoleEnv marks the process started as the watcher. The shell shim is this same binary named
// bash, so its first argument cannot say it: `bash watch x` is a script named watch.
const watcherRoleEnv = "INFUSE_WATCHER_ROLE"

// watcherRole reports whether this process is a watcher started by EnsureWatcher, and for which root.
func watcherRole(args []string) (string, bool) {
	if os.Getenv(watcherRoleEnv) == "" || len(args) != 2 || args[0] != "watch" {
		return "", false
	}
	return args[1], true
}

// WatcherIdle is how long a watcher waits for a write before it stops.
func WatcherIdle() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("INFUSE_WATCHER_IDLE_MIN")); err == nil && v > 0 {
		return time.Duration(v) * time.Minute
	}
	return 45 * time.Minute
}

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
	spawnWatcher(root)
}

// spawnWatcher starts the detached process; it returns at once, so the shell never waits for a build.
var spawnWatcher = func(root string) {
	cmd := exec.Command(core.SelfPath(), "watch", root)
	cmd.Env = append(os.Environ(), "INFUSE_ROOT="+root, watcherRoleEnv+"=1")
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

// Timing of the map's refresh after an event: the touched files soon, everything once writes stop.
const (
	refreshDelay  = 300 * time.Millisecond
	fullDelay     = 20 * time.Second
	maxIncludable = 64 // past this many files in one burst, a full build is the cheaper reading
)

type writeWatcher struct {
	root    string
	w       *fsnotify.Watcher
	mu      sync.Mutex
	pending map[string]bool // files touched since the last refresh
	refresh *time.Timer
	full    *time.Timer
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
	wt := &writeWatcher{root: root, w: w, last: map[string]int64{}, pending: map[string]bool{}}
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
		wt.touch(rel)
		return true
	}
	return false
}

// recordWrite journals a file write (a created directory is added to the tree instead); it reports
// whether to refresh.
func (wt *writeWatcher) recordWrite(ev fsnotify.Event, rel string) bool {
	st, err := os.Stat(ev.Name)
	if err != nil {
		wt.touch(rel)
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
	wt.touch(rel)
	return true
}

func (wt *writeWatcher) touch(rel string) {
	wt.mu.Lock()
	wt.pending[rel] = true
	wt.mu.Unlock()
}

// scheduleRefresh updates the map shortly after the last event, and rebuilds it when events stop.
func (wt *writeWatcher) scheduleRefresh() {
	if wt.refresh != nil {
		wt.refresh.Stop()
	}
	if wt.full != nil {
		wt.full.Stop()
	}
	wt.refresh = time.AfterFunc(refreshDelay, wt.flush)
	wt.full = time.AfterFunc(fullDelay, wt.rebuild)
}

// flush applies the touched files to the map.
func (wt *writeWatcher) flush() {
	wt.mu.Lock()
	rels := make([]string, 0, len(wt.pending))
	for r := range wt.pending {
		rels = append(rels, r)
	}
	wt.pending = map[string]bool{}
	wt.mu.Unlock()
	if len(rels) > maxIncludable {
		wt.rebuild()
		return
	}
	projectmap.UpdateFiles(wt.root, rels)
}

func (wt *writeWatcher) rebuild() { projectmap.WriteMap(wt.root, projectmap.BuildMap(wt.root)) }
