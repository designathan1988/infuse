package oracle

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSpec(t *testing.T, root, spec string) {
	t.Helper()
	p := SpecFile(root)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExecutorVerdictFromEvidence(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	_ = os.WriteFile(filepath.Join(root, "soma.js"), []byte("module.exports = (a, b) => a + b;\n"), 0o644) // uncommitted: the snapshot must include it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<h1>Login</h1>") }))
	defer srv.Close()
	writeSpec(t, root, `{"checks":[
		{"id":"arquivo","kind":"file","path":"soma.js","expect":{"contains":"a + b"}},
		{"id":"comando","kind":"command","run":"test -f soma.js && echo achou","expect":{"contains":"achou"}},
		{"id":"pagina","kind":"http","url":"`+srv.URL+`","expect":{"status":200,"contains":"Login"}},
		{"id":"tamanho","kind":"maxLines","glob":"**/*.js","max":50}
	]}`)
	v, err := RunOracle(context.Background(), root, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "aprovado" || len(v.Checks) != 4 || !strings.HasPrefix(v.Snapshot, "git:") {
		t.Fatalf("%+v", v)
	}
	if _, err := os.Stat(filepath.Join(EvidenceDir(root), v.Run, "comando.log")); err != nil {
		t.Fatal("evidence not stored")
	}
	if st := gitOutput(root, "status", "--porcelain"); !strings.Contains(st, "soma.js") {
		t.Fatal("the user's index or working tree was changed by the snapshot")
	}

	// a failing criterion: the verdict is "reprovado" with the reason, whatever the agent claims
	writeSpec(t, root, `{"checks":[{"id":"teste","kind":"command","run":"exit 3"},{"id":"existe","kind":"file","path":"nao-existe.js"}]}`)
	v, _ = RunOracle(context.Background(), root, root, "")
	if v.Verdict != "reprovado" || v.Checks[0].Passed || !strings.Contains(v.Checks[0].Detail, "exit 3") || v.Checks[1].Passed {
		t.Fatalf("%+v", v)
	}
	if len(ListVerdicts(root, 10)) != 2 {
		t.Fatal("verdict history")
	}
}

func TestExecutorWithoutGitAndWithoutCriteria(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	v, _ := RunOracle(context.Background(), root, root, "")
	if v.Verdict != "sem-criterios" {
		t.Fatalf("%+v", v)
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("ok\n"), 0o644)
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt","expect":{"contains":"ok"}}]}`)
	v, _ = RunOracle(context.Background(), root, root, "")
	if v.Verdict != "aprovado" || !strings.HasPrefix(v.Snapshot, "sha256:") {
		t.Fatalf("%+v", v)
	}
}

func TestAgentCannotTouchCriteriaOrEvidence(t *testing.T) {
	root, g := bypassRoot(t)
	for _, cmd := range []string{
		"echo '{}' > .infuse/oracle/checks.json && echo x",
		"rm -rf .infuse/evidence",
		"cd .infuse/evidence && rm -f */verdict.json",
	} {
		out, _, err := g.GuardCommand(cmd, root)
		if err == nil && !strings.Contains(out, "/dev/null") {
			t.Fatalf("not refused or neutralized: %q → %q", cmd, out)
		}
	}
}
