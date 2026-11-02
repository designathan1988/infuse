package agent

// The Rules page shows every prohibition the user set in this project, across sessions, with the
// words they used. The owner of a ban is still the session that recorded it; this is the read-only
// view over all of them.

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SessionBans is one session's prohibitions, as the policy view reads them.
type SessionBans struct {
	Session string `json:"session"`
	Title   string `json:"title,omitempty"`
	Bans    []Ban  `json:"bans"`
}

// ProjectBans reads every session's ban file of a project, newest session first.
func ProjectBans(root string) []SessionBans {
	entries, err := os.ReadDir(sessionsDir(root))
	if err != nil {
		return []SessionBans{}
	}
	out := []SessionBans{} // the page reads a list, also when a project has sessions and no prohibition
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".bans.json")
		if !ok {
			continue
		}
		var list []Ban
		if core.ReadJSON(filepath.Join(sessionsDir(root), e.Name()), &list) != nil || len(list) == 0 {
			continue
		}
		out = append(out, SessionBans{Session: id, Title: sessionTitle(root, id), Bans: list})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Session > out[j].Session })
	return out
}

func sessionTitle(root, id string) string {
	var sd struct {
		Title string `json:"title"`
	}
	if core.ReadJSON(filepath.Join(sessionsDir(root), id+".json"), &sd) != nil {
		return ""
	}
	return sd.Title
}
