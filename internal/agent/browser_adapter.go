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

// ProjectFile resolves a path the page will receive: the read gate applies, and the real file must be
// inside the project, because what is uploaded leaves the machine.
func (tc *ToolContext) ProjectFile(p string) (string, error) {
	abs, err := tc.resolvePath(p, false)
	if err != nil {
		return "", err
	}
	if !within(tc.Root, abs) || !within(realPath(tc.Root), realPath(abs)) {
		return "", errMsg(M("path.outside", "path", p))
	}
	return abs, nil
}

func (tc *ToolContext) AttachImage(b Block) {
	tc.S.Mu.Lock()
	tc.S.pendingImages = append(tc.S.pendingImages, b)
	tc.S.Mu.Unlock()
}

func (tc *ToolContext) Clip(s string) string { return tc.clip(s) }

// NoteScreenshot records where the capture looked and what it showed, so a UI change is not
// "verified" by a screenshot of some other page (or of another state of the same page).
func (tc *ToolContext) NoteScreenshot(url, html string) { tc.S.noteScreenshotAt(url, html) }

// ShowURL makes the page the agent is using visible in the user's Browser pane.
func (tc *ToolContext) ShowURL(u string) { tc.S.showURL(u) }

// showURL tells the UI which page to show in the Browser pane. The pane only follows the session on
// screen, so an agent working in the background never takes over the user's view.
func (s *Session) showURL(u string) { s.emit("browser", map[string]string{"url": u}) }

type browserToolAdapter struct{}

func (browserToolAdapter) Category() string { return browser.Tool{}.Category() }
func (browserToolAdapter) Spec() ToolSpec   { return browser.Tool{}.Spec() }
func (browserToolAdapter) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	return browser.Tool{}.Run(ctx, tc, input)
}

func closeAgentBrowsers() { browser.CloseAgentBrowsers() }
