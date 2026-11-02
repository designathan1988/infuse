package api

// appExtensions: the routes of what extends the agent: the settings' layers, output styles, the status
// line, the turn log, subagent kinds, plugins and the composer's memory note.

import (
	"errors"
	"infuse/internal/agent"
	"infuse/internal/config"
	"infuse/internal/core"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var reSessionStatus = regexp.MustCompile(`^/api/session/([^/]+)/statusline$`)

func appExtensions(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	if m := reSessionStatus.FindStringSubmatch(path); m != nil {
		statusLineRoute(w, r, root, m[1])
		return true
	}
	switch {
	case path == "/api/settings/layers":
		sendJSON(w, 200, maskedLayers(root))
	case path == "/api/settings/local":
		saveLocalSettings(w, body, root)
	case path == "/api/output-styles":
		sendJSON(w, 200, map[string]any{"styles": agent.OutputStyles(root), "current": config.EffectiveSettings(root).OutputStyle})
	case path == "/api/turnlog":
		turnLogRoute(w, q, root)
	case path == "/api/agents/kinds":
		sendJSON(w, 200, agent.SubagentKinds(root))
	case path == "/api/memory/note":
		memoryNoteRoute(w, root, str)
	case path == "/api/plugins":
		sendJSON(w, 200, agent.ListPlugins(root))
	case path == "/api/plugins/install":
		p, err := agent.InstallPlugin(r.Context(), root, str("url"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "plugin": p})
	case path == "/api/plugins/approve":
		err := agent.ApprovePlugin(root, str("name"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "plugins": agent.ListPlugins(root)})
	case path == "/api/plugins/remove":
		err := agent.RemovePlugin(root, str("name"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "plugins": agent.ListPlugins(root)})
	case path == "/api/plugins/marketplace":
		list, err := marketplaceIndex(r, root)
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "plugins": list})
	default:
		return false
	}
	return true
}

func statusLineRoute(w http.ResponseWriter, r *http.Request, root, id string) {
	s, err := agent.LoadSession(root, id)
	if err != nil {
		sendJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	sendJSON(w, 200, map[string]any{"text": s.StatusLine(r.Context())})
}

func turnLogRoute(w http.ResponseWriter, q map[string]string, root string) {
	turn, _ := strconv.Atoi(q["turn"])
	limit, _ := strconv.Atoi(q["limit"])
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	sendJSON(w, 200, agent.QueryTurnLog(root, agent.TurnQuery{Session: q["session"], Turn: turn, Kind: q["kind"], Tool: q["tool"], Limit: limit}))
}

// memoryNoteRoute is the composer's "#" shortcut: one note, in the project's memory or the global one.
func memoryNoteRoute(w http.ResponseWriter, root string, str func(string) string) {
	file := agent.ProjectMemoryFile(root)
	if str("scope") == "global" {
		file = agent.GlobalMemoryFile()
	}
	it := agent.MemoryItem{Text: str("text"), Origin: "usuario"}
	err := agent.NormalizeMemory(&it)
	if err == nil {
		err = agent.SaveMemory(file, append(agent.LoadMemory(file), it))
	}
	sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
}

const marketplaceBytes = 4 << 20

// marketplaceIndex fetches the JSON index the settings name.
func marketplaceIndex(r *http.Request, root string) ([]agent.MarketEntry, error) {
	u := config.EffectiveSettings(root).MarketplaceURL
	if u == "" {
		return nil, userMsgError(root, "plugin.no_marketplace")
	}
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://127.0.0.1") {
		return nil, errors.New("the marketplace address must be https")
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("the marketplace answered " + resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, marketplaceBytes))
	if err != nil {
		return nil, err
	}
	return agent.ParseMarketIndex(b)
}

func userMsgError(root, id string) error {
	return errors.New(core.M(id).Render(config.RootLanguage(root)))
}
