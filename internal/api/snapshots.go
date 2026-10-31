package api

import (
	"infuse/internal/agent"
	"infuse/internal/workspace"
)

// snapshots adapts the agent's live sessions to the workspace snapshot port.
type snapshots struct{}

func (snapshots) LiveSessions(root string) []workspace.LiveSession {
	return agent.LiveSessions(root)
}
