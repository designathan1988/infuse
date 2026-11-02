package agent

// Package existence is decided from a local cache, never by a network call inside the Bash gate. A
// package missing from the cache is looked up in the background; the next command already sees the
// answer. The cache lives in the user's store, not in the project.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"infuse/internal/core"
)

type pkgCacheEntry struct {
	Exists  bool  `json:"exists"`
	Created int64 `json:"created,omitempty"`
	At      int64 `json:"at"`
}

var (
	pkgCacheMu   sync.Mutex
	pkgCache     map[string]pkgCacheEntry
	pkgCacheFrom string
	pkgRefresh   sync.Map // key → true while a lookup is in flight
)

func packageCacheFile() string { return filepath.Join(InfuseHome(), "packages.json") }

func pkgKey(p pkgRef) string { return p.registry + "/" + p.name }

func loadPackageCache() {
	f := packageCacheFile()
	if pkgCache != nil && pkgCacheFrom == f {
		return
	}
	pkgCache = map[string]pkgCacheEntry{}
	_ = core.ReadJSON(f, &pkgCache)
	pkgCacheFrom = f
}

// cachedPackage returns the last known answer for a package.
func cachedPackage(p pkgRef) (pkgCacheEntry, bool) {
	pkgCacheMu.Lock()
	defer pkgCacheMu.Unlock()
	loadPackageCache()
	e, ok := pkgCache[pkgKey(p)]
	return e, ok
}

// rememberPackage stores an answer and persists it for the next runs.
func rememberPackage(p pkgRef, exists bool, created time.Time) {
	pkgCacheMu.Lock()
	defer pkgCacheMu.Unlock()
	loadPackageCache()
	e := pkgCacheEntry{Exists: exists, At: time.Now().UnixMilli()}
	if !created.IsZero() {
		e.Created = created.UnixMilli()
	}
	pkgCache[pkgKey(p)] = e
	_ = core.WriteJSON(packageCacheFile(), pkgCache)
}

var reInstaller = regexp.MustCompile(`(?i)\b(?:npm|pnpm|yarn)\s+(?:i|install|ci|add)\b|\bpip3?\s+install\b|\bgo\s+get\b|\bcargo\s+add\b|\bpoetry\s+add\b|\bbundle\s+install\b`)

// installerCommand reports a command that installs dependencies: its success rebases the turn's
// installed-tree fingerprint, so the change it made is not treated as a tampering after the fact.
func installerCommand(cmd string) bool {
	if len(packagesInCommand(cmd)) > 0 {
		return true
	}
	if strings.Contains(cmd, "--package-lock-only") || strings.Contains(cmd, "--dry-run") {
		return false
	}
	return reInstaller.MatchString(cmd)
}

// newManifestPackages lists npm dependencies the edit added to package.json. A name that did not
// exist before is checked against the registry before the file is accepted, so "npm install" cannot
// run a name the agent invented.
func newManifestPackages(before, after string) []pkgRef {
	read := func(s string) map[string]bool {
		out := map[string]bool{}
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(s), &m) != nil {
			return out
		}
		for _, key := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
			var deps map[string]string
			if raw, ok := m[key]; ok && json.Unmarshal(raw, &deps) == nil {
				for name := range deps {
					out[name] = true
				}
			}
		}
		return out
	}
	was := read(before)
	var refs []pkgRef
	for name := range read(after) {
		if !was[name] {
			refs = append(refs, pkgRef{name: name, registry: "npm"})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].name < refs[j].name })
	return refs
}

// verifyPackageRefs is the registry check shared by the command gate and the manifest gate.
func verifyPackageRefs(ctx context.Context, refs []pkgRef) Msg {
	var missing, fresh, unverified []string
	for _, p := range refs {
		e, ok := cachedPackage(p)
		if !ok {
			refreshPackageAsync(p)
			e, ok = waitForPackage(ctx, p, packageWait)
			if !ok {
				unverified = append(unverified, fmt.Sprintf("%s (%s)", p.name, p.registry))
				continue
			}
		}
		created := time.UnixMilli(e.Created)
		if !e.Exists {
			missing = append(missing, fmt.Sprintf("%s (%s)", p.name, p.registry))
		} else if e.Created > 0 && time.Since(created) < 30*24*time.Hour {
			fresh = append(fresh, fmt.Sprintf("%s (%s, %s)", p.name, p.registry, created.Format("2006-01-02")))
		}
	}
	switch {
	case len(missing) > 0:
		return M("pkg.missing", "packages", strings.Join(missing, ", "))
	case len(fresh) > 0:
		return M("pkg.fresh", "packages", strings.Join(fresh, ", "))
	case len(unverified) > 0:
		return M("pkg.unverified", "packages", strings.Join(unverified, ", "))
	}
	return Msg{}
}

// packageWait bounds how long a package gate waits for the registry before refusing an unverified
// name. A variable so tests do not spend the real deadline.
var packageWait = 3 * time.Second

// waitForPackage waits, bounded, for the lookup started for p.
func waitForPackage(ctx context.Context, p pkgRef, d time.Duration) (pkgCacheEntry, bool) {
	deadline := time.Now().Add(d)
	for {
		if e, ok := cachedPackage(p); ok {
			return e, true
		}
		if time.Now().After(deadline) {
			return pkgCacheEntry{}, false
		}
		select {
		case <-ctx.Done():
			return pkgCacheEntry{}, false
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// refreshPackageAsync asks the registry once, in the background, and stores the answer.
func refreshPackageAsync(p pkgRef) {
	key := pkgKey(p)
	if _, loaded := pkgRefresh.LoadOrStore(key, true); loaded {
		return
	}
	lookup := registryLookup // captured before the goroutine: no read races with a test or a swap
	go func() {
		defer pkgRefresh.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if ok, created, err := lookup(ctx, p); err == nil {
			rememberPackage(p, ok, created)
		}
	}()
}
