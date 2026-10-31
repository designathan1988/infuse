package api

import "net/http"

// appGroups is the ordered table of route groups the app API dispatches to.
type appGroup func(http.ResponseWriter, *http.Request, map[string]string, map[string]any, string, string, func(string) string, func(string) int, func(string) bool) bool

var appGroups = []appGroup{
	appProviders,
	appOracle,
	appDocs,
	appHooks,
	appMCP,
	appMemory,
	appSessions,
	appReview,
	appFiles,
	appTerminal,
	appDevServers,
	appTasks,
	appCommands,
}
