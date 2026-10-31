package api

import "net/http"

// studioGroups is the ordered table of route groups the studio API dispatches to.
type studioGroup func(http.ResponseWriter, *http.Request, map[string]string, map[string]any, string, func(string) string) bool

var studioGroups = []studioGroup{
	studioRoutes1,
	studioRoutes2,
}
