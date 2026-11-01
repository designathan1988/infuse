package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// A sidecar such as <id>.bans.json must never appear as a session, even when its JSON decodes
// (a "null" document unmarshals into a zero SessionData).
func TestListSessionsIgnoresSidecarsAndEmptyIDs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".infuse", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("101.json", `{"id":"101","title":"real"}`)
	write("101.bans.json", "null\n")
	write("null.json", "null\n")

	got := ListSessions(root)
	if len(got) != 1 || got[0].ID != "101" {
		t.Fatalf("ListSessions = %+v, want only session 101", got)
	}
}
