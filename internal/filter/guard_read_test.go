package filter

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func TestShellRefusesReadersOfHiddenRecords(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	sessions := filepath.Join(root, ".infuse", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, "s1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newGuard(root, nil)
	refused(t, g, root,
		"cat .infuse/sessions/s1.json",
		"cat .infuse/sessions/*",
		"cat .inf*/sess*/*",
		"head -5 .infuse/journal.jsonl",
		"tail -n 1 .infuse/lessons.jsonl",
		"grep -r secret .infuse",
		"grep -e x .infuse/journal.jsonl",
		"tar czf a.tgz .infuse",
		"cp .infuse/lessons.jsonl out.txt",
		"cat < .infuse/journal.jsonl",
		"while read l; do echo $l; done < .infuse/journal.jsonl",
		"ls .infuse/sessions | xargs cat",
		"find .infuse/sessions -type f -exec cat {} +",
		"cd .infuse && cat journal.jsonl",
		"bash -c 'cat .infuse/journal.jsonl'",
		"X=$(cat .infuse/journal.jsonl)",
		"Get-Content .infuse/journal.jsonl",
	)
	allowed(t, g, root,
		"cat .infuse/oracle/checks.json", // the agent sees the criteria
		"cat .infuse/docs/x.md",
		"cat .infuse/conventions.md",
		"cat .infuse/map/MAP.md",
		"cat .infuse/skills/x/SKILL.md",
		"grep -rn '.infuse/sessions' src",
		"cat README.md",
		"ls -la",
	)
}

// From a worktree kept under .infuse, the project's records are one ../../../ away.
func TestShellRefusesHiddenRecordsFromAWorktree(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	work := filepath.Join(root, ".infuse", "worktrees", "w")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	g := newGuard(root, nil).In(work)
	refused(t, g, work, "cat ../../../.infuse/journal.jsonl", "cat ../../sessions/x.json", "cat ../other/src/a.go")
	allowed(t, g, work, "cat src/a.go", "cat .infuse/docs/x.md")
}

func TestHiddenAt(t *testing.T) {
	root := filepath.FromSlash("/p/proj")
	work := filepath.Join(root, ".infuse", "worktrees", "w")
	if runtime := filepath.VolumeName(os.TempDir()); runtime != "" {
		root, work = runtime+root, runtime+work
	}
	cases := []struct {
		abs  string
		want bool
	}{
		{filepath.Join(work, "src", "a.go"), false},
		{filepath.Join(work, ".infuse", "sessions", "s.json"), true},
		{filepath.Join(work, ".infuse", "docs", "x.md"), false},
		{filepath.Join(root, ".infuse", "journal.jsonl"), true},
		{filepath.Join(root, ".infuse", "worktrees", "other", "a.go"), true},
		{filepath.Join(root, "src", "a.go"), false},
		{filepath.Join(filepath.Dir(root), "other", ".infuse", "sessions", "x"), true},
		{filepath.Join(filepath.Dir(root), "other", "a.go"), false},
	}
	for _, c := range cases {
		if _, got := HiddenAt(c.abs, work, root); got != c.want {
			t.Errorf("%s: got %v", c.abs, got)
		}
	}
}

// The project's policy reaches a worktree: Write("CLAUDE.md") there is the project's instruction file.
func TestPolicyMatchesInsideAWorktree(t *testing.T) {
	root := t.TempDir()
	for _, work := range []string{filepath.Join(root, ".infuse", "worktrees", "w"), filepath.Join(t.TempDir(), "external")} {
		if err := os.MkdirAll(work, 0o755); err != nil {
			t.Fatal(err)
		}
		g := NewGuard(root, Policy{Protected: []string{"secrets/**"}, References: []string{"docs/ref"}, InstructionFiles: []string{"CLAUDE.md"}, Memory: []string{".infuse/memory"}}).In(work)
		for _, rel := range []string{"CLAUDE.md", "secrets/key", "docs/ref/a.md", ".infuse/memory/x"} {
			if _, ok := g.Protects(work, rel); !ok {
				t.Errorf("%s in %s escapes the project's policy", rel, work)
			}
		}
		if _, ok := g.Protects(work, "src/a.go"); ok {
			t.Error("ordinary worktree file became protected")
		}
	}
	// a worktree in the default folder is recognized even by a guard that was not told about it
	g := NewGuard(root, Policy{InstructionFiles: []string{"CLAUDE.md"}})
	if _, ok := g.Protects(root, ".infuse/worktrees/w/CLAUDE.md"); !ok {
		t.Fatal("the default worktree folder lost the policy")
	}
}

func TestControlFilesAreProtectedAndSkillsStayReadable(t *testing.T) {
	root := t.TempDir()
	g := NewGuard(root, Policy{})
	for _, rel := range []string{".infuse/rules.json", ".infuse/launch.json", ".infuse/commands/x.md", ".infuse/skills/s/SKILL.md", ".claude/settings.local.json"} {
		if _, ok := g.Protects(root, rel); !ok {
			t.Errorf("%s is not protected", rel)
		}
	}
	if HiddenInternal(".infuse/skills/s/SKILL.md") {
		t.Fatal("skills must stay readable")
	}
	if _, ok := g.Protects(root, ".infuse/docs/pinned.md"); ok {
		t.Fatal("pinned docs stay writable")
	}
	found := map[string]bool{}
	for _, r := range InfuseRecords() {
		found[r] = true
	}
	for _, r := range []string{".infuse/rules.json", ".infuse/commands", ".git/config", ".git/hooks"} {
		if !found[r] {
			t.Errorf("the sandbox does not get %s", r)
		}
	}
}
