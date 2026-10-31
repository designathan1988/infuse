package main

import (
	"strings"
	"testing"
)

// FuzzGuardNeverLetsAWriteToTheOracleThrough: whatever the command, the guard must not panic, and a
// command that still mentions an oracle path as a redirect target after guarding must be refused or
// neutralized. Run longer with: go test -fuzz=FuzzGuard -fuzztime=60s
func FuzzGuardNeverLetsAWriteToTheOracleThrough(f *testing.F) {
	for _, seed := range []string{
		"echo x > .infuse/oracle/done.md",
		"cat <<EOF > .infuse/oracle/a.md\nx\nEOF",
		"cd .infuse && echo x >> oracle/b.md",
		"bash -c 'echo x > .infuse/oracle/c.md'",
		"tee .infuse/oracle/d.md < /dev/null",
		"cp /tmp/x .infuse/evidence/run/verdict.json",
		"sed -i s/a/b/ .infuse/oracle/checks.json",
		"git checkout -- .infuse/oracle",
		"powershell -Command \"Set-Content .infuse/oracle/e.md x\"",
		"ls; echo ok",
		"python -c \"open('.infuse/oracle/f.md','w').write('x')\"",
		"find . -name '*.md' -exec rm {} \\;",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, cmd string) {
		root := t.TempDir()
		g := newGuard(root, nil)
		out, _, err := g.GuardCommand(cmd, root)
		if err != nil {
			return // refused
		}
		// after guarding, no plain redirect may still point into the oracle or the evidence
		for _, m := range reRedir.FindAllStringSubmatch(out, -1) {
			target := strings.Trim(m[3], `"'`)
			if target == "/dev/null" || strings.ContainsAny(target, "$`*?") {
				continue // neutralized, or decided only at run time (expansions are refused by the guard itself)
			}
			if rel, bad := g.protects(root, target); bad {
				t.Fatalf("redirect into a protected path (%s) survived the guard:\n in: %q\nout: %q", rel, cmd, out)
			}
		}
	})
}

// A refusal and a note of the guard are messages: the model reads the English, the user their language.
func TestGuardSpeaksInMessages(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, []string{"secrets/**"})
	_, _, err := g.GuardCommandMsgs("rm secrets/key.pem", root)
	m := msgOf(err)
	want := M("guard.protected", "file", "secrets/key.pem", "detail", M("guard.with_command", "why", M("guard.command"), "command", "rm secrets/key.pem"))
	if m.String() != want.String() || err.Error() != want.String() {
		t.Fatalf("%v", err)
	}
	if !strings.HasPrefix(m.Render("pt-BR"), "caminho protegido, somente leitura para o agente: secrets/key.pem (comando sobre caminho protegido recusado: rm") {
		t.Fatal(m.Render("pt-BR"))
	}
	_, notes, err := g.GuardCommandMsgs("echo x > secrets/a", root)
	if err != nil || len(notes) != 1 || notes[0].String() != M("guard.redirected", "file", "secrets/a").String() {
		t.Fatalf("%v %v", notes, err)
	}
}
