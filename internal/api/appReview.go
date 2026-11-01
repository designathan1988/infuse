package api

import (
	"errors"
	"infuse/internal/workspace"
	"net/http"
)

// appReview handles one group of application routes.

func appReview(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- review
	case path == "/api/changes":
		ch := workspace.Changes(dir)
		if ch["isGit"] == false { // a local folder: counted against the copies infuse kept before each first change
			ch = workspace.LocalChanges(root, dir, r.URL.Query().Get("session"), r.URL.Query().Get("scope"), snapshots{})
		}
		sendJSON(w, 200, ch)
		return true
	case path == "/api/pr" && r.Method == http.MethodGet:
		sendJSON(w, 200, workspace.PRStatus(dir))
		return true
	case path == "/api/pr":
		var out string
		var err error
		switch str("action") {
		case "create":
			out, err = workspace.CreatePR(dir)
		case "merge-auto":
			out, err = workspace.GhRun(dir, "pr", "merge", "--auto", "--squash")
		case "failed-logs": // the failing CI run of this branch, for the agent to fix
			out, err = workspace.FailedCILogs(dir)
		default:
			err = errors.New("unknown action")
		}
		res := map[string]any{"ok": err == nil, "output": out}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true
	}
	return false
}
