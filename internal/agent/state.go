package agent

// The session state machine, in one place. The table documents every state a conversation can be in
// and which states may follow; tests keep it aligned with the loop.
var sessionStates = map[string][]string{
	"idle":               {"thinking", "error"},
	"thinking":           {"tool", "idle", "error", "compacting", "waiting_permission", "waiting_user"},
	"tool":               {"thinking", "idle", "error", "compacting", "waiting_permission", "waiting_user"},
	"waiting_permission": {"thinking", "tool", "idle", "error"},
	"waiting_user":       {"thinking", "tool", "idle", "error"},
	"compacting":         {"thinking", "idle", "error"},
	"error":              {"thinking", "idle"},
}

// CanTransition reports whether a session may move from one state to another.
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	for _, next := range sessionStates[from] {
		if next == to {
			return true
		}
	}
	return false
}

// States lists the declared session states.
func States() []string {
	return []string{"idle", "thinking", "tool", "waiting_permission", "waiting_user", "compacting", "error"}
}
