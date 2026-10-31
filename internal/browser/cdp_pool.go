package browser

import (
	"crypto/sha1"
	"fmt"
	"infuse/internal/core"
	"path/filepath"
	"sync"
	"time"
)

// The per-project agent browser pool and its idle policy.

var (
	cdpBrowsers   = map[string]*Browser{}
	cdpBrowsersMu sync.Mutex
)

// AgentBrowser returns the project's headless browser, starting it when needed.
func AgentBrowser(root string) (*Browser, error) {
	cdpBrowsersMu.Lock()
	defer cdpBrowsersMu.Unlock()
	if b := cdpBrowsers[root]; b != nil {
		b.mu.Lock()
		ok := !b.closed
		b.mu.Unlock()
		if ok {
			return b, nil
		}
	}
	sum := sha1.Sum([]byte(filepath.Clean(root)))
	b, err := startCDP(filepath.Join(core.InfuseHome(), "agent-browser", fmt.Sprintf("%x", sum[:6])))
	if err != nil {
		return nil, err
	}
	cdpBrowsers[root] = b
	go func() { // close after 15 idle minutes
		for {
			time.Sleep(time.Minute)
			b.mu.Lock()
			idle, closed := time.Since(b.lastUse), b.closed
			b.mu.Unlock()
			if closed {
				return
			}
			if idle > 15*time.Minute {
				cdpBrowsersMu.Lock()
				if cdpBrowsers[root] == b {
					delete(cdpBrowsers, root)
				}
				cdpBrowsersMu.Unlock()
				b.close()
				return
			}
		}
	}()
	return b, nil
}

// CloseAgentBrowsers is called when the app exits.
func CloseAgentBrowsers() {
	cdpBrowsersMu.Lock()
	defer cdpBrowsersMu.Unlock()
	for k, b := range cdpBrowsers {
		b.close()
		delete(cdpBrowsers, k)
	}
}
