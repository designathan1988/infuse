package agent

// Bridges the browser domain to the root tool registry and to the session it runs for.

import (
	"context"
	"encoding/json"
	"infuse/internal/browser"
	"infuse/internal/config"
)

// Host methods on ToolContext: the browser tool sees only what it needs from the session.
func (tc *ToolContext) RootDir() string { return tc.Root }

func (tc *ToolContext) Settings() config.Settings { return tc.S.settings }

func (tc *ToolContext) AttachImage(b Block) {
	tc.S.Mu.Lock()
	tc.S.pendingImages = append(tc.S.pendingImages, b)
	tc.S.Mu.Unlock()
}

func (tc *ToolContext) Clip(s string) string { return tc.clip(s) }

type browserToolAdapter struct{}

func (browserToolAdapter) Category() string { return browser.Tool{}.Category() }
func (browserToolAdapter) Spec() ToolSpec   { return browser.Tool{}.Spec() }
func (browserToolAdapter) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	return browser.Tool{}.Run(ctx, tc, input)
}

func init() { registerTool(browserToolAdapter{}) }

func closeAgentBrowsers() { browser.CloseAgentBrowsers() }
