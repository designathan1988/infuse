package api

import "net/http"

// appDevServers handles one group of application routes.

func appDevServers(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- dev servers
	case path == "/api/devservers" && r.Method == http.MethodGet:
		sendJSON(w, 200, DevServers(root))
		return true
	case path == "/api/devservers":
		var err error
		switch str("action") {
		case "approve": // the user approved this exact command in the window
			for _, c := range launchConfigs(root) {
				if c.Name == str("name") {
					err = approveLaunch(root, c)
				}
			}
			if err == nil {
				err = StartDevServer(root, str("name"))
			}
		case "start":
			err = StartDevServer(root, str("name"))
			if na, ok := err.(errLaunchNeedsApproval); ok {
				sendJSON(w, 200, map[string]any{"ok": false, "needsApproval": true, "command": na.Command, "error": na.Error()})
				return true
			}
		case "stop":
			StopDevServer(root, str("name"))
		case "stopall":
			for _, c := range launchConfigs(root) {
				StopDevServer(root, c.Name)
			}
		}
		res := map[string]any{"ok": err == nil, "servers": DevServers(root)}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true
	}
	return false
}
