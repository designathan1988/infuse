package api

import (
	"infuse/internal/core"
	"net/http"
)

// appLocale serves the message catalog: every Go message and every interface string, in both
// languages, with stable ids. The UI renders from the same data the server uses.
func appLocale(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	if r.URL.Path == "/api/i18n" && r.Method == http.MethodGet {
		sendJSON(w, 200, core.Catalog())
		return true
	}
	return false
}
