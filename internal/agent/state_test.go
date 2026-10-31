package agent

import "testing"

// Every state the loop can set is declared, and the common paths are allowed.
func TestSessionStateMachineIsDeclared(t *testing.T) {
	for _, st := range States() {
		if _, ok := sessionStates[st]; !ok {
			t.Fatalf("state %q has no transition entry", st)
		}
	}
	for _, pair := range [][2]string{
		{"idle", "thinking"},
		{"thinking", "tool"},
		{"tool", "thinking"},
		{"thinking", "idle"},
		{"tool", "waiting_permission"},
		{"waiting_permission", "thinking"},
		{"thinking", "compacting"},
		{"compacting", "thinking"},
		{"thinking", "error"},
		{"error", "idle"},
	} {
		if !CanTransition(pair[0], pair[1]) {
			t.Errorf("%s → %s should be allowed", pair[0], pair[1])
		}
	}
	if CanTransition("waiting_user", "compacting") {
		t.Error("waiting_user → compacting should not be allowed")
	}
}
