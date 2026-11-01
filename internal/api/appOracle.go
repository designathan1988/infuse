package api

import (
	"encoding/json"
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/oracle"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// appOracle handles one group of application routes.

func appOracle(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	_ = num
	_ = boolean
	switch {
	case path == "/api/oracle/spec" || path == "/api/oracle/run":
		if strings.TrimSpace(root) == "" { // never resolve criteria against the process's own folder
			sendJSON(w, 400, map[string]any{"error": "the project folder is missing", "code": "project.chooseRoot"})
			return true
		}
	}
	switch {
	case path == "/api/oracle/spec":
		return oracleSpecRoute(w, r, body, root)
	case path == "/api/oracle/run" && r.Method == http.MethodPost:
		return oracleRunRoute(w, r, root, dir, str)
	case path == "/api/shot":
		return shotRoute(w, r, q, root)
	}
	return false
}

// oracleSpecRoute reads the user's criteria, or saves what the user edited.
func oracleSpecRoute(w http.ResponseWriter, r *http.Request, body map[string]any, root string) bool {
	if r.Method == http.MethodPut {
		var spec oracle.OracleSpec
		b, _ := json.Marshal(body["spec"])
		if err := json.Unmarshal(b, &spec); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		_ = os.MkdirAll(filepath.Dir(oracle.SpecFile(root)), 0o755)
		if err := oracle.SaveSpec(root, spec); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		journal.Journal(root, journal.JournalEntry{Kind: "oracle", Path: ".infuse/oracle/checks.json", Detail: "criteria edited by the user"})
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	}
	spec, err := oracle.LoadOracleSpec(root)
	if err != nil {
		sendJSON(w, 200, map[string]any{"error": err.Error(), "spec": oracle.OracleSpec{Checks: []oracle.OracleCheck{}}})
		return true
	}
	if spec.Checks == nil {
		spec.Checks = []oracle.OracleCheck{}
	}
	sendJSON(w, 200, map[string]any{"spec": spec})
	return true
}

// oracleRunRoute is "Ready?": only the user's window calls it (a POST carries the API token); a local
// GET from any process, the agent's own shell included, cannot start the executor.
func oracleRunRoute(w http.ResponseWriter, r *http.Request, root, dir string, str func(string) string) bool {
	session := str("session")
	v, err := oracle.RunOracleTurn(r.Context(), root, dir, session, agent.TurnOf(session))
	if err != nil {
		sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "verdict": v})
		return true
	}
	sendJSON(w, 200, map[string]any{"ok": true, "verdict": v})
	return true
}

// shotRoute serves a screenshot the agent's browser saved in .infuse/shots.
func shotRoute(w http.ResponseWriter, r *http.Request, q map[string]string, root string) bool {
	name := filepath.Base(q["name"])
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		sendJSON(w, 400, map[string]any{"error": "invalid name", "code": "oracle.invalidName"})
		return true
	}
	if ext == ".png" {
		w.Header().Set("Content-Type", "image/png")
	} else {
		w.Header().Set("Content-Type", "image/jpeg")
	}
	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeFile(w, r, filepath.Join(root, core.InfuseDirName, "shots", name))
	return true
}
