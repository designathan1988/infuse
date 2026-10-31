package api

import (
	"encoding/json"
	"net/http"
)

// appTasks handles one group of application routes.

func appTasks(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- scheduled tasks
	case path == "/api/tasks" && r.Method == http.MethodGet:
		sendJSON(w, 200, ListTasks())
		return true
	case path == "/api/tasks" && r.Method == http.MethodPut:
		var t ScheduledTask
		b, _ := json.Marshal(body)
		_ = json.Unmarshal(b, &t)
		if old := str("previousName"); old != "" && taskSlug(old) != taskSlug(t.Name) {
			_ = DeleteTask(old)
		}
		saved, err := SaveTask(t)
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, saved)
		return true
	case path == "/api/tasks/run":
		id, err := RunTask(str("name"))
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		t, _ := LoadTask(str("name"))
		sendJSON(w, 200, map[string]any{"ok": true, "session": id, "root": t.Root})
		return true
	case path == "/api/tasks/delete":
		_ = DeleteTask(str("name"))
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	}
	return false
}
