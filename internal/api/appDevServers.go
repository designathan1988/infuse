package api

import (
	"infuse/internal/workspace"
	"net/http"
)

// appDevServers handles one group of application routes.

func appDevServers(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- dev servers
	case path == "/api/devservers" && r.Method == http.MethodGet:
		sendJSON(w, 200, workspace.DevServers(root))
		return true
	case path == "/api/devservers":
		var err error
		switch str("action") {
		case "approve": // the user approved this exact command in the window
			for _, c := range workspace.LaunchConfigs(root) {
				if c.Name == str("name") {
					err = workspace.ApproveLaunch(root, c)
				}
			}
			if err == nil {
				err = workspace.StartDevServer(root, str("name"))
			}
		case "start":
			err = workspace.StartDevServer(root, str("name"))
			if na, ok := err.(workspace.ErrLaunchNeedsApproval); ok {
				sendJSON(w, 200, map[string]any{"ok": false, "needsApproval": true, "command": na.Command, "error": na.Error()})
				return true
			}
		case "stop":
			workspace.StopDevServer(root, str("name"))
		case "stopall":
			for _, c := range workspace.LaunchConfigs(root) {
				workspace.StopDevServer(root, c.Name)
			}
		}
		res := map[string]any{"ok": err == nil, "servers": workspace.DevServers(root)}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true
	}
	return false
}
