package agent

// Package existence is decided from a local cache, never by a network call inside the Bash gate. A
// package missing from the cache is looked up in the background; the next command already sees the
// answer. The cache lives in the user's store, not in the project.

import (
	"context"
	"path/filepath"
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

// refreshPackageAsync asks the registry once, in the background, and stores the answer.
func refreshPackageAsync(p pkgRef) {
	key := pkgKey(p)
	if _, loaded := pkgRefresh.LoadOrStore(key, true); loaded {
		return
	}
	go func() {
		defer pkgRefresh.Delete(key)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if ok, created, err := registryLookup(ctx, p); err == nil {
			rememberPackage(p, ok, created)
		}
	}()
}
