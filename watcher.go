package main

// Instant write watcher — the always-available half of the filesystem interposition.
// Every write by any tool (native Write/Edit, apply_patch, write_file, or a shell command) is
// journaled within milliseconds and the map is refreshed. Runs as a detached process per project,
// started by the interposed shell on first use, stopping after a period with no writes.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcherState struct {
	Pid     int   `json:"pid"`
	Started int64 `json:"started"`
}

func watcherFile(root string) string { return filepath.Join(root, infuseDirName, "watcher.json") }

func watcherAlive(root string) (int, bool) {
	var st watcherState
	if readJSON(watcherFile(root), &st) != nil || st.Pid == 0 {
		return 0, false
	}
	return st.Pid, processAlive(st.Pid)
}

// EnsureWatcher starts a detached watcher for this project if none is alive.
func EnsureWatcher(root string) {
	if _, alive := watcherAlive(root); alive {
		return
	}
	cmd := exec.Command(selfPath(), "watch", root)
	cmd.Env = append(os.Environ(), "INFUSE_ROOT="+root)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return
	}
	_ = writeJSON(watcherFile(root), watcherState{Pid: cmd.Process.Pid, Started: time.Now().UnixMilli()})
	_ = cmd.Process.Release()
}

func StopWatcher(root string) bool {
	pid, alive := watcherAlive(root)
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

func ignoredPath(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return true
	}
	for _, seg := range strings.Split(toPosix(rel), "/") {
		if ignoredDirs[seg] || (strings.HasPrefix(seg, ".") && seg != "." && seg != "..") {
			return true
		}
	}
	return false
}

// RunWatcher is the body of the detached process.
func RunWatcher(root string, idle time.Duration) {
	_ = writeJSON(watcherFile(root), watcherState{Pid: os.Getpid(), Started: time.Now().UnixMilli()})
	defer os.Remove(watcherFile(root))
	WriteMap(root, BuildMap(root))

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer w.Close()
	addTree := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != root && ignoredPath(root, p) {
					return filepath.SkipDir
				}
				_ = w.Add(p)
			}
			return nil
		})
	}
	addTree(root)

	var refresh *time.Timer
	idleTimer := time.NewTimer(idle)
	last := map[string]int64{} // dedupe Create+Write bursts for the same path
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ignoredPath(root, ev.Name) {
				continue
			}
			rel, _ := filepath.Rel(root, ev.Name)
			rel = toPosix(rel)
			switch {
			case ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write):
				if st, err := os.Stat(ev.Name); err == nil {
					if st.IsDir() {
						if ev.Has(fsnotify.Create) {
							addTree(ev.Name)
						}
						continue
					}
					now := time.Now().UnixMilli()
					if now-last[rel] > 200 {
						Journal(root, JournalEntry{Kind: "write", Path: rel, Bytes: int(st.Size())})
					}
					last[rel] = now
				}
			case ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename):
				Journal(root, JournalEntry{Kind: "unlink", Path: rel})
			default:
				continue
			}
			if refresh != nil {
				refresh.Stop()
			}
			refresh = time.AfterFunc(300*time.Millisecond, func() { WriteMap(root, BuildMap(root)) })
			idleTimer.Reset(idle)
		case <-w.Errors:
		case <-idleTimer.C:
			return
		}
	}
}
