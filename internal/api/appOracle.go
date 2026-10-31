package api

import (
	"encoding/json"
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
	switch {
	// ---------------------------------------------------------------- the oracle's executor
	case path == "/api/oracle/spec" && r.Method == http.MethodGet:
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
	case path == "/api/oracle/spec" && r.Method == http.MethodPut:
		var spec oracle.OracleSpec

		b, _ := json.Marshal(body["spec"])
		if err := json.Unmarshal(b, &spec); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		_ = os.MkdirAll(filepath.Dir(oracle.SpecFile(root)), 0o755)
		if err := core.WriteJSON(oracle.SpecFile(root), spec); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		journal.Journal(root, journal.JournalEntry{Kind: "oracle", Path: ".infuse/oracle/checks.json", Detail: "critérios editados pelo usuário"})
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	case path == "/api/oracle/run":
		// "Pronto?": only the user's window calls this (API token); no agent tool can.
		work := dir
		v, err := oracle.RunOracle(r.Context(), root, work, str("session"))
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "verdict": v})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "verdict": v})
		return true
	case path == "/api/shot": // screenshots taken by the agent's browser
		name := filepath.Base(q["name"])
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			sendJSON(w, 400, map[string]any{"error": "nome inválido"})
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
	return false
}
